package archLuksSuspend

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var DebugMode = false

// Poweroff attempts to shutdown the system via /proc/sysrq-trigger
func Poweroff() {
	if DebugMode {
		fmt.Fprintln(os.Stderr, "POWEROFF")
		os.Exit(1)
	}
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600)
	}
}

// SuspendToRAM attempts to suspend the system via /sys/power/state
func SuspendToRAM() error {
	return ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
}

// Dump writes the names of a slice of CryptDevices as a NUL delimited
// sequence of bytes
func Dump(path string, cryptdevices []CryptDevice) error {
	buf := make([][]byte, len(cryptdevices))
	for i := range cryptdevices {
		buf[i] = []byte(cryptdevices[i].Name)
	}
	return ioutil.WriteFile(path, bytes.Join(buf, []byte{0}), 0600)
}

// Load loads the names written to a path by Dump
func Load(path string) ([]string, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(buf), "\x00"), nil
}
