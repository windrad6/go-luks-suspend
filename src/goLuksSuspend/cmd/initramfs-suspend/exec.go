package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
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

func suspendCryptDevicesOrPoweroff(deviceNames []string) {
	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	ch := make(chan string)

	wg.Add(1)
	go func() {
		for i := range deviceNames {
			ch <- deviceNames[i]
		}
		close(ch)
		wg.Done()
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for name := range ch {
				g.Debug("suspending " + name)
				g.Assert(exec.Command("/usr/bin/cryptsetup", "luksSuspend", name).Run())
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func luksResume(device string, stdin io.Reader) error {
	cmd := exec.Command("/usr/bin/cryptsetup", "--tries=1", "luksResume", device)
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

	fmt.Printf("\nPress Escape to suspend to RAM.\n\nEnter passphrase for %s: ", rootdev)

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
