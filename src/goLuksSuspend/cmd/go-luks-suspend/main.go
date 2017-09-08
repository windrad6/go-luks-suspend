package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	g "goLuksSuspend"
)

var systemdServices = []string{
	// journald may attempt to write to the suspended device
	"syslog.socket",
	"systemd-journald.socket",
	"systemd-journald-dev-log.socket",
	"systemd-journald-audit.socket",
	"systemd-journald.service",
}

const initramfsDir = "/run/initramfs"
const cryptdevicesPath = "/run/initramfs/run/cryptdevices"

func main() {
	debugFlag := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	poweroffFlag := flag.Bool("poweroff", false, "power off on failure to unlock root device")
	flag.Parse()
	g.DebugMode = *debugFlag
	poweroffOnUnlockFailure := *poweroffFlag

	g.Debug("gathering cryptdevices")
	cryptdevs, err := getcryptdevices()
	g.Assert(err)
	if g.DebugMode {
		g.Debug(fmt.Sprintf("%#v", cryptdevs))
	}

	g.Debug("gathering filesystems with write barriers")
	filesystems, err := getFilesystemsWithWriteBarriers()
	g.Assert(err)
	if g.DebugMode {
		g.Debug(fmt.Sprintf("%#v", filesystems))
	}

	g.Debug("checking for suspend program in initramfs")
	g.Assert(checkRootOwnedAndExecutablePath(filepath.Join(initramfsDir, "suspend")))

	g.Debug("preparing initramfs chroot")
	g.Assert(bindInitramfs())

	defer func() {
		g.Debug("unmounting initramfs bind mounts")
		g.Assert(unbindInitramfs())
	}()

	g.Debug("running pre-suspend scripts")
	g.Assert(runSystemSuspendScripts("pre"))

	defer func() {
		g.Debug("running post-suspend scripts")
		g.Assert(runSystemSuspendScripts("post"))
	}()

	g.Debug("stopping selected system services")
	services, err := stopSystemServices(systemdServices)
	g.Assert(err)
	g.Debug("stopped " + strings.Join(services, ", "))

	defer func() {
		g.Debug("starting previously stopped system services")
		g.Assert(startSystemServices(services))
	}()

	g.Debug("flushing pending writes")
	syscall.Sync()

	g.Debug("disabling write barriers on filesystems to avoid IO hangs")
	g.Assert(disableWriteBarriers(filesystems))

	defer func() {
		g.Debug("re-enabling write barriers on filesystems")
		enableWriteBarriers(filesystems)
	}()

	g.Debug("dumping list of cryptdevice names to initramfs")
	g.Assert(dumpCryptdevices(cryptdevicesPath, cryptdevs))
	if g.DebugMode {
		buf, _ := ioutil.ReadFile(cryptdevicesPath) // errcheck: debugmode only
		g.Debug(fmt.Sprintf("%s: %#v", cryptdevicesPath, string(buf)))
	}

	defer func() {
		g.Debug("removing cryptdevice dump file")
		g.Assert(os.Remove(cryptdevicesPath))
	}()

	g.Debug("calling suspend in initramfs chroot")
	g.Assert(runInInitramfsChroot(suspendCmdline(g.DebugMode, poweroffOnUnlockFailure)))

	defer func() {
		g.Debug("resuming cryptdevices with keyfiles")
		resumeDevicesWithKeyfiles(cryptdevs)
	}()

	// User has unlocked the root device, so let's be less paranoid
	g.PoweroffOnError = false
}
