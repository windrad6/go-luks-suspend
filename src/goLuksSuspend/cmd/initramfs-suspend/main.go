package main

import (
	"errors"
	"flag"

	g "goLuksSuspend"
)

func main() {
	debugFlag := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	poweroffFlag := flag.Bool("poweroff", false, "power off on failure to unlock root device")
	flag.Parse()
	g.DebugMode = *debugFlag
	poweroffOnUnlockFailure := *poweroffFlag

	if flag.NArg() != 1 {
		g.Assert(errors.New("cryptmounts path unspecified"))
	}

	g.Debug("loading cryptdevice names")
	cryptnames, err := loadCryptnames(flag.Arg(0))
	g.Assert(err)

	g.Debug("suspending cryptdevices")
	suspendCryptDevicesOrPoweroff(cryptnames)

	// Crypt keys have been purged, so be less paranoid
	g.PoweroffOnError = false

	g.Debug("suspending system to RAM")
	g.Assert(g.SuspendToRAM())

loop:
	for {
		g.Debug("resuming root cryptdevice")
		var err error
		for i := 0; i < 3; i++ {
			err = resumeRootCryptDevice(cryptnames[0])
			if err == nil {
				break loop
			}
		}
		if poweroffOnUnlockFailure {
			g.PoweroffOnError = true
			g.Assert(err)
		}
	}
}
