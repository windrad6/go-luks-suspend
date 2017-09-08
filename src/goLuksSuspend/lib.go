package goLuksSuspend

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"syscall"
)

func DebugShell() (ok bool) {
	log.Println("===============================================================")
	log.Println("  DEBUG: `exit 42` if go-luks-suspend should resume execution  ")
	log.Println("===============================================================")

	cmd := exec.Command("/bin/sh")
	cmd.Env = []string{"PS1=[\\w \\u\\$] "}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if ws, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				return ws.ExitStatus() == 42
			}
		}
	}

	return false
}

func Poweroff() {
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600) // errcheck: POWERING OFF!
	}
}
