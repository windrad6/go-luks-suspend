package suspend

import (
	"reflect"
	"testing"
)

func TestScanCrypttab(t *testing.T) {
	cds, err := scanCrypttab("test_crypttab.txt")

	if err != nil {
		t.Errorf("unexpected error: %#v", err)
	}

	expected := []CryptDevice{
		{DMName: "crypt1", Keyfile: "/path/to/keyfile1"},
		{DMName: "crypt2", Keyfile: "/path/to/keyfile2"},
	}

	if !reflect.DeepEqual(cds, expected) {
		t.Errorf("%#v != %#v", cds, expected)
	}
}
