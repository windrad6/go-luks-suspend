package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	g "goLuksSuspend"

	"github.com/guns/golibs/editreader"
	"github.com/guns/golibs/sys"
)

// loadCryptnames loads the names written to a path by Dump
func loadCryptnames(path string) ([]string, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(buf), "\x00"), nil
}

func suspendCryptDevices(deviceNames []string) error {
	// Iterate backwards so that we suspend the root device last. This prevents
	// a logical deadlock in which a cryptdevice is actually a file on the root
	// device. There is no way of solving this problem in the general case
	// without building a directed graph of cryptdevices -> cryptdevices.
	for i := len(deviceNames) - 1; i >= 0; i-- {
		g.Debug("suspending " + deviceNames[i])
		err := exec.Command("/usr/bin/cryptsetup", "luksSuspend", deviceNames[i]).Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func luksResume(device string, stdin io.Reader) error {
	cmd := exec.Command("/usr/bin/cryptsetup", "--tries=1", "luksResume", device)
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

func resumeRootCryptDevice(rootdev string) error {
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

	printPassphrasePrompt(rootdev)

	// The `secure` parameter to editreader.New zeroes memory aggressively
	r := editreader.New(os.Stdin, 4096, true, func(i int, b byte) editreader.Op {
		switch b {
		case 0x14: // ^T
			if g.DebugMode {
				g.DebugShell()
			}
			return 0
		case 0x1b: // ^[
			g.Debug("suspending to RAM")
			g.Assert(g.SuspendToRAM())
			fmt.Println()
			printPassphrasePrompt(rootdev)
			return editreader.Kill
		case 0x17: // ^W
			return editreader.Kill
		case '\n':
			fmt.Println()
			g.Assert(restoreTTY())
			ttyRestored = true
			return editreader.Append | editreader.Flush | editreader.Close
		default:
			return editreader.BasicLineEdit(i, b)
		}
	})

	return luksResume(rootdev, r)
}
