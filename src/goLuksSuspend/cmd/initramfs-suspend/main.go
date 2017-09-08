package main

import (
	"errors"
	"flag"
	"log"

	"goLuksSuspend"
)

var debugMode = false
var poweroffOnError = true

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

func main() {
	debugFlag := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	poweroffFlag := flag.Bool("poweroff", false, "power off on failure to unlock root device")
	flag.Parse()
	debugMode = *debugFlag
	poweroffOnUnlockFailure := *poweroffFlag

	if flag.NArg() != 1 {
		assert(errors.New("cryptmounts path unspecified"))
	}

	debug("loading cryptdevice names")
	cryptnames, err := loadCryptnames(flag.Arg(0))
	assert(err)

	debug("suspending cryptdevices")
	suspendCryptDevicesOrPoweroff(cryptnames)

	// Crypt keys have been purged, so be less paranoid
	poweroffOnError = false

	debug("suspending system to RAM")
	assert(suspendToRAM())

loop:
	for {
		debug("resuming root cryptdevice")
		var err error
		for i := 0; i < 3; i++ {
			err = resumeRootCryptDevice(cryptnames[0])
			if err == nil {
				break loop
			}
		}
		if poweroffOnUnlockFailure {
			poweroffOnError = true
			assert(err)
		}
	}
}
