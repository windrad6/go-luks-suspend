package goLuksSuspend

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

type Cryptdevice struct {
	Name         string
	UUID         []byte
	dmdir        string
	Keyfile      Keyfile
	IsRootDevice bool
}

var luksUUIDPrefix = []byte("CRYPT-LUKS1-")

func GetCryptdevices() ([]Cryptdevice, map[string]*Cryptdevice, error) {
	dirs, err := filepath.Glob("/sys/block/*/dm")
	if err != nil || len(dirs) == 0 {
		return nil, nil, err
	}

	rootdev, rootkey, err := getLUKSParamsFromKernelCmdline()
	if err != nil {
		return nil, nil, err
	}

	cryptdevs := make([]Cryptdevice, 0, len(dirs))
	cdmap := make(map[string]*Cryptdevice, len(dirs))

	for i := range dirs {
		// Skip if not a LUKS device
		uuid, err := ioutil.ReadFile(filepath.Join(dirs[i], "uuid"))
		if err != nil {
			return nil, nil, err
		} else if !bytes.Equal(uuid[:len(luksUUIDPrefix)], luksUUIDPrefix) {
			continue
		}

		cd := Cryptdevice{
			dmdir: dirs[i],
			UUID:  bytes.TrimSuffix(uuid, []byte{'\n'}),
		}

		// Skip if suspended
		if cd.Suspended() {
			continue
		}

		name, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "name"))
		if err != nil {
			return nil, nil, err
		}

		cd.Name = string(bytes.TrimSpace(name))
		if cd.Name == rootdev {
			cd.IsRootDevice = true
			cd.Keyfile = rootkey
		}
		cryptdevs = append(cryptdevs, cd)

		if v, ok := cdmap[cd.Name]; ok {
			return nil, nil, fmt.Errorf("duplicate cryptdevice: %#v", v)
		}
		cdmap[cd.Name] = &cryptdevs[len(cryptdevs)-1]
	}

	return cryptdevs, cdmap, nil
}

func (cd *Cryptdevice) Exists() bool {
	uuid, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "uuid"))
	if err != nil {
		// A read error implies this device has been removed
		return false
	}

	return bytes.Equal(cd.UUID, bytes.TrimSuffix(uuid, []byte{'\n'}))
}

func (cd *Cryptdevice) Suspended() bool {
	buf, err := ioutil.ReadFile(filepath.Join(cd.dmdir, "suspended"))
	if err != nil || len(buf) == 0 {
		// Ignore the error here for a cleaner API; read errors imply
		// that the device is gone, so technically, it's not suspended
		return false
	}

	return buf[0] == '1'
}

func (cd *Cryptdevice) ResumeWithKeyfile() error {
	args := make([]string, 0, 8)

	args = append(args, "--key-file", cd.Keyfile.path)
	if cd.Keyfile.offset > 0 {
		args = append(args, "--keyfile-offset", strconv.Itoa(cd.Keyfile.offset))
	}
	if cd.Keyfile.size > 0 {
		args = append(args, "--keyfile-size", strconv.Itoa(cd.Keyfile.size))
	}
	args = append(args, "luksResume", cd.Name)

	return exec.Command("/usr/bin/cryptsetup", args...).Run()
}

var kernelCmdline = "/proc/cmdline"

func getLUKSParamsFromKernelCmdline() (rootdev string, key Keyfile, err error) {
	buf, err := ioutil.ReadFile(kernelCmdline)
	if err != nil {
		return "", Keyfile{}, err
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
		return "", Keyfile{}, errors.New("no root cryptdevice")
	}

	return rootdev, key, nil
}

var ignoreLinePattern = regexp.MustCompile(`\A\s*\z|\A\s*#`)

func AddKeyfilesFromCrypttab(cdmap map[string]*Cryptdevice) error {
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
			cd.Keyfile = key
		}
	}

	return file.Close()
}
