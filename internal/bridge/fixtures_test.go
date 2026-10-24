package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupFixturesHaveBoundedPublicShapes(t *testing.T) {
	paths, err := filepath.Glob("../../protocol/fixtures/setup-*.json")
	if err != nil || len(paths) == 0 {
		t.Fatal("setup fixtures missing")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"control_token", "capability", "launch=", LocalOrigin} {
				if strings.Contains(string(data), private) {
					t.Fatal("fixture leaks private authority")
				}
			}
			if caps, ok := value["capabilities"]; ok {
				items, ok := caps.([]any)
				if !ok || len(items) > 16 {
					t.Fatal("unbounded capabilities")
				}
				for _, item := range items {
					name, ok := item.(string)
					if !ok || len([]byte(name)) > 64 {
						t.Fatal("invalid capability")
					}
				}
			}
			if state, ok := value["approval_ui"]; ok {
				if state != "manual" && state != "opening" && state != "dispatched" && state != "failed" {
					t.Fatal("invalid approval_ui")
				}
			}
			if retry, ok := value["retry_after_ms"]; ok {
				n, ok := retry.(float64)
				if !ok || n < 1 || n > 60000 {
					t.Fatal("unbounded throttle")
				}
			}
		})
	}
}
