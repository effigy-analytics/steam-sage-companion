package install

import "errors"

func UserPaths(string) (Paths, error) {
	return Paths{}, errors.New("Windows installation is experimental and not yet verified")
}
func desktopEntry(Paths) []byte { return nil }
