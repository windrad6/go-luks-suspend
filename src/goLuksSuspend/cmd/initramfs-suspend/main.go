package main

import (
	"errors"
	"flag"
	"log"

	"goLuksSuspend"
)

var debugMode = false

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
		goLuksSuspend.Poweroff(debugMode)
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

	if debugMode {
		debug("debug mode: skipping suspend to RAM")
	} else {
		debug("suspending system to RAM")
		assert(suspendToRAM())
	}

	debug("resuming root cryptdevice")
	for {
		err := luksResume(cryptnames[0])
		if err == nil {
			break
		} else if poweroffOnUnlockFailure {
			assert(err)
		}
	}
}
