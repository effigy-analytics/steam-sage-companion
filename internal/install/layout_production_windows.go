//go:build windows && !packageqa

package install

import "github.com/effigy-analytics/steam-sage-companion/internal/winuser"

func resolvePackageLayout(configDir string) (winuser.Layout, error) {
	return winuser.LayoutFor(configDir)
}

var uninstallRegistry = `Software\Microsoft\Windows\CurrentVersion\Uninstall\SteamSageCompanion`
var runRegistry = `Software\Microsoft\Windows\CurrentVersion\Run`
