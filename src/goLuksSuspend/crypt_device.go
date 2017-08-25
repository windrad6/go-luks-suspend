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
)

type CryptDevice struct {
	Name         string
	DMDir        string // /sys/block/dm-%d/dm
	Keyfile      string
	IsRootDevice bool
}

// GetCryptDevices returns active non-root crypt devices from /etc/crypttab
func GetCryptDevices() ([]CryptDevice, error) {
	cryptdevices, err := cryptDevicesFromSysfs()
	if err != nil {
		return nil, err
	}

	cdmap := make(map[string]*CryptDevice, len(cryptdevices))

	for i := range cryptdevices {
		cdmap[cryptdevices[i].Name] = &cryptdevices[i]
	}

	if err := addKeyfilesFromCrypttab(cdmap); err != nil {
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
