package goLuksSuspend

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"syscall"
)

func Poweroff(debugMode bool) {
	if debugMode {
		log.Println("==========================================================")
		log.Println("  DEBUG SHELL: spawning /bin/sh instead of powering off!  ")
		log.Println("   `exit 42` if go-luks-suspend should resume execution   ")
		log.Println("==========================================================")
		err := Run([]string{"PS1=[\\w \\u\\$] "}, []string{"/bin/sh"}, true)
		if err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				if ws, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					if ws.ExitStatus() == 42 {
						return
					}
				}
			}
		}
		os.Exit(1)
	}

	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600) // errcheck: POWERING OFF!
	}
}

func Run(env []string, cmdline []string, stdio bool) error {
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Env = env
	if stdio {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}
