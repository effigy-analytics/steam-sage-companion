package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverDeduplicatesSymlinksAndRejectsInvalidAccounts(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"1", "0", "-1", "4294967296", "alice", "01"} {
		if err := os.MkdirAll(filepath.Join(root, "userdata", id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "userdata", "2"), []byte("not a directory"), 0600)
	alias := filepath.Join(t.TempDir(), "Steam")
	roots := []string{root, root, filepath.Join(root, "missing")}
	if os.Symlink(root, alias) == nil {
		roots = append(roots, alias)
	}
	got, err := Discover(roots)
	if err != nil || len(got) != 1 || got[0].SteamID != "76561197960265729" {
		t.Fatalf("wrong accounts: %+v %v", got, err)
	}
}
