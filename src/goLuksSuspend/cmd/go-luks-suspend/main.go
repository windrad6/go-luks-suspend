package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"goLuksSuspend"
)

var debugMode = false
var poweroffOnError = true
var systemdServices = []string{
	// journald may attempt to write to the suspended device
	"syslog.socket",
	"systemd-journald.socket",
	"systemd-journald-dev-log.socket",
	"systemd-journald-audit.socket",
	"systemd-journald.service",
}

func debug(msg string) {
	if debugMode {
		warn(msg)
	}
}

func warn(msg string) {
	log.Println(msg)
}

func assert(err error) {
	if err != nil {
		warn(err.Error())
		if debugMode {
			goLuksSuspend.DebugShell()
		} else if poweroffOnError {
			goLuksSuspend.Poweroff()
		}
	}
}

const initramfsDir = "/run/initramfs"
const cryptdevicesPath = "/run/initramfs/run/cryptdevices"

func main() {
	debugFlag := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	poweroffFlag := flag.Bool("poweroff", false, "power off on failure to unlock root device")
	flag.Parse()
	debugMode = *debugFlag
	poweroffOnUnlockFailure := *poweroffFlag

	debug("gathering cryptdevices")
	cryptdevs, err := getcryptdevices()
	assert(err)
	if debugMode {
		debug(fmt.Sprintf("%#v", cryptdevs))
	}

	debug("gathering filesystems with write barriers")
	filesystems, err := getFilesystemsWithWriteBarriers()
	assert(err)
	if debugMode {
		debug(fmt.Sprintf("%#v", filesystems))
	}

	debug("checking for suspend program in initramfs")
	assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsDir, "suspend")))

	debug("preparing initramfs chroot")
	assert(bindInitramfs())

	defer func() {
		debug("unmounting initramfs bind mounts")
		assert(unbindInitramfs())
	}()

	debug("running pre-suspend scripts")
	assert(runSystemSuspendScripts("pre"))

	defer func() {
		debug("running post-suspend scripts")
		assert(runSystemSuspendScripts("post"))
	}()

	debug("stopping selected system services")
	services, err := stopSystemServices(systemdServices)
	assert(err)
	debug("stopped " + strings.Join(services, ", "))

	defer func() {
		debug("starting previously stopped system services")
		assert(startSystemServices(services))
	}()

	debug("flushing pending writes")
	syscall.Sync()

	debug("disabling write barriers on filesystems to avoid IO hangs")
	assert(disableWriteBarriers(filesystems))

	defer func() {
		debug("re-enabling write barriers on filesystems")
		enableWriteBarriers(filesystems)
	}()

	debug("dumping list of cryptdevice names to initramfs")
	assert(dumpCryptdevices(cryptdevicesPath, cryptdevs))
	if debugMode {
		buf, _ := ioutil.ReadFile(cryptdevicesPath) // errcheck: debugmode only
		debug(fmt.Sprintf("%s: %#v", cryptdevicesPath, string(buf)))
	}

	defer func() {
		debug("removing cryptdevice dump file")
		assert(os.Remove(cryptdevicesPath))
	}()

	debug("calling suspend in initramfs chroot")
	assert(runInInitramfsChroot(suspendCmdline(debugMode, poweroffOnUnlockFailure)))

	defer func() {
		debug("resuming cryptdevices with keyfiles")
		resumeDevicesWithKeyfiles(cryptdevs)
	}()

	// User has unlocked the root device, so let's be less paranoid
	poweroffOnError = false
}
