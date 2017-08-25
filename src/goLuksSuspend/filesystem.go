package goLuksSuspend

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

type Filesystem string

func GetFilesystemsWithWriteBarriers() ([]Filesystem, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}

	fs := []Filesystem{}
	s := bufio.NewScanner(file)

	for s.Scan() {
		fields := strings.Fields(s.Text())

		if hasWriteBarrier(fields[2], fields[3]) {
			fs = append(fs, Filesystem(fields[1]))
		}
	}

	if err := file.Close(); err != nil {
		return nil, err
	}

	return fs, nil
}

func (fs Filesystem) Mountpoint() string {
	return string(fs)
}

func (fs Filesystem) DisableWriteBarrier() error {
	return syscall.Mount("", fs.Mountpoint(), "", syscall.MS_REMOUNT, "nobarrier")
}

func (fs Filesystem) EnableWriteBarrier() error {
	return syscall.Mount("", fs.Mountpoint(), "", syscall.MS_REMOUNT, "barrier")
}

func hasWriteBarrier(fstype, mountopts string) bool {
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
