package platform

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func SteamRoots() []string {
	var roots []string
	for _, location := range []struct {
		root       registry.Key
		path, name string
	}{{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"}, {registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"}, {registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"}} {
		key, err := registry.OpenKey(location.root, location.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetStringValue(location.name)
		key.Close()
		if err == nil && value != "" {
			roots = append(roots, value)
		}
	}
	for _, name := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := os.Getenv(name); base != "" {
			roots = append(roots, filepath.Join(base, "Steam"))
		}
	}
	return roots
}
