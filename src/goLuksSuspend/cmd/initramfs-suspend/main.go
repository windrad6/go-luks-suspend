package main

import (
	"errors"
	"flag"
	"log"
	"runtime"
	"sync"

	"goLuksSuspend"
)

func assert(err error) {
	if err != nil {
		log.Println(err)
		goLuksSuspend.Poweroff()
	}
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
				goLuksSuspend.Log("suspending " + name)
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
	debug := flag.Bool("debug", false, "do not poweroff the machine on errors")
	flag.Parse()
	goLuksSuspend.DebugMode = *debug
	l := goLuksSuspend.Log

	if flag.NArg() != 1 {
		assert(errors.New("cryptmounts path unspecified"))
	}

	l("loading cryptdevice names")
	deviceNames, err := goLuksSuspend.Load(flag.Arg(0))
	assert(err)

	l("suspending cryptdevices")
	suspendCryptDevicesOrPoweroff(deviceNames)

	if goLuksSuspend.DebugMode {
		l("debug mode: skipping suspend to RAM")
	} else {
		l("suspending system to RAM")
		assert(goLuksSuspend.SuspendToRAM())
	}

	l("resuming root cryptdevice")
	assert(luksResume(deviceNames[0]))
}
