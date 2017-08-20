package archLuksSuspend

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

type CryptDevice struct {
	Name         string
	DMPath       string // /sys/block/dm-1/dm
	Mountpoint   string
	FSType       string
	MountOpts    string
	Keyfile      string
	NeedsRemount bool
}

// GetCryptDevices returns active non-root crypt devices from /etc/crypttab
func GetCryptDevices() ([]CryptDevice, error) {
	cryptdevices, err := cryptDevicesFromSysfs()
	if err != nil {
		return nil, err
	}

	cdmap := make(map[string]*CryptDevice, 2*len(cryptdevices))
	for i := range cryptdevices {
		cdmap[cryptdevices[i].Name] = &cryptdevices[i]
		cdmap["/dev/mapper/"+cryptdevices[i].Name] = &cryptdevices[i] // to match entry in /proc/mounts
	}

	if err := addKeyfilesFromCrypttab(cdmap, "/etc/crypttab"); err != nil {
		return nil, err
	}

	if err := addMountInfo(cdmap, "/proc/mounts"); err != nil {
		return nil, err
	}

	return cryptdevices, nil
}

func (cd *CryptDevice) IsSuspended() (bool, error) {
	buf, err := ioutil.ReadFile(filepath.Join(cd.DMPath, "suspended"))
	if err != nil {
		return false, err
	}

	return buf[0] == '1', nil
}

func cryptDevicesFromSysfs() ([]CryptDevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil {
		return nil, err
	} else if len(dirs) == 0 {
		return nil, nil
	}

	cryptdevices := make([]CryptDevice, 0, len(dirs))

	for i := range dirs {
		// Skip if not a LUKS device
		buf, err := ioutil.ReadFile(filepath.Join(dirs[i], "uuid"))
		if err != nil {
			return nil, err
		} else if string(buf[:12]) != "CRYPT-LUKS1-" {
			continue
		}

		cd := CryptDevice{DMPath: dirs[i]}

		// Skip if suspended
		susp, err := cd.IsSuspended()
		if err != nil {
			return nil, err
		} else if susp {
			continue
		}

		name, err := ioutil.ReadFile(filepath.Join(cd.DMPath, "name"))
		if err != nil {
			return nil, err
		}

		cd.Name = string(bytes.TrimSpace(name))
		cryptdevices = append(cryptdevices, cd)
	}

	return cryptdevices, nil
}

var ignoreLinePattern = regexp.MustCompile(`\A\s*\z|\A\s*#`)

func addKeyfilesFromCrypttab(cdmap map[string]*CryptDevice, crypttabPath string) error {
	file, err := os.Open(crypttabPath)
	if err != nil {
		return err
	}

	s := bufio.NewScanner(file)

	for s.Scan() {
		line := s.Bytes()
		if ignoreLinePattern.Match(line) {
			continue
		}

		fields := bytes.Fields(line)

		if cd, ok := cdmap[string(fields[0])]; ok {
			cd.Keyfile = string(fields[2])
		}
	}

	return file.Close()
}

func addMountInfo(cdmap map[string]*CryptDevice, mountsPath string) error {
	file, err := os.Open(mountsPath)
	if err != nil {
		return err
	}

	s := bufio.NewScanner(file)

	for s.Scan() {
		fields := strings.Fields(s.Text())

		if cd, ok := cdmap[fields[0]]; ok {
			cd.Mountpoint = fields[1]
			cd.FSType = fields[2]
			cd.MountOpts = fields[3]
			cd.NeedsRemount = needsRemount(cd.FSType, cd.MountOpts)
		}
	}

	return file.Close()
}

func needsRemount(fstype, mountopts string) bool {
	if fstype != "ext4" {
		return false
	}

	for _, opt := range strings.Split(mountopts, ",") {
		if opt == "barrier=0" || opt == "nobarrier" {
			return false
		}
	}

	return true
}
