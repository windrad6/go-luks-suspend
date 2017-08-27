package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"runtime"
	"strings"
	"sync"

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

// loadCryptnames loads the names written to a path by Dump
func loadCryptnames(path string) ([]string, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(buf), "\x00"), nil
}

func suspendToRAM() error {
	return ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
}

func suspendCryptDevicesOrPoweroff(deviceNames []string) {
	n := runtime.NumCPU()
	wg := sync.WaitGroup{}
	ch := make(chan string)

	wg.Add(1)
	go func() {
		for i := range deviceNames {
			ch <- deviceNames[i]
		}
		close(ch)
		wg.Done()
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for name := range ch {
				debug("suspending " + name)
				assert(goLuksSuspend.Run(
					nil,
					[]string{"/usr/bin/cryptsetup", "luksSuspend", name},
					false,
				))
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func luksResume(device string) error {
	return goLuksSuspend.Run(
		nil,
		[]string{"/usr/bin/cryptsetup", "luksResume", device},
		true,
	)
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
