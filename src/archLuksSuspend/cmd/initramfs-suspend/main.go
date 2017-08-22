package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"archLuksSuspend"
)

func assert(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		archLuksSuspend.Poweroff()
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
				archLuksSuspend.Log("suspending " + name)
				assert(exec.Command("/usr/bin/cryptsetup", "luksSuspend", name).Run())
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func luksResume(device string) error {
	cmd := exec.Command("/usr/bin/cryptsetup", "luksResume", device)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	debug := flag.Bool("debug", false, "do not poweroff the machine on errors")
	flag.Parse()
	archLuksSuspend.DebugMode = *debug
	l := archLuksSuspend.Log

	if flag.NArg() != 1 {
		assert(errors.New("cryptmounts path unspecified"))
	}

	l("loading cryptdevice names")
	deviceNames, err := archLuksSuspend.Load(flag.Arg(0))
	assert(err)

	l("suspending cryptdevices")
	suspendCryptDevicesOrPoweroff(deviceNames)

	l("suspending system to RAM")
	assert(archLuksSuspend.SuspendToRAM())

	l("resuming root cryptdevice")
	assert(luksResume(deviceNames[0]))
}
