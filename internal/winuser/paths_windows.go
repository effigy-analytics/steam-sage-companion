// Package winuser resolves per-user Windows paths and enforces private ownership.
package winuser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type Layout struct {
	InstallDir, Executable, Uninstaller, Ledger               string
	StartMenuDir, LaunchShortcut, SettingsShortcut, ConfigDir string
}

var knownFolderPath = windows.KnownFolderPath

// CanonicalDir resolves existing path aliases without following reparse points.
// Missing suffixes are permitted so callers can preflight destinations before creation.
func CanonicalDir(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return "", errors.New("invalid Windows directory")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(strings.TrimPrefix(abs, filepath.VolumeName(abs)), `:*?"<>|`) {
		return "", errors.New("invalid Windows directory")
	}
	// Win32 strips trailing ASCII dots/spaces. Reject these spellings before
	// probing or hashing a missing suffix, so creation cannot change identity.
	for _, component := range strings.Split(strings.TrimPrefix(abs, filepath.VolumeName(abs)), string(filepath.Separator)) {
		if strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return "", errors.New("ambiguous Windows directory component")
		}
	}
	var missing []string
	existing := ""
	for part := abs; ; part = filepath.Dir(part) {
		p, err := windows.UTF16PtrFromString(part)
		if err != nil {
			return "", err
		}
		attrs, err := windows.GetFileAttributes(p)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", err
			}
			if existing == "" {
				missing = append(missing, filepath.Base(part))
			}
		} else {
			if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
				return "", errors.New("directory is a reparse point or wrong type")
			}
			if existing == "" {
				existing = part
			}
		}
		if filepath.Dir(part) == part {
			break
		}
	}
	if existing == "" {
		return "", errors.New("directory volume is unavailable")
	}
	p, err := windows.UTF16PtrFromString(existing)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	// Use the filesystem's spelling for existing names, including short-name aliases.
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buf)) {
		return "", errors.New("directory path is too long")
	}
	result := windows.UTF16ToString(buf[:n])
	if strings.HasPrefix(result, `\\?\UNC\`) {
		result = `\\` + strings.TrimPrefix(result, `\\?\UNC\`)
	} else {
		result = strings.TrimPrefix(result, `\\?\`)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		result = filepath.Join(result, missing[i])
	}
	return filepath.Clean(result), nil
}

// LayoutFor computes fixed product destinations independently of package ledgers.
func LayoutFor(configDir string) (Layout, error) {
	local, err := knownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return Layout{}, err
	}
	programs, err := knownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return Layout{}, err
	}
	if configDir == "" {
		base, err := knownFolderPath(windows.FOLDERID_RoamingAppData, 0)
		if err != nil {
			return Layout{}, err
		}
		configDir = filepath.Join(base, "steam-sage-companion")
	}
	install, err := CanonicalDir(filepath.Join(local, "Programs", "Steam Sage Companion"))
	if err != nil {
		return Layout{}, err
	}
	menu, err := CanonicalDir(filepath.Join(programs, "Steam Sage Companion"))
	if err != nil {
		return Layout{}, err
	}
	config, err := CanonicalDir(configDir)
	if err != nil {
		return Layout{}, err
	}
	return Layout{InstallDir: install, Executable: filepath.Join(install, "steam-sage-companion.exe"), Uninstaller: filepath.Join(install, "Uninstall.exe"), Ledger: filepath.Join(install, "package.ini"), StartMenuDir: menu, LaunchShortcut: filepath.Join(menu, "Steam Sage Companion.lnk"), SettingsShortcut: filepath.Join(menu, "Companion Settings.lnk"), ConfigDir: config}, nil
}
