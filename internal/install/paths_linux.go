package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
)

func UserPaths(configDir string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	if strings.ContainsAny(home+data+configDir, "\r\n") || !filepath.IsAbs(data) || !filepath.IsAbs(configDir) {
		return Paths{}, errors.New("installation needs absolute per-user paths")
	}
	return Paths{Executable: filepath.Join(home, ".local", "bin", "steam-sage-companion"), Desktop: filepath.Join(data, "applications", "steam-sage-companion.desktop"), ConfigDir: configDir}, nil
}
func desktopEntry(paths Paths) []byte {
	return []byte("# Managed by Steam Sage Companion\n[Desktop Entry]\nType=Application\nName=Steam Sage Companion\nComment=Connect Steam collections to Sage\nExec=" + platform.DesktopExec(paths.Executable, "open", "--config-dir", paths.ConfigDir) + "\nTerminal=false\nCategories=Game;Utility;\n")
}
