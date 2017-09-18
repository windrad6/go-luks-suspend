package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	g "goLuksSuspend"

	"github.com/guns/golibs/editreader"
	"github.com/guns/golibs/sys"
)

func loadCryptdevices(r io.Reader) (cryptdevs []g.Cryptdevice, err error) {
	err = gob.NewDecoder(r).Decode(&cryptdevs)
	return cryptdevs, err
}

func suspendCryptdevices(cryptdevs []g.Cryptdevice) error {
	// Iterate backwards so that we suspend the root device last. This prevents
	// a logical deadlock in which a cryptdevice is actually a file on the root
	// device. There is no way of solving this problem in the general case
	// without building a directed graph of cryptdevices -> cryptdevices.
	for i := len(cryptdevs) - 1; i >= 0; i-- {
		g.Debug("suspending " + cryptdevs[i].Name)
		err := exec.Command("/usr/bin/cryptsetup", "luksSuspend", cryptdevs[i].Name).Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func startUdevDaemon() error {
	return exec.Command("/usr/lib/systemd/systemd-udevd", "--daemon", "--resolve-names=never").Run()
}

func stopUdevDaemon() error {
	return exec.Command("/usr/bin/udevadm", "control", "--exit").Run()
}

func luksResume(dev g.Cryptdevice, stdin io.Reader) error {
	cmd := exec.Command("/usr/bin/cryptsetup", "--tries=1", "luksResume", dev.Name)
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPassphrasePrompt(rootdev string) {
	if g.DebugMode {
		fmt.Println("\nPress Escape to suspend to RAM or Ctrl-T to start a debug shell.")
	} else {
		fmt.Println("\nPress Escape to suspend to RAM.")
	}
	fmt.Printf("\nEnter passphrase for %s: ", rootdev)
}

func resumeRootCryptdevice(rootdev g.Cryptdevice) error {
	restoreTTY, err := sys.AlterTTY(os.Stdin.Fd(), sys.TCSETSF, func(tty syscall.Termios) syscall.Termios {
		tty.Lflag &^= syscall.ICANON | syscall.ECHO
		return tty
	})

	ttyRestored := false

	if restoreTTY != nil {
		defer func() {
			if !ttyRestored {
				g.Assert(restoreTTY())
			}
		}()
	}

	if err != nil {
		g.Warn(err.Error())
		return luksResume(rootdev, os.Stdin)
	}

	printPassphrasePrompt(rootdev.Name)

	// The `secure` parameter to editreader.New zeroes memory aggressively
	r := editreader.New(os.Stdin, 4096, true, func(i int, b byte) editreader.Op {
		switch b {
		case 0x1b: // ^[
			g.Debug("suspending to RAM")
			g.Assert(g.SuspendToRAM())
			fmt.Println()
			printPassphrasePrompt(rootdev.Name)
			return editreader.Kill
		case 0x17: // ^W
			return editreader.Kill
		case '\n':
			fmt.Println()
			g.Assert(restoreTTY())
			ttyRestored = true
			return editreader.Append | editreader.Flush | editreader.Close
		case 0x14: // ^T
			if g.DebugMode {
				g.DebugShell()
				printPassphrasePrompt(rootdev.Name)
				return editreader.Kill
			}
			fallthrough
		default:
			return editreader.BasicLineEdit(i, b)
		}
	})

	return luksResume(rootdev, r)
}
