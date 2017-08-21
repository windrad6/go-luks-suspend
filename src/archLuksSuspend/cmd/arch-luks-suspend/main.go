package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"archLuksSuspend"
)

const initramfsDir = "/run/initramfs"
const cryptdevicesPath = "/run/initramfs/run/cryptdevices"
const systemSleepDir = "/usr/lib/systemd/system-sleep"

var bindDirs = []string{"/sys", "/proc", "/dev", "/run"}
var systemdServices = []string{
	// journald may attempt to write to the suspended device
	"systemd-journald-dev-log.socket",
	"systemd-journald.socket",
	"systemd-journald.service",
}

func assert(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		archLuksSuspend.Poweroff()
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
	dir, err := os.Open(systemSleepDir)
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
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		err := exec.Command(filepath.Join(systemSleepDir, fs[i].Name()), scriptarg, "suspend").Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func bindInitramfs() error {
	for _, dir := range bindDirs {
		err := syscall.Mount(dir, filepath.Join(initramfsDir, dir), "", syscall.MS_BIND, "")
		if err != nil {
			return err
		}
	}
	return nil
}

func unbindInitramfs() error {
	for _, dir := range bindDirs {
		err := syscall.Unmount(filepath.Join(initramfsDir, dir), 0)
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

			var err error

			if enable {
				err = cryptdevices[i].EnableWriteBarrier()
			} else {
				err = cryptdevices[i].DisableWriteBarrier()
			}

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func chrootAndRun(newroot string, cmdline ...string) error {
	args := make([]string, 0, len(cmdline)+1)
	args = append(args, newroot)
	args = append(args, cmdline...)
	return exec.Command("/usr/bin/chroot", args...).Run()
}

func resumeDevicesWithKeyfilesOrPoweroff(cryptdevices []archLuksSuspend.CryptDevice) {
	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	ch := make(chan *archLuksSuspend.CryptDevice)

	wg.Add(1)
	go func() {
		for i := range cryptdevices {
			if len(cryptdevices[i].Keyfile) > 0 {
				ch <- &cryptdevices[i]
			}
		}
		close(ch)
		wg.Done()
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for cd := range ch {
				fmt.Fprintln(os.Stderr, "Resuming "+cd.Name)
				assert(cd.LuksResume())
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func main() {
	debug := flag.Bool("debug", false, "do not poweroff the machine on errors")
	flag.Parse()
	archLuksSuspend.DebugMode = *debug

	// Ensure suspend program exists in initramfs
	assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsDir, "suspend")))

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

	// Dump devices to be suspended
	assert(archLuksSuspend.Dump(cryptdevicesPath, cryptdevices))

	// Hand over execution to program in initramfs environment
	args := []string{"/suspend"}
	if archLuksSuspend.DebugMode {
		args = append(args, "-debug")
	}
	args = append(args, filepath.Join("run", filepath.Base(cryptdevicesPath)))
	assert(chrootAndRun(initramfsDir, args...))

	// Clean up
	assert(os.Remove(cryptdevicesPath))

	// The user has unlocked the root device, so resume all other devices with keyfiles
	resumeDevicesWithKeyfilesOrPoweroff(cryptdevices)

	// Re-enable write barriers on filesystems that had them
	assert(remountDevicesWithWriteBarriers(cryptdevices, enableBarrier))

	// Restart stopped services
	assert(systemctlServices("start"))

	// Run post-suspend scripts
	assert(runSystemSuspendScripts("post"))
}
