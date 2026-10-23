package steam

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
)

var ErrMalformed = errors.New("malformed")
var ErrLimit = errors.New("limit")
var ErrChanging = errors.New("changing")
var ErrMissing = errors.New("missing")
var ErrUnreadable = errors.New("unreadable")

func ParseCollections(data []byte) ([]protocol.Collection, error) {
	if len(data) > protocol.MaxBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(data) {
		return nil, ErrMalformed
	}
	var entries []json.RawMessage
	if json.Unmarshal(data, &entries) != nil || entries == nil {
		return nil, ErrMalformed
	}
	collections := []protocol.Collection{}
	seen := map[string]bool{}
	allIDs := map[uint32]bool{}
	for _, raw := range entries {
		var pair []json.RawMessage
		if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 {
			return nil, ErrMalformed
		}
		var key string
		if json.Unmarshal(pair[0], &key) != nil || key == "" {
			return nil, ErrMalformed
		}
		if !strings.HasPrefix(key, "user-collections.") {
			continue
		}
		var meta map[string]json.RawMessage
		if json.Unmarshal(pair[1], &meta) != nil || meta == nil {
			return nil, ErrMalformed
		}
		if raw, ok := meta["is_deleted"]; ok {
			var deleted bool
			if json.Unmarshal(raw, &deleted) != nil {
				return nil, ErrMalformed
			}
			if deleted {
				continue
			}
		}
		var value string
		if json.Unmarshal(meta["value"], &value) != nil {
			return nil, ErrMalformed
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(value), &object) != nil || object == nil {
			return nil, ErrMalformed
		}
		var id, name string
		if json.Unmarshal(object["id"], &id) != nil || json.Unmarshal(object["name"], &name) != nil || strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || seen[id] {
			return nil, ErrMalformed
		}
		if len(id) > 256 || len(name) > 256 || len(collections) >= protocol.MaxCollections {
			return nil, ErrLimit
		}
		seen[id] = true
		c := protocol.Collection{ID: id, Name: name, Status: "supported", AppIDs: []uint32{}}
		for field := range object {
			if field != "id" && field != "name" && field != "added" && field != "removed" {
				c.Status = "unsupported"
			}
		}
		if _, ok := object["added"]; !ok {
			c.Status = "unsupported"
		}
		if _, ok := object["removed"]; !ok {
			c.Status = "unsupported"
		}
		if c.Status == "unsupported" {
			c.AppIDs = nil
			collections = append(collections, c)
			continue
		}
		var added, removed []uint32
		if json.Unmarshal(object["added"], &added) != nil || json.Unmarshal(object["removed"], &removed) != nil || added == nil || removed == nil {
			return nil, ErrMalformed
		}
		remove := map[uint32]bool{}
		for _, id := range removed {
			if id == 0 {
				return nil, ErrMalformed
			}
			remove[id] = true
		}
		ids := map[uint32]bool{}
		for _, id := range added {
			if id == 0 {
				return nil, ErrMalformed
			}
			if !remove[id] {
				ids[id] = true
			}
		}
		for id := range ids {
			allIDs[id] = true
			c.AppIDs = append(c.AppIDs, id)
		}
		if len(allIDs) > protocol.MaxGameIDs {
			return nil, ErrLimit
		}
		sort.Slice(c.AppIDs, func(i, j int) bool { return c.AppIDs[i] < c.AppIDs[j] })
		collections = append(collections, c)
	}
	sort.Slice(collections, func(i, j int) bool { return collections[i].ID < collections[j].ID })
	return collections, nil
}
