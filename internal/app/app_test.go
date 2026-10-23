package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
)

func TestMultipleAccountsRequireChoice(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"1", "2"} {
		dir := filepath.Join(root, "userdata", id, "config", "cloudstorage")
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "cloud-storage-namespace-1.json"), []byte(`[]`), 0600)
	}
	store, _ := config.Open(t.TempDir())
	a, err := New(store, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if store.View().SelectedSteamID != "" || a.Snapshot("76561197960265729").Snapshot != nil {
		t.Fatal("multiple accounts were silently chosen")
	}
	if err := a.SelectAccount("76561197960265729"); err != nil {
		t.Fatal(err)
	}
	if a.Snapshot("76561197960265729").Snapshot == nil || a.Snapshot("76561197960265730").Snapshot != nil {
		t.Fatal("snapshot crossed account selection")
	}
}
func TestAccountChangeRevokesTokens(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"1", "2"} {
		os.MkdirAll(filepath.Join(root, "userdata", id), 0700)
	}
	store, _ := config.Open(t.TempDir())
	a, _ := New(store, []string{root})
	a.SelectAccount("76561197960265729")
	store.Update(func(c *config.Config) error {
		c.Pairings = []config.Pairing{{Hash: "oldhash", Origin: "https://steam-sage.effigy-analytics.com", SteamID: "76561197960265729"}}
		return nil
	})
	if err := a.SelectAccount("76561197960265730"); err != nil || len(store.View().Pairings) != 0 {
		t.Fatal("old account token survived selection change")
	}
	if err := a.SelectAccount("76561197960265731"); err == nil {
		t.Fatal("accepted an undiscovered account")
	}
}

func TestSameAccountAcrossInstallationsSelectsAndRemembersSource(t *testing.T) {
	roots := []string{t.TempDir(), t.TempDir()}
	for i, root := range roots {
		dir := filepath.Join(root, "userdata", "1", "config", "cloudstorage")
		os.MkdirAll(dir, 0700)
		raw := fmt.Sprintf(`[["user-collections.done",{"value":%q}]]`, fmt.Sprintf(`{"id":"done","name":"Completed","added":[%d],"removed":[]}`, i+1))
		os.WriteFile(filepath.Join(dir, "cloud-storage-namespace-1.json"), []byte(raw), 0600)
	}
	store, _ := config.Open(t.TempDir())
	a, _ := New(store, roots)
	if err := a.SelectAccount("76561197960265729"); err == nil {
		t.Fatal("ambiguous account silently picked a source")
	}
	views := a.Accounts()
	if len(views) != 2 || views[0].ID == views[1].ID || views[0].Location == views[1].Location {
		t.Fatal("sources cannot be distinguished")
	}
	if err := a.SelectAccount(views[1].ID); err != nil {
		t.Fatal(err)
	}
	got := a.Snapshot(views[1].SteamID)
	if got.Snapshot == nil {
		t.Fatal("chosen source unavailable")
	}
	wanted := got.Snapshot.SourceID
	appIDs := got.Snapshot.Collections[0].AppIDs
	store.Update(func(c *config.Config) error {
		c.Pairings = []config.Pairing{{Hash: "key", SteamID: views[1].SteamID}}
		return nil
	})
	restarted, _ := New(store, roots)
	if snapshot := restarted.Snapshot(views[1].SteamID).Snapshot; snapshot == nil || snapshot.SourceID != wanted || snapshot.Collections[0].AppIDs[0] != appIDs[0] {
		t.Fatal("restart lost source choice")
	}
	if err := restarted.SelectAccount(views[0].ID); err != nil || len(store.View().Pairings) != 0 {
		t.Fatal("same-account source switch did not revoke")
	}
	selected := 0
	for _, view := range restarted.Accounts() {
		if view.Selected {
			selected++
		}
	}
	if selected != 1 {
		t.Fatal("more than one source selected")
	}
}
