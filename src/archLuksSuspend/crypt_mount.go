package archLuksSuspend

type CryptMount struct {
	Name       string
	Mountpoint string
}

func CryptMounts(cds []CryptDevice) []CryptMount {
	cms := make([]CryptMount, len(cds))

	for i := range cds {
		cms[i] = CryptMount{Name: cds[i].Name, Mountpoint: cds[i].Mountpoint}
	}

	return cms
}
