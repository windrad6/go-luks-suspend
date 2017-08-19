package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"suspend"
)

const (
	systemSleepPath = "/usr/lib/systemd/system-sleep"
	initramfsPath   = "/run/initramfs"
)

var BindPaths = []string{"/sys", "/proc", "/dev", "/run"}

type ScriptType int

const (
	PreSuspend ScriptType = iota
	PostSuspend
)

func assert(err error) {
	if err != nil {
		suspend.Poweroff()
	}
}

func checkRootOwnedAndExecutablePath(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	return checkRootOwnedAndExecutable(fi)
}

func checkRootOwnedAndExecutable(fi os.FileInfo) error {
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", fi.Name())
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to read stat_t for %s", fi.Name())
	}

	switch {
	case stat.Uid != 0:
		return fmt.Errorf("%s is not root owned", fi.Name())
	case fi.Mode()&0022 != 0:
		return fmt.Errorf("%s is writable by group or world", fi.Name())
	case fi.Mode()&0111 == 0:
		return fmt.Errorf("%s is not executable", fi.Name())
	}

	return nil
}

func runSystemSuspendScripts(stype ScriptType) error {
	var arg1 string

	switch stype {
	case PreSuspend:
		arg1 = "pre"
	case PostSuspend:
		arg1 = "post"
	default:
		return errors.New("unknown ScriptType")
	}

	dir, err := os.Open(systemSleepPath)
	if err != nil {
		return err
	}

	fs, err := dir.Readdir(0)
	if err != nil {
		return err
	}

	if err := dir.Close(); err != nil {
		return err
	}

	for i := range fs {
		if err := checkRootOwnedAndExecutable(fs[i]); err != nil {
			fmt.Println(err)
			continue
		}

		err := exec.Command(filepath.Join(systemSleepPath, fs[i].Name()), arg1, "suspend").Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsPath, "suspend")))
}
