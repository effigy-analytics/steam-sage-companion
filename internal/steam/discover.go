// Package steam reads local library organization without changing Steam.
package steam

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

const steamBase uint64 = 76561197960265728

type Account struct {
	SteamID string
	Root    string
}

func Discover(roots []string) ([]Account, error) {
	result := []Account{}
	seen := map[string]bool{}
	for _, root := range roots {
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil || seen[canonical] {
			continue
		}
		seen[canonical] = true
		entries, err := os.ReadDir(filepath.Join(canonical, "userdata"))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			id, err := strconv.ParseUint(entry.Name(), 10, 32)
			if err != nil || id == 0 || strconv.FormatUint(id, 10) != entry.Name() {
				continue
			}
			result = append(result, Account{SteamID: strconv.FormatUint(steamBase+id, 10), Root: canonical})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SteamID == result[j].SteamID {
			return result[i].Root < result[j].Root
		}
		return result[i].SteamID < result[j].SteamID
	})
	return result, nil
}

func (a Account) collectionPath() (string, error) {
	id, err := strconv.ParseUint(a.SteamID, 10, 64)
	if err != nil || id <= steamBase || id > steamBase+4294967295 {
		return "", ErrMalformed
	}
	return filepath.Join(a.Root, "userdata", strconv.FormatUint(id-steamBase, 10), "config", "cloudstorage", "cloud-storage-namespace-1.json"), nil
}
