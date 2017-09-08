package goLuksSuspend

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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

func DebugShell() {
	log.Println("===========================")
	log.Println("        DEBUG SHELL        ")
	log.Println("===========================")

	cmd := exec.Command("/bin/sh")
	cmd.Env = []string{"PS1=[\\w \\u\\$] "}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	_ = cmd.Run()

	fmt.Println("EXIT DEBUG SHELL")
}

const freezeTimeoutPath = "/sys/power/pm_freeze_timeout"

func SetFreezeTimeout(timeout []byte) (oldtimeout []byte, err error) {
	oldtimeout, err = ioutil.ReadFile(freezeTimeoutPath)
	if err != nil {
		return nil, err
	}
	return oldtimeout, ioutil.WriteFile(freezeTimeoutPath, timeout, 0644)
}

func SuspendToRAM() error {
	return ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
}

func Poweroff() {
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600) // errcheck: POWERING OFF!
	}
}
