package steam

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func record(id, name string, added, removed []int) []any {
	b, _ := json.Marshal(map[string]any{"id": id, "name": name, "added": added, "removed": removed})
	return []any{"user-collections." + id, map[string]any{"value": string(b)}}
}
func encode(v any) []byte { b, _ := json.Marshal(v); return b }

func TestParseHonorsRemovedAndTombstones(t *testing.T) {
	raw := []any{record("done", "Completed", []int{1, 2, 1}, []int{2}), []any{"user-collections.deleted", map[string]any{"is_deleted": true}}, []any{"unrelated", map[string]any{"secret": "never export"}}}
	got, err := ParseCollections(encode(raw))
	if err != nil || len(got) != 1 || len(got[0].AppIDs) != 1 || got[0].AppIDs[0] != 1 || got[0].Status != "supported" {
		t.Fatalf("wrong membership: %+v %v", got, err)
	}
}
func TestUnknownShapeIsUnsupportedNotEmpty(t *testing.T) {
	v := []any{[]any{"user-collections.dynamic", map[string]any{"value": `{"id":"dynamic","name":"Completed","filter":{}}`}}}
	got, err := ParseCollections(encode(v))
	if err != nil || len(got) != 1 || got[0].Status != "unsupported" || got[0].AppIDs != nil {
		t.Fatalf("unresolved membership: %+v %v", got, err)
	}
}
func TestParseRejectsDamagedSupportedRecords(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `[`, `[["user-collections.x",{"value":"bad json"}]]`, `[["user-collections.x",{"value":"{\"id\":\"x\",\"name\":\"Done\",\"added\":[-1],\"removed\":[]}"}]]`} {
		if _, err := ParseCollections([]byte(raw)); err == nil {
			t.Errorf("accepted damaged data %s", raw)
		}
	}
	if _, err := ParseCollections(encode([]any{record("x", "Done", []int{}, []int{}), record("x", "Dup", []int{}, []int{})})); err == nil {
		t.Fatal("accepted duplicate collection ID")
	}
}
func TestEmptySnapshotAndLimits(t *testing.T) {
	got, err := ParseCollections([]byte(`[]`))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("valid empty source rejected: %v %v", got, err)
	}
	if _, err := ParseCollections([]byte(strings.Repeat(" ", 2*1024*1024+1))); err == nil {
		t.Fatal("accepted oversize source")
	}
	ids := make([]int, 10001)
	for i := range ids {
		ids[i] = i + 1
	}
	if _, err := ParseCollections(encode([]any{record("x", "Done", ids, []int{})})); err == nil {
		t.Fatal("accepted too many unique IDs")
	}
	if _, err := ParseCollections(encode([]any{record("x", strings.Repeat("a", 257), []int{}, []int{})})); err == nil {
		t.Fatal("accepted oversize name")
	}
}
func TestReadRejectsPartialAndOversizeWithoutSnapshot(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "userdata", "1", "config", "cloudstorage")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "cloud-storage-namespace-1.json")
	for _, raw := range []string{`[`, strings.Repeat(" ", 2*1024*1024+1)} {
		os.WriteFile(file, []byte(raw), 0600)
		snapshot, err := Read(Account{SteamID: "76561197960265729", Root: root})
		if err == nil || snapshot.SteamID != "" {
			t.Fatalf("bad source emitted snapshot: %+v %v", snapshot, err)
		}
	}
	os.WriteFile(file, encode([]any{record("done", "Completed", []int{1}, []int{})}), 0600)
	a, err := Read(Account{SteamID: "76561197960265729", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(Account{SteamID: "76561197960265729", Root: root})
	if err != nil || a.Revision != b.Revision || a.Revision == "" {
		t.Fatal("unstable semantic revision")
	}
	encoded := string(encode(a))
	if strings.Contains(encoded, root) || strings.Contains(encoded, "namespace") {
		t.Fatal("snapshot leaks local paths")
	}
}

func TestMalformedNamespaceEntriesCannotBecomeEmptySnapshot(t *testing.T) {
	for _, raw := range []string{`[null]`, `[{}]`, `[[]]`, `[[123,{}]]`, `[["user-collections.done"]]`, `[["unrelated"]]`} {
		if _, err := ParseCollections([]byte(raw)); err == nil {
			t.Errorf("damaged namespace accepted as complete: %s", raw)
		}
	}
}
