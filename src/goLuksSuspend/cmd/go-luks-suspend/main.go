package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"goLuksSuspend"
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
		log.Println(err)
		goLuksSuspend.Poweroff()
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
			log.Println(err)
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
	return goLuksSuspend.Run(
		nil,
		append([]string{"/usr/bin/systemctl", command}, systemdServices...),
		false,
	)
}

func disableWriteBarriers(filesystems []goLuksSuspend.Filesystem) error {
	for i := range filesystems {
		if err := filesystems[i].DisableWriteBarrier(); err != nil {
			return err
		}
	}
	return nil
}

func enableWriteBarriers(filesystems []goLuksSuspend.Filesystem) error {
	for i := range filesystems {
		// The underlying device may have disappeared
		if err := filesystems[i].EnableWriteBarrier(); err != nil {
			return err
		}
	}
	return nil
}

func runInInitramfsChroot(cmdline []string) error {
	chroot := make([]string, 0, len(cmdline)+2)
	chroot = append(chroot, "/usr/bin/chroot", initramfsDir)
	chroot = append(chroot, cmdline...)
	return goLuksSuspend.Run([]string{}, chroot, true)
}

func resumeDevicesWithKeyfilesOrPoweroff(cryptdevices []goLuksSuspend.CryptDevice) {
	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	ch := make(chan *goLuksSuspend.CryptDevice)

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
				if ok, err := cd.CanResumeWithKeyfile(); err != nil {
					assert(err)
				} else if !ok {
					continue
				}

				fmt.Fprintln(os.Stderr, "Resuming "+cd.Name)
				assert(cd.LuksResumeWithKeyfile())
				fmt.Fprintln(os.Stderr, cd.Name+" resumed")
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func main() {
	debug := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	flag.Parse()
	goLuksSuspend.DebugMode = *debug
	l := goLuksSuspend.Log

	l("checking for suspend program in initramfs")
	assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsDir, "suspend")))

	l("gathering cryptdevices")
	cryptdevices, err := goLuksSuspend.GetCryptDevices()
	assert(err)
	if goLuksSuspend.DebugMode {
		log.Printf("%#v\n", cryptdevices)
	}

	l("gathering filesystems with write barriers")
	filesystems, err := goLuksSuspend.GetFilesystemsWithWriteBarriers()
	assert(err)
	if goLuksSuspend.DebugMode {
		log.Printf("%#v\n", filesystems)
	}

	defer func() {
		l("unmounting initramfs bind mounts")
		assert(unbindInitramfs())
	}()

	l("preparing initramfs chroot")
	assert(bindInitramfs())

	l("running pre-suspend scripts")
	assert(runSystemSuspendScripts("pre"))

	l("stopping selected systemd services")
	assert(systemctlServices("stop"))

	l("flushing pending writes")
	syscall.Sync()

	l("disabling write barriers on filesystems to avoid IO hangs")
	assert(disableWriteBarriers(filesystems))

	l("dumping list of cryptdevice names to initramfs")
	assert(goLuksSuspend.Dump(cryptdevicesPath, cryptdevices))
	if goLuksSuspend.DebugMode {
		buf, _ := ioutil.ReadFile(cryptdevicesPath) // errcheck: debugmode only
		log.Printf("%s: %#v\n", cryptdevicesPath, string(buf))
	}

	l("calling suspend in initramfs chroot")
	args := []string{"/suspend"}
	if goLuksSuspend.DebugMode {
		args = append(args, "-debug")
	}
	args = append(args, filepath.Join("run", filepath.Base(cryptdevicesPath)))
	assert(runInInitramfsChroot(args))

	// At this point the user has unlocked the root device, so avoid
	// powering off the machine from here.
	goLuksSuspend.PoweroffOnError = false

	l("removing cryptdevice dump file")
	assert(os.Remove(cryptdevicesPath))

	l("resuming cryptdevices with keyfiles")
	resumeDevicesWithKeyfilesOrPoweroff(cryptdevices)

	l("re-enabling write barriers on filesystems")
	assert(enableWriteBarriers(filesystems))

	l("starting previously stopped systemd services")
	assert(systemctlServices("start"))

	l("running post-suspend scripts")
	assert(runSystemSuspendScripts("post"))
}
