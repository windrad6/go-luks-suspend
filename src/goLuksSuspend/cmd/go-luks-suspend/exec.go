package main

import (
	"fmt"
	"goLuksSuspend"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

const systemSleepDir = "/usr/lib/systemd/system-sleep"

var bindDirs = []string{"/sys", "/proc", "/dev", "/run"}

func checkRootOwnedAndExecutablePath(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	return checkRootOwnedAndExecutable(fi)
}

func checkRootOwnedAndExecutable(fi os.FileInfo) error {
	mode := fi.Mode()

	switch {
	case !mode.IsRegular():
		return fmt.Errorf("%s is not a regular file", fi.Name())
	case mode&0022 != 0:
		return fmt.Errorf("%s is writable by group or world", fi.Name())
	case mode&0111 == 0:
		return fmt.Errorf("%s is not executable", fi.Name())
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s stat_t missing", fi.Name())
	} else if stat.Uid != 0 {
		return fmt.Errorf("%s is not owned by root", fi.Name())
	}

	return nil
}

func unbindInitramfs() error {
	for _, dir := range bindDirs {
		d := filepath.Join(initramfsDir, dir)
		err := syscall.Unmount(d, 0)
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
			warn(err.Error())
			continue
		}

		err := goLuksSuspend.Run(
			nil,
			[]string{filepath.Join(systemSleepDir, fs[i].Name()), scriptarg, "suspend"},
			false,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

var systemctlPath = "/usr/bin/systemctl"

func stopSystemServices(services []string) (stoppedServices []string, err error) {
	for _, s := range services {
		if goLuksSuspend.Run(nil, []string{systemctlPath, "--quiet", "is-active", s}, false) == nil {
			err := goLuksSuspend.Run(nil, []string{systemctlPath, "stop", s}, false)
			if err != nil {
				return stoppedServices, err
			}
			stoppedServices = append(stoppedServices, s)
		}
	}

	return stoppedServices, nil
}

func startSystemServices(services []string) error {
	return goLuksSuspend.Run(nil, append([]string{systemctlPath, "start"}, services...), false)
}

func disableWriteBarriers(filesystems []filesystem) error {
	for i := range filesystems {
		if err := filesystems[i].disableWriteBarrier(); err != nil {
			return err
		}
	}
	return nil
}

func enableWriteBarriers(filesystems []filesystem) {
	for i := range filesystems {
		// The underlying device may have disappeared
		if !filesystems[i].isMounted() {
			warn("[WARNING] missing filesystem mounted at " + filesystems[i].mountpoint)
			continue
		}
		if err := filesystems[i].enableWriteBarrier(); err != nil {
			warn(fmt.Sprintf(
				"[WARNING] mount -o remount,barrier %s: %s",
				filesystems[i].mountpoint,
				err.Error(),
			))
		}
	}
}

func runInInitramfsChroot(cmdline []string) error {
	chroot := make([]string, 0, len(cmdline)+2)
	chroot = append(chroot, "/usr/bin/chroot", initramfsDir)
	chroot = append(chroot, cmdline...)
	return goLuksSuspend.Run([]string{}, chroot, true)
}

func resumeDevicesWithKeyfiles(cryptdevs []cryptdevice) {
	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	ch := make(chan *cryptdevice)

	wg.Add(1)
	go func() {
		for i := range cryptdevs {
			ch <- &cryptdevs[i]
		}
		close(ch)
		wg.Done()
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for cd := range ch {
				if !cd.suspended() {
					continue
				} else if !cd.exists() {
					warn("[WARNING] missing cryptdevice " + cd.name)
					continue
				} else if len(cd.keyfile) == 0 {
					warn(fmt.Sprintf("[WARNING] no keyfile for cryptdevice %s; skipping", cd.name))
					continue
				}

				warn("Resuming " + cd.name)

				err := cd.resumeWithKeyfile()
				if err != nil {
					warn(fmt.Sprintf("[ERROR] failed to resume %s: %s", cd.name, err.Error()))
				} else {
					warn(cd.name + " resumed")
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()
}
