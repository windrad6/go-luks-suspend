package archLuksSuspend

import (
	"fmt"
	"io/ioutil"
	"os"
)

// Poweroff attempts to shutdown the system via /proc/sysrq-trigger
func Poweroff(debugmode bool) {
	if debugmode {
		fmt.Fprintln(os.Stderr, "POWEROFF")
		os.Exit(1)
	}
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600)
	}
}

// SuspendToRAM attempts to suspend the system via /sys/power/state
func SuspendToRAM(debugmode bool) {
	if err := ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600); err != nil {
		Poweroff(debugmode)
	}
}
