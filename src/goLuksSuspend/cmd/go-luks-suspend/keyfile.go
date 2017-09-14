package main

import (
	"strconv"
	"strings"
)

type keyfile struct {
	path   string
	offset int
	size   int
}

func parseKeyfileFromCrypttabEntry(line string) (name string, key keyfile) {
	fields := strings.Fields(line)

	// fields: name, device, keyfile, options
	if len(fields) < 3 {
		return "", keyfile{}
	}

	k := keyfile{path: fields[2]}

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

func (k *keyfile) exists() bool {
	return len(k.path) > 0
}
