package platform

import (
	"os"
	"path/filepath"
	"strings"

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
	seen := map[string]bool{}
	add := func(path string) {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return
		}
		path = filepath.Clean(path)
		identity := strings.ToLower(path)
		if !seen[identity] {
			seen[identity] = true
			roots = append(roots, path)
		}
	}
	for _, location := range []struct {
		root       registry.Key
		path, name string
		view       uint32
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath", registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath", registry.WOW64_32KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath", registry.WOW64_64KEY},
	} {
		if value, err := readSteamRegistry(location.root, location.path, location.name, registry.QUERY_VALUE|location.view); err == nil {
			add(value)
		}
	}
	for _, name := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := steamEnvironment(name); base != "" {
			add(filepath.Join(base, "Steam"))
		}
	}
	return roots
}
