package goLuksSuspend

import (
	"bufio"
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type CryptDevice struct {
	Name         string
	DMDir        string // /sys/block/dm-%d/dm
	DMDevice     string // /dev/mapper/%s
	Mountpoint   string
	Keyfile      string
	NeedsRemount bool
	IsRootDevice bool
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
		cdmap[cryptdevices[i].DMDevice] = &cryptdevices[i] // to match entry in /proc/mounts
	}

	if err := addKeyfilesFromCrypttab(cdmap); err != nil {
		return nil, err
	}

	if err := addMountInfo(cdmap); err != nil {
		return nil, err
	}

	return cryptdevices, nil
}

func (cd *CryptDevice) IsSuspended() (bool, error) {
	buf, err := ioutil.ReadFile(filepath.Join(cd.DMDir, "suspended"))
	if err != nil {
		return false, err
	}

	return buf[0] == '1', nil
}

func (cd *CryptDevice) CanResumeWithKeyfile() (bool, error) {
	if len(cd.Keyfile) == 0 {
		return false, nil
	}

	if suspended, err := cd.IsSuspended(); err != nil {
		return false, err
	} else if !suspended {
		return false, nil
	}

	return true, nil
}

// LuksResumeWithKeyfile resumes this CryptDevice with its keyfile
func (cd *CryptDevice) LuksResumeWithKeyfile() error {
	return Run(
		nil,
		[]string{"/usr/bin/cryptsetup", "--key-file", cd.Keyfile, "luksResume", cd.Name},
		false,
	)
}

func (cd *CryptDevice) DisableWriteBarrier() error {
	return syscall.Mount("", cd.Mountpoint, "", syscall.MS_REMOUNT, "nobarrier")
}

func (cd *CryptDevice) EnableWriteBarrier() error {
	return syscall.Mount("", cd.Mountpoint, "", syscall.MS_REMOUNT, "barrier")
}

var kernelCmdline = "/proc/cmdline"

func getCryptdeviceFromKernelCmdline() (string, error) {
	buf, err := ioutil.ReadFile(kernelCmdline)
	if err != nil {
		return "", err
	}

	params := strings.Fields(string(buf))

	// Grab the last instance in case of duplicates
	for i := len(params) - 1; i >= 0; i-- {
		p := params[i]
		if len(p) > 12 && p[:12] == "cryptdevice=" {
			fields := strings.SplitN(p, ":", 3)
			if len(fields) < 2 {
				return "", errors.New("malformed cryptdevice= kernel parameter")
			}

			return fields[1], nil
		}
	}

	return "", nil
}

func cryptDevicesFromSysfs() ([]CryptDevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil {
		return nil, err
	} else if len(dirs) == 0 {
		return nil, nil
	}

	rootdev, err := getCryptdeviceFromKernelCmdline()
	if err != nil {
		return nil, err
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

		cd := CryptDevice{DMDir: dirs[i]}

		// Skip if suspended
		susp, err := cd.IsSuspended()
		if err != nil {
			return nil, err
		} else if susp {
			continue
		}

		name, err := ioutil.ReadFile(filepath.Join(cd.DMDir, "name"))
		if err != nil {
			return nil, err
		}

		cd.Name = string(bytes.TrimSpace(name))
		cd.DMDevice = "/dev/mapper/" + cd.Name
		if cd.Name == rootdev {
			cd.IsRootDevice = true
		}
		cryptdevices = append(cryptdevices, cd)
	}

	return cryptdevices, nil
}

var ignoreLinePattern = regexp.MustCompile(`\A\s*\z|\A\s*#`)

func addKeyfilesFromCrypttab(cdmap map[string]*CryptDevice) error {
	file, err := os.Open("/etc/crypttab")
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

func addMountInfo(cdmap map[string]*CryptDevice) error {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return err
	}

	s := bufio.NewScanner(file)

	for s.Scan() {
		fields := strings.Fields(s.Text())

		if cd, ok := cdmap[fields[0]]; ok {
			cd.Mountpoint = fields[1]
			cd.NeedsRemount = needsRemount(fields[2], fields[3])
		}
	}

	return file.Close()
}

func needsRemount(fstype, mountopts string) bool {
	switch fstype {
	// ReiserFS supports write barriers, but the option syntax appears to
	// be unconventional. Since it's fading into obscurity, just ignore it.
	case "ext3", "ext4", "btrfs":
		for _, o := range strings.Split(mountopts, ",") {
			// Write barriers are on by default and do not show up
			// in the list of mount options, so check for the negative
			if o == "barrier=0" || o == "nobarrier" {
				return false
			}
		}
		return true
	}
	return false
}
