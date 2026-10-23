package steam

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type manualClock struct{ tick chan time.Time }

func (c manualClock) Now() time.Time                       { return time.Now() }
func (c manualClock) After(time.Duration) <-chan time.Time { return c.tick }
func fixtureAccount(t *testing.T) Account {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "userdata", "1", "config", "cloudstorage")
	os.MkdirAll(dir, 0700)
	a := Account{SteamID: "76561197960265729", Root: root}
	file, _ := a.collectionPath()
	os.WriteFile(file, encode([]any{record("done", "Completed", []int{1}, []int{})}), 0600)
	return a
}
func TestBrokenReadRetainsLastGood(t *testing.T) {
	a := fixtureAccount(t)
	w := NewWatcher(a, nil)
	before := w.Current()
	file, _ := a.collectionPath()
	os.WriteFile(file, []byte("["), 0600)
	w.Refresh()
	after := w.Current()
	if after.State != "stale" || after.Snapshot == nil || after.Snapshot.Revision != before.Snapshot.Revision || after.Snapshot.ReadAt != before.Snapshot.ReadAt {
		t.Fatalf("last-good lost: %+v", after)
	}
	after.Snapshot.Collections[0].AppIDs[0] = 99
	if w.Current().Snapshot.Collections[0].AppIDs[0] != 1 {
		t.Fatal("caller mutated stored snapshot")
	}
}
func TestAtomicReplacementAndMissedEvent(t *testing.T) {
	a := fixtureAccount(t)
	clock := manualClock{make(chan time.Time, 2)}
	w := NewWatcher(a, clock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	file, _ := a.collectionPath()
	tmp := file + ".new"
	os.WriteFile(tmp, encode([]any{record("done", "Completed", []int{2}, []int{})}), 0600)
	os.Rename(tmp, file)
	clock.tick <- time.Now() // fallback covers even an event missed during watcher setup
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && w.Current().Snapshot.Collections[0].AppIDs[0] != 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if w.Current().Snapshot.Collections[0].AppIDs[0] != 2 {
		t.Fatal("replacement/fallback did not refresh")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
}
func TestUnchangedReadRefreshesFreshnessNotRevision(t *testing.T) {
	a := fixtureAccount(t)
	w := NewWatcher(a, nil)
	before := w.Current()
	w.Refresh()
	after := w.Current()
	if after.Snapshot.Revision != before.Snapshot.Revision || after.Snapshot.ReadAt == before.Snapshot.ReadAt {
		t.Fatal("freshness/revision conflated")
	}
}
