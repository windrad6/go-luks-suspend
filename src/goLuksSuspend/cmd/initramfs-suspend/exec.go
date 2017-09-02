package main

import (
	"goLuksSuspend"
	"io/ioutil"
	"runtime"
	"strings"
	"sync"
)

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
