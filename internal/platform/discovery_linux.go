package platform

import (
	"os"
	"path/filepath"
)

func SteamRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, ".local/share/Steam"), filepath.Join(home, ".steam/steam"), filepath.Join(home, ".steam/root"), filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam")}
}
