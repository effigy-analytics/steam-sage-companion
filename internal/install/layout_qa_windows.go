//go:build windows && packageqa

// This adapter exists only in explicitly labeled QA builds. Production binaries
// contain no environment variable, flag, or linker variable for redirecting roots.
package install

import (
	"errors"
	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"path/filepath"
)

var qaRoot string
var qaMenuRoot string
var uninstallRegistry = `Software\SteamSageCompanionQA\` + filepath.Base(qaRoot) + `\Uninstall`
var runRegistry = `Software\SteamSageCompanionQA\` + filepath.Base(qaRoot) + `\Run`

func resolvePackageLayout(string) (winuser.Layout, error) {
	if !filepath.IsAbs(qaRoot) {
		return winuser.Layout{}, errors.New("QA build requires fixed absolute fixture root")
	}
	root, err := winuser.CanonicalDir(qaRoot)
	if err != nil {
		return winuser.Layout{}, err
	}
	install := filepath.Join(root, "Programs", "Steam Sage Companion")
	menu := filepath.Join(root, "Start Menu", "Steam Sage Companion")
	if qaMenuRoot != "" {
		if !filepath.IsAbs(qaMenuRoot) {
			return winuser.Layout{}, errors.New("QA menu root must be absolute")
		}
		menu = filepath.Join(qaMenuRoot, "Steam Sage Companion")
	}
	return winuser.Layout{InstallDir: install, Executable: filepath.Join(install, "steam-sage-companion.exe"), Uninstaller: filepath.Join(install, "Uninstall.exe"), Ledger: filepath.Join(install, "package.ini"), StartMenuDir: menu, LaunchShortcut: filepath.Join(menu, "Steam Sage Companion.lnk"), SettingsShortcut: filepath.Join(menu, "Companion Settings.lnk"), ConfigDir: filepath.Join(root, "Config")}, nil
}
