package goLuksSuspend

import (
	"strconv"
	"strings"
)

type Keyfile struct {
	path   string
	offset int
	size   int
}

func parseKeyfileFromCrypttabEntry(line string) (name string, key Keyfile) {
	fields := strings.Fields(line)

	// fields: name, device, keyfile, options
	//
	// crypttab(5):
	// The third field specifies the encryption password. If the field is
	// not present or the password is set to "none" or "-", the password
	// has to be manually entered during system boot.
	if len(fields) < 3 || fields[2] == "-" || fields[2] == "none" {
		return "", Keyfile{}
	}

	k := Keyfile{path: fields[2]}

	if len(fields) >= 4 {
		opts := strings.Split(fields[3], ",")
		for i := range opts {
			kv := strings.SplitN(opts[i], "=", 2)
			if len(kv) < 2 {
				continue
			} else if kv[0] == "keyfile-offset" {
				n, err := strconv.Atoi(kv[1])
				if err != nil {
					continue
				}
				k.offset = n
			} else if kv[0] == "keyfile-size" {
				n, err := strconv.Atoi(kv[1])
				if err != nil {
					continue
				}
				k.size = n
			}
		}
	}

	return fields[0], k
}

func (k *Keyfile) Exists() bool {
	return len(k.path) > 0
}
