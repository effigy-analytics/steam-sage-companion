package install

import (
	"errors"
)

func UserPaths(configDir string) (Paths, error) {
	l, err := packageLayout(configDir)
	return Paths{Executable: l.Executable, Desktop: l.LaunchShortcut, ConfigDir: l.ConfigDir}, err
}
func Install(string, Paths) error {
	return errors.New("Use Steam Sage Companion Setup.exe to install or update this Windows package")
}
func Uninstall(Paths, bool) (bool, error) {
	return true, errors.New("Use Windows Settings > Apps > Steam Sage Companion to uninstall; settings are preserved")
}
func StartupExecutable(current string, paths Paths) string {
	l, err := packageLayout(paths.ConfigDir)
	if err != nil {
		return current
	}
	record, err := loadLedger(l)
	if err != nil || record == nil {
		return current
	}
	hash, err := managedHash(l.Executable)
	if err == nil && record["exe"]["sha256"] == hash {
		return l.Executable
	}
	return current
}

// Kept behind an adapter so unit fixtures never touch actual known folders.
var packageLayout = resolvePackageLayout
