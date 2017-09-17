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
	uuid         []byte
	dmdir        string
	keyfile      keyfile
	isRootDevice bool
}

var luksUUIDPrefix = []byte("CRYPT-LUKS1-")

func getcryptdevices() ([]cryptdevice, map[string]*cryptdevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil {
		return nil, nil, err
	} else if len(dirs) == 0 {
		return nil, nil, nil
	}

	rootdev, rootkey, err := getLUKSParamsFromKernelCmdline()
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
		} else if !bytes.Equal(uuid[:len(luksUUIDPrefix)], luksUUIDPrefix) {
			continue
		}

		cd := cryptdevice{
			dmdir: dirs[i],
			uuid:  bytes.TrimSuffix(uuid, []byte{'\n'}),
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
			cd.keyfile = rootkey
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

	return bytes.Equal(cd.uuid, bytes.TrimSuffix(uuid, []byte{'\n'}))
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

var kernelCmdline = "/proc/cmdline"

func getLUKSParamsFromKernelCmdline() (rootdev string, key keyfile, err error) {
	buf, err := ioutil.ReadFile(kernelCmdline)
	if err != nil {
		return "", keyfile{}, err
	}

	//
	// https://git.archlinux.org/svntogit/packages.git/tree/trunk/encrypt_hook?h=packages/cryptsetup
	//

	params := strings.Fields(string(buf))
	key.path = "/crypto_keyfile.bin"

	for i := range params {
		kv := strings.SplitN(params[i], "=", 2)
		if len(kv) < 2 {
			continue
		} else if kv[0] == "cryptdevice" {
			// cryptdevice=device:dmname:options
			fields := strings.SplitN(kv[1], ":", 3)
			if len(fields) < 2 {
				continue
			}

			rootdev = fields[1]
		} else if kv[0] == "cryptkey" {
			fields := strings.SplitN(kv[1], ":", 3)
			if len(fields) < 2 {
				continue
			}

			// cryptkey=rootfs:path
			if len(fields) == 2 && fields[0] == "rootfs" {
				key.path = fields[1]
				continue
			}

			if len(fields) < 3 {
				continue
			}

			if offset, err := strconv.Atoi(fields[1]); err == nil {
				// cryptkey=device:offset:size
				size, err := strconv.Atoi(fields[2])
				if err != nil {
					continue // ignore malformed entry
				}
				key.path = fields[0]
				key.offset = offset
				key.size = size
				continue
			}

			// cryptkey=device:filesystem:path
		}
	}

	if len(rootdev) == 0 {
		return "", keyfile{}, errors.New("no root cryptdevice")
	}

	return rootdev, key, nil
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
