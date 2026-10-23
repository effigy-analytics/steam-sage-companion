package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateAtomicStoreAndRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "settings")
	c, err := Load(dir)
	if err != nil || c.InstanceID == "" {
		t.Fatalf("initial settings: %v", err)
	}
	c.SelectedSteamID = "76561198000000001"
	c.Pairings = []Pairing{{Hash: "hashed-token", Origin: "https://example.invalid", SteamID: c.SelectedSteamID}}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.InstanceID != c.InstanceID || len(got.Pairings) != 1 {
		t.Fatalf("restart: %+v %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "settings.json"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("settings are not private")
		}
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("broken"), 0600)
	if _, err := Load(dir); err == nil {
		t.Fatal("silently replaced damaged settings")
	}
}
