package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"archLuksSuspend"
)

const initramfsPath = "/run/initramfs"
const systemSleepPath = "/usr/lib/systemd/system-sleep"

var debugmode = false
var bindPaths = []string{"/sys", "/proc", "/dev", "/run"}
var systemdServices = []string{
	// journald may attempt to write to the suspended device
	"systemd-journald-dev-log.socket",
	"systemd-journald.socket",
	"systemd-journald.service",
}

func assert(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		archLuksSuspend.Poweroff(debugmode)
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

func runSystemSuspendScripts(scriptarg string) error {
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

		err := exec.Command(filepath.Join(systemSleepPath, fs[i].Name()), scriptarg, "suspend").Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func bindInitramfs() error {
	for _, dir := range bindPaths {
		err := syscall.Mount(dir, filepath.Join(initramfsPath, dir), "", syscall.MS_BIND, "")
		if err != nil {
			return err
		}
	}
	return nil
}

func unbindInitramfs() error {
	for _, dir := range bindPaths {
		err := syscall.Unmount(filepath.Join(initramfsPath, dir), 0)
		if err != nil {
			return err
		}
	}
	return nil
}

func systemctlServices(command string) error {
	return exec.Command("/usr/bin/systemctl", append([]string{command}, systemdServices...)...).Run()
}

const disableBarrier = false
const enableBarrier = true

func remountDevicesWithWriteBarriers(cryptdevices []archLuksSuspend.CryptDevice, enable bool) error {
	for i := range cryptdevices {
		if cryptdevices[i].NeedsRemount {
			if suspended, err := cryptdevices[i].IsSuspended(); err != nil {
				return err
			} else if suspended {
				continue
			}

			if enable {
				cryptdevices[i].EnableWriteBarrier()
			} else {
				cryptdevices[i].DisableWriteBarrier()
			}
		}
	}

	return nil
}

func main() {
	debug := flag.Bool("debug", false, "do not poweroff the machine on errors")
	flag.Parse()
	debugmode = *debug

	// Ensure initramfs program exists
	assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsPath, "suspend")))

	cryptdevices, err := archLuksSuspend.GetCryptDevices()
	assert(err)

	// Prepare chroot
	defer func() { assert(unbindInitramfs()) }()
	assert(bindInitramfs())

	// Run pre-suspend scripts
	assert(runSystemSuspendScripts("pre"))

	// Stop services that may block suspend
	assert(systemctlServices("stop"))

	// Flush writes before luksSuspend
	syscall.Sync()

	// Disable write barriers on filesystems to avoid IO hangs
	assert(remountDevicesWithWriteBarriers(cryptdevices, disableBarrier))

	// Chroot and hand over execution to initramfs environment

	// The user has unlocked the root device, so now resume all other devices with known keyfiles

	// Re-enable write barriers on filesystems that had them
	assert(remountDevicesWithWriteBarriers(cryptdevices, enableBarrier))

	// Restart stopped services
	assert(systemctlServices("start"))

	// Run post-suspend scripts
	assert(runSystemSuspendScripts("post"))
}
