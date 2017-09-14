package main

import (
	"errors"
	"flag"

	g "goLuksSuspend"
)

func main() {
	g.ParseFlags()

	if flag.NArg() != 1 {
		g.Assert(errors.New("cryptmounts path unspecified"))
	}

	g.Debug("loading cryptdevice names")
	cryptnames, err := loadCryptnames(flag.Arg(0))
	g.Assert(err)

	g.Debug("suspending cryptdevices")
	g.Assert(suspendCryptDevices(cryptnames))

	// Crypt keys have been purged, so be less paranoid
	g.IgnoreErrors = true

	// Shorten task freeze timeout
	oldtimeout, err := g.SetFreezeTimeout([]byte{'1', '0', '0', '0'})
	if err == nil {
		defer func() {
			_, err := g.SetFreezeTimeout(oldtimeout)
			g.Assert(err)
		}()
	} else {
		g.Assert(err)
	}

	if g.DebugMode {
		g.Debug("debug: skipping suspend to RAM")
	} else {
		g.Debug("suspending system to RAM")
		g.Assert(g.SuspendToRAM())
	}

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
		// The -poweroff flag indicates the user's desire to take the
		// system offline on failure to unlock.
		if g.PoweroffOnError {
			g.IgnoreErrors = false
			g.Assert(err)
		}
	}
}
