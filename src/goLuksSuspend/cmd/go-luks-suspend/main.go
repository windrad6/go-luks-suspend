package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	g "goLuksSuspend"
)

var systemdServices = []string{
	// journald may attempt to write to root device
	"syslog.socket",
	"systemd-journald.socket",
	"systemd-journald-dev-log.socket",
	"systemd-journald-audit.socket",
	"systemd-journald.service",
	// udevd often attempts to read from the root device
	"systemd-udevd-control.socket",
	"systemd-udevd-kernel.socket",
	"systemd-udevd.service",
}

const initramfsDir = "/run/initramfs"
const cryptdevicesPath = "/run/initramfs/run/cryptdevices"

func main() {
	g.ParseFlags()

	g.Debug("gathering cryptdevices")
	cryptdevs, err := getcryptdevices()
	g.Assert(err)
	if g.DebugMode {
		g.Debug(fmt.Sprintf("%#v", cryptdevs))
	}

	if len(cryptdevs) == 0 {
		g.IgnoreErrors = true
	}

	g.Debug("running pre-suspend scripts")
	g.Assert(runSystemSuspendScripts("pre"))

	defer func() {
		g.Debug("running post-suspend scripts")
		g.Assert(runSystemSuspendScripts("post"))
	}()

	if len(cryptdevs) == 0 {
		g.Debug("no cryptdevices found, doing normal suspend")
		g.SuspendToRAM()
		return
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
	g.Assert(runInInitramfsChroot(suspendCmdline(g.DebugMode, g.PoweroffOnError)))

	defer func() {
		g.Debug("resuming cryptdevices with keyfiles")
		resumeDevicesWithKeyfiles(cryptdevs)
	}()

	// User has unlocked the root device, so let's be less paranoid
	g.IgnoreErrors = true
}
