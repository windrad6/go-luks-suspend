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
	"strconv"
	"strings"
)

type cryptdevice struct {
	name         string
	uuid         string
	dmdir        string
	keyfile      keyfile
	isRootDevice bool
}

func getcryptdevices() ([]cryptdevice, map[string]*cryptdevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil {
		return nil, nil, err
	} else if len(dirs) == 0 {
		return nil, nil, nil
	}

	rootdev, err := getCryptdeviceFromKernelCmdline("/proc/cmdline")
	if err != nil {
		return nil, nil, err
	}

	cryptdevs := make([]cryptdevice, 0, len(dirs))
	cdmap := make(map[string]*cryptdevice, len(dirs))

	for i := range dirs {
		// Skip if not a LUKS device
		uuid, err := ioutil.ReadFile(filepath.Join(dirs[i], "uuid"))
		if err != nil {
			return nil, nil, err
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
			return nil, nil, err
		}

		cd.name = string(bytes.TrimSpace(name))
		if cd.name == rootdev {
			cd.isRootDevice = true
		}
		cryptdevs = append(cryptdevs, cd)

		if v, ok := cdmap[cd.name]; ok {
			return nil, nil, fmt.Errorf("duplicate cryptdevice: %#v", v)
		}
		cdmap[cd.name] = &cryptdevs[len(cryptdevs)-1]
	}

	return cryptdevs, cdmap, nil
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
	if err != nil || len(buf) == 0 {
		// Ignore the error here for a cleaner API; read errors imply
		// that the device is gone, so technically, it's not suspended
		return false
	}

	return buf[0] == '1'
}

func (cd *cryptdevice) resumeWithKeyfile() error {
	args := make([]string, 0, 8)

	args = append(args, "--key-file", cd.keyfile.path)
	if cd.keyfile.offset > 0 {
		args = append(args, "--keyfile-offset", strconv.Itoa(cd.keyfile.offset))
	}
	if cd.keyfile.size > 0 {
		args = append(args, "--keyfile-size", strconv.Itoa(cd.keyfile.size))
	}
	args = append(args, "luksResume", cd.name)

	return exec.Command("/usr/bin/cryptsetup", args...).Run()
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
		kv := strings.SplitN(params[i], "=", 2)
		if len(kv) < 2 || kv[0] != "cryptdevice" {
			continue
		}

		// cryptdevice=device:dmname:options
		fields := strings.SplitN(kv[1], ":", 3)
		if len(fields) < 2 {
			continue
		}

		return fields[1], nil
	}

	return "", errors.New("no root cryptdevice")
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

		name, key := parseKeyfileFromCrypttabEntry(string(line))
		if len(name) == 0 {
			continue
		}

		if cd, ok := cdmap[name]; ok {
			cd.keyfile = key
		}
	}

	return file.Close()
}
