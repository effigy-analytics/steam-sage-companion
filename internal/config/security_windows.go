package config

import (
	"os"
	"path/filepath"

	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"golang.org/x/sys/windows"
)

func ensurePrivateDir(dir string) error { return winuser.EnsurePrivateDir(dir) }
func validatePrivateLoad(dir, path string) error {
	if _, err := winuser.CanonicalDir(dir); err != nil {
		return err
	}
	if err := winuser.ValidatePrivateDir(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := winuser.ValidatePrivateFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func protectPrivateFile(file *os.File) error { return winuser.ProtectNewFile(file) }
func replacePrivateFile(temp, dst string) error {
	if err := validatePrivateLoad(filepath.Dir(dst), dst); err != nil {
		return err
	}
	if err := winuser.ValidatePrivateFile(temp); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	// Same-directory rename keeps the protected source DACL. Never delete the old
	// settings first, and do not fall back to a cross-volume copy/delete operation.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
