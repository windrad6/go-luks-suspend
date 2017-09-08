package goLuksSuspend

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"syscall"
)

var DebugMode = false
var PoweroffOnError = true

func Debug(msg string) {
	if DebugMode {
		Warn(msg)
	}
}

func Warn(msg string) {
	log.Println(msg)
}

func Assert(err error) {
	if err != nil {
		Warn(err.Error())
		if DebugMode {
			DebugShell()
		} else if PoweroffOnError {
			Poweroff()
		}
	}
}

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

func SuspendToRAM() error {
	return ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
}

func Poweroff() {
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600) // errcheck: POWERING OFF!
	}
}
