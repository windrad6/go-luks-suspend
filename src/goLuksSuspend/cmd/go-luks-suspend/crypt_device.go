package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type cryptdevice struct {
	name         string
	uuid         string
	dmdir        string
	keyfile      string
	isRootDevice bool
}

func getcryptdevices() ([]cryptdevice, error) {
	cryptdevs, err := cryptdevicesFromSysfs()
	if err != nil {
		return nil, err
	} else if len(cryptdevs) == 0 {
		return nil, nil
	}

	cdmap := make(map[string]*cryptdevice, len(cryptdevs))

	for i := range cryptdevs {
		cdmap[cryptdevs[i].name] = &cryptdevs[i]
	}

	if err := addKeyfilesFromCrypttab(cdmap); err != nil {
		return nil, err
	}

	return cryptdevs, nil
}

func (cd *cryptdevice) exists() bool {
	uuid, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "uuid"))
	if err != nil {
		// A read error implies this device has been removed
		return false
	}

	return cd.uuid == string(bytes.TrimSpace(uuid))
}

func (cd *cryptdevice) suspended() bool {
	buf, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "suspended"))
	if err != nil {
		// Ignore the error here for a cleaner API; read errors imply
		// that the device is gone, so technically, it's not suspended
		return false
	}

	return buf[0] == '1'
}

func (cd *cryptdevice) resumeWithKeyfile() error {
	return exec.Command("/usr/bin/cryptsetup", "--key-file", cd.keyfile, "luksResume", cd.name).Run()
}

func dumpCryptdevices(path string, cryptdevs []cryptdevice) error {
	buf := make([][]byte, len(cryptdevs))
	j := 1

	for i := range cryptdevs {
		if cryptdevs[i].isRootDevice {
			if len(buf[0]) > 0 {
				return fmt.Errorf(
					"multiple root cryptdevices: %s, %s",
					string(buf[0]),
					cryptdevs[i].name,
				)
			}
			buf[0] = []byte(cryptdevs[i].name)
		} else if j >= len(buf) {
			return errors.New("no root cryptdevice")
		} else {
			buf[j] = []byte(cryptdevs[i].name)
			j++
		}
	}

	return ioutil.WriteFile(path, bytes.Join(buf, []byte{0}), 0600)
}

func getCryptdeviceFromKernelCmdline(path string) (string, error) {
	buf, err := ioutil.ReadFile(path)
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

func cryptdevicesFromSysfs() ([]cryptdevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil {
		return nil, err
	} else if len(dirs) == 0 {
		return nil, nil
	}

	rootdev, err := getCryptdeviceFromKernelCmdline("/proc/cmdline")
	if err != nil {
		return nil, err
	}

	cryptdevs := make([]cryptdevice, 0, len(dirs))

	for i := range dirs {
		// Skip if not a LUKS device
		uuid, err := ioutil.ReadFile(filepath.Join(dirs[i], "uuid"))
		if err != nil {
			return nil, err
		} else if string(uuid[:12]) != "CRYPT-LUKS1-" {
			continue
		}

		cd := cryptdevice{
			dmdir: dirs[i],
			uuid:  string(bytes.TrimSpace(uuid)),
		}

		// Skip if suspended
		if cd.suspended() {
			continue
		}

		name, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "name"))
		if err != nil {
			return nil, err
		}

		cd.name = string(bytes.TrimSpace(name))
		if cd.name == rootdev {
			cd.isRootDevice = true
		}
		cryptdevs = append(cryptdevs, cd)
	}

	return cryptdevs, nil
}

var ignoreLinePattern = regexp.MustCompile(`\A\s*\z|\A\s*#`)

func addKeyfilesFromCrypttab(cdmap map[string]*cryptdevice) error {
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
			cd.keyfile = string(fields[2])
		}
	}

	return file.Close()
}
