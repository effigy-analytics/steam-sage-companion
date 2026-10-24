package bridge

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func readControl(s *Server, token, focus string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", local+"/control/state?request_id="+url.QueryEscape(focus), nil)
	r.Header.Set("Origin", local)
	r.Header.Set("X-Companion-Control", token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestFocusedControlLifecycleKeepsExactRequestAndOriginalExpiry(t *testing.T) {
	for _, terminal := range []string{"claimed", "declined", "cancelled", "expired"} {
		t.Run(terminal, func(t *testing.T) {
			s, _ := newTest(t)
			now := time.Now()
			s.now = func() time.Time { return now }
			id, verifier := pairing(t, s)
			expires := now.Add(120 * time.Second).UTC().Format(time.RFC3339)
			token := control(t, s)
			check := func(want string) {
				t.Helper()
				w := readControl(s, token, id)
				focused, ok := obj(t, w)["focused_request"].(map[string]any)
				if w.Code != 200 || !ok || focused["request_id"] != id || focused["origin"] != origin || focused["expires_at"] != expires || focused["state"] != want {
					t.Fatalf("focused %s: %d %s", want, w.Code, w.Body)
				}
			}
			check("pending")
			if terminal == "claimed" {
				admin(s, "/control/approve", token, map[string]string{"request_id": id})
				check("approved")
				request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": verifier})
			} else if terminal == "declined" {
				admin(s, "/control/decline", token, map[string]string{"request_id": id})
			} else if terminal == "cancelled" {
				pairAction(s, "cancel", id, verifier)
			} else {
				now = now.Add(121 * time.Second)
			}
			pairing(t, s) // Another pending request must never replace the focused one.
			check(terminal)
			if terminal == "expired" {
				if w := pairAction(s, "cancel", id, verifier); w.Code != 410 {
					t.Fatal("private expiry history must not revive an expired cancellation")
				}
				check("expired")
			}
			if w := readControl(s, "", id); w.Code != 401 || strings.Contains(w.Body.String(), "focused_request") {
				t.Fatal("focused request escaped private authentication")
			}
			if w := readControl(s, token, strings.Repeat("x", 65)); w.Code != 400 {
				t.Fatal("unbounded focus accepted")
			}
		})
	}
}
