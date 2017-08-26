package main

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestKernelCmdlineParsing(t *testing.T) {
	path := "test_kernel_cmdline"
	defer os.Remove(path) // errcheck: rm -f

	data := []struct {
		in, out string
	}{
		{
			in:  "cryptdevice=UUID=d55cc35b-e99b-44ce-be89-4c573fccfb0b:cryptroot root=/dev/mapper/cryptroot\n",
			out: "cryptroot",
		},
		{
			in:  "cryptdevice=/dev/sda1:cryptroot1 cryptdevice=/dev/sda2:cryptroot2\n",
			out: "cryptroot2",
		},
		{
			in:  "cryptdevice=UUID=cd5dd4dc-5766-493e-b3c6-3d6dfd195082:cryptolvm:allow-discards root=/dev/mapper/system-root",
			out: "cryptolvm",
		},
	}

	for _, row := range data {
		err := ioutil.WriteFile(path, []byte(row.in), 0644)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		dev, err := getCryptdeviceFromKernelCmdline(path)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
		if dev != row.out {
			t.Errorf("%#v != %#v", dev, row.out)
		}
	}
}
