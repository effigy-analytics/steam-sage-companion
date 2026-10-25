package winuser

import (
	"errors"
	"golang.org/x/sys/windows"
)

type Layout struct{ InstallDir, Executable, Uninstaller, Ledger, StartMenuDir, LaunchShortcut, SettingsShortcut, ConfigDir string }

var knownFolderPath = windows.KnownFolderPath

func CanonicalDir(path string) (string, error) {
	return "", errors.New("canonical paths are not implemented")
}
func LayoutFor(configDir string) (Layout, error) {
	return Layout{}, errors.New("layout is not implemented")
}
