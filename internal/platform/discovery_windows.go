package platform

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

var readSteamRegistry = func(root registry.Key, path, name string, access uint32) (string, error) {
	key, err := registry.OpenKey(root, path, access)
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	return value, err
}
var steamEnvironment = os.Getenv

func SteamRoots() []string {
	var roots []string
	for _, location := range []struct {
		root       registry.Key
		path, name string
	}{{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"}, {registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"}, {registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"}} {
		value, err := readSteamRegistry(location.root, location.path, location.name, registry.QUERY_VALUE)
		if err == nil && value != "" {
			roots = append(roots, value)
		}
	}
	for _, name := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := steamEnvironment(name); base != "" {
			roots = append(roots, filepath.Join(base, "Steam"))
		}
	}
	return roots
}
