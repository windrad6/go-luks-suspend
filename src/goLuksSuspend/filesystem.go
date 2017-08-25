package goLuksSuspend

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
)

type Filesystem struct {
	Mountpoint string
	DevNo      uint64
}

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
			devno, err := lstatDevno(fields[1])
			if err != nil {
				return nil, err
			}

			fs = append(fs, Filesystem{
				Mountpoint: fields[1],
				DevNo:      devno,
			})
		}
	}

	if err := file.Close(); err != nil {
		return nil, err
	}

	return fs, nil
}

func (fs Filesystem) IsMounted() bool {
	devno, err := lstatDevno(fs.Mountpoint)
	if err != nil {
		return false
	}

	return fs.DevNo == devno
}

func (fs Filesystem) DisableWriteBarrier() error {
	return syscall.Mount("", fs.Mountpoint, "", syscall.MS_REMOUNT, "nobarrier")
}

func (fs Filesystem) EnableWriteBarrier() error {
	return syscall.Mount("", fs.Mountpoint, "", syscall.MS_REMOUNT, "barrier")
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

func lstatDevno(path string) (uint64, error) {
	// stat(2):
	// On Linux, lstat() will generally not trigger automounter action,
	// whereas stat() will
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}

	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("lstat %#v: no stat_t", path)
	}

	return st.Dev, nil
}
