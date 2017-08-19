package suspend

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"regexp"
)

type CryptDevice struct {
	Device       string
	DMName       string
	Mountpoint   string
	FSType       string
	MountOpts    string
	Keyfile      string
	NeedsRemount bool
}

// GetCryptDevices returns active non-root crypt devices from /etc/crypttab
func GetCryptDevices() (cds []CryptDevice) {
	return nil
}

var ignoreLinePattern = regexp.MustCompile(`\A\s*\z|\A\s*#`)

func scanCrypttab(path string) ([]CryptDevice, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var cds []CryptDevice
	s := bufio.NewScanner(file)

	for s.Scan() {
		line := s.Bytes()
		if ignoreLinePattern.Match(line) {
			continue
		}

		fields := bytes.Fields(line)
		cds = append(cds, CryptDevice{DMName: string(fields[0]), Keyfile: string(fields[2])})
	}

	if err = file.Close(); err != nil {
		return nil, err
	}

	return cds, nil
}

// Poweroff attempts to shutdown the system via /proc/sysrq-trigger
func Poweroff() {
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600)
	}
}

// SuspendToRAM attempts to suspend the system via /sys/power/state
func SuspendToRAM() {
	if err := ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600); err != nil {
		Poweroff()
	}
}
