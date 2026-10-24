package bridge

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func dispatcher(t *testing.T, s *Server, fn func(string, func(error)) error) {
	t.Helper()
	s.cfg.OpenBrowser = fn
}
func pairAction(s *Server, action, id, v string) *httptest.ResponseRecorder {
	return request(s, "POST", "/v1/pair/"+action, origin, "", map[string]string{"request_id": id, "verifier": v})
}
func TestPairingOpeningRequiresAllowedOriginAndVerifier(t *testing.T) {
	s, _ := newTest(t)
	id, v := pairing(t, s)
	s.cfg.ExtraOrigins = []string{"https://dev.invalid"}
	for _, action := range []string{"open", "cancel"} {
		if w := pairAction(s, action, id, "wrong"); w.Code != 403 {
			t.Fatalf("%s verifier: %d", action, w.Code)
		}
		if w := request(s, "POST", "/v1/pair/"+action, "https://evil.invalid", "", map[string]string{"request_id": id, "verifier": v}); w.Code != 403 {
			t.Fatal("origin accepted")
		}
		if w := request(s, "POST", "/v1/pair/"+action, "https://dev.invalid", "", map[string]string{"request_id": id, "verifier": v}); w.Code != 403 {
			t.Fatal("different allowed origin accepted request authority")
		}
		r := httptest.NewRequest("POST", local+"/v1/pair/"+action, nil)
		r.Host = "evil.invalid:43871"
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("Host accepted")
		}
	}
	dispatcher(t, s, func(string, func(error)) error { return nil })
	w := pairAction(s, "open", id, v)
	if w.Code != 202 || obj(t, w)["state"] != "opening" {
		t.Fatalf("open: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "launch") || strings.Contains(w.Body.String(), local) {
		t.Fatal("private launch leaked")
	}
}
func TestCancelAfterClaimRevokesOnlyItsGrant(t *testing.T) {
	s, store := newTest(t)
	older := approveAndClaim(t, s)
	id, v := pairing(t, s)
	admin(s, "/control/approve", control(t, s), map[string]string{"request_id": id})
	w := pairAction(s, "claim", id, v)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	newer := approveAndClaim(t, s)
	if w := pairAction(s, "cancel", id, "wrong"); w.Code != 403 {
		t.Fatal("cancel verifier ignored")
	}
	for i := 0; i < 2; i++ {
		if w := pairAction(s, "cancel", id, v); w.Code != 200 || len(store.View().Pairings) != 2 {
			t.Fatalf("wrong grant withdrawn: %d %s", w.Code, w.Body)
		}
	}
	for _, token := range []string{older, newer} {
		if w := request(s, "GET", "/v1/snapshot", origin, token, nil); w.Code != 200 {
			t.Fatal("another pairing lost")
		}
	}
	if w := pairAction(s, "claim", id, v); w.Code != 410 || obj(t, w)["error"] != "cancelled" {
		t.Fatal("cancelled claim allowed")
	}
}
func TestDeclinedRequestCannotBeClaimed(t *testing.T) {
	s, _ := newTest(t)
	id, v := pairing(t, s)
	if w := admin(s, "/control/decline", "wrong", map[string]string{"request_id": id}); w.Code != 401 {
		t.Fatal("unguarded decline")
	}
	if w := admin(s, "/control/decline", control(t, s), map[string]string{"request_id": id}); w.Code != 200 {
		t.Fatalf("decline: %d", w.Code)
	}
	if w := pairAction(s, "claim", id, v); w.Code != 410 || obj(t, w)["error"] != "declined" {
		t.Fatalf("declined claim: %d %s", w.Code, w.Body)
	}
	if w := pairAction(s, "claim", id, "wrong"); w.Code != 403 {
		t.Fatal("terminal verifier bypassed")
	}
}
func TestNativeOpenCallbackRunsOutsideMutex(t *testing.T) {
	s, _ := newTest(t)
	id, v := pairing(t, s)
	calls := make(chan string, 1)
	dispatcher(t, s, func(u string, exited func(error)) error {
		s.NewControlURL()
		exited(errors.New("failed"))
		calls <- u
		return nil
	})
	if w := pairAction(s, "open", id, v); w.Code != 202 {
		t.Fatal(w.Code)
	}
	select {
	case u := <-calls:
		if !strings.Contains(u, "request_id="+id) {
			t.Fatal("missing focus")
		}
	case <-time.After(time.Second):
		t.Fatal("browser dispatcher held mutex")
	}
	if w := pairAction(s, "claim", id, v); obj(t, w)["approval_ui"] != "failed" {
		t.Fatalf("late failure missing: %s", w.Body)
	}
}
func TestLateOpenCannotRetargetNewRequest(t *testing.T) {
	s, _ := newTest(t)
	id, v := pairing(t, s)
	callbacks := make(chan func(error), 2)
	urls := make(chan string, 2)
	dispatcher(t, s, func(u string, exited func(error)) error { urls <- u; callbacks <- exited; return nil })
	pairAction(s, "open", id, v)
	old := receiveOpening(t, callbacks)
	u := receiveOpening(t, urls)
	pairAction(s, "cancel", id, v)
	newID, newV := pairing(t, s)
	receiveOpening(t, callbacks)
	newURL := receiveOpening(t, urls)
	old(errors.New("old failure"))
	if strings.Contains(u, newID) || !strings.Contains(newURL, newID) {
		t.Fatal("request focus retargeted")
	}
	if w := pairAction(s, "claim", newID, newV); obj(t, w)["approval_ui"] == "failed" {
		t.Fatal("late callback changed newer request")
	}
	if w := admin(s, "/control/approve", control(t, s), map[string]string{"request_id": id}); w.Code != 410 {
		t.Fatal("cancelled page can approve")
	}
}
func TestOpeningCooldownExpiryAndReceiptBounds(t *testing.T) {
	s, _ := newTest(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	id, v := pairing(t, s)
	dispatcher(t, s, func(string, func(error)) error { return nil })
	pairAction(s, "open", id, v)
	if w := pairAction(s, "open", id, v); w.Code != 429 || obj(t, w)["retry_after_ms"] != float64(5000) {
		t.Fatalf("cooldown: %d %s", w.Code, w.Body)
	}
	for i := 0; i < 4; i++ {
		s.mu.Lock()
		now = now.Add(5 * time.Second)
		s.mu.Unlock()
		if w := pairAction(s, "open", id, v); w.Code != 202 {
			t.Fatal(w.Code)
		}
	}
	s.mu.Lock()
	now = now.Add(5 * time.Second)
	s.mu.Unlock()
	if w := pairAction(s, "open", id, v); w.Code != 429 {
		t.Fatal("global opening limit missing")
	}
	s.mu.Lock()
	now = now.Add(95 * time.Second)
	s.mu.Unlock()
	if w := pairAction(s, "open", id, v); w.Code != 410 {
		t.Fatal("reopening extended expiry")
	}
	for i := 0; i < 65; i++ {
		s.mu.Lock()
		now = now.Add(13 * time.Second)
		s.mu.Unlock()
		id, v = pairing(t, s)
		pairAction(s, "cancel", id, v)
	}
	// Terminal TTL is fixed at cancellation and is not renewed by idempotent reuse.
	s.mu.Lock()
	now = now.Add(119 * time.Second)
	s.mu.Unlock()
	if w := pairAction(s, "cancel", id, v); w.Code != 200 {
		t.Fatal("receipt expired early")
	}
	s.mu.Lock()
	now = now.Add(time.Second)
	s.mu.Unlock()
	if w := pairAction(s, "cancel", id, v); w.Code != 410 {
		t.Fatal("receipt reused beyond TTL")
	}
	if w := pairAction(s, "claim", id, v); w.Code != 410 || obj(t, w)["error"] != "expired" {
		t.Fatal("expiry classification missing")
	}
}

func TestTerminalReceiptCapAndMemoryOnly(t *testing.T) {
	s, store := newTest(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	for i := 0; i < 65; i++ {
		id := fmt.Sprintf("receipt-%d", i)
		// Seed pending authority to isolate terminal bounds from the independent request throttle.
		s.pending[id] = &pending{Origin: origin, Challenge: hash("verifier"), Expires: now.Add(120 * time.Second)}
		if w := pairAction(s, "cancel", id, "verifier"); w.Code != 200 {
			t.Fatal(w.Code)
		}
		now = now.Add(time.Millisecond)
	}
	if len(s.receipts) != 64 {
		t.Fatal("terminal receipts unbounded")
	}
	if w := pairAction(s, "cancel", "receipt-0", "verifier"); w.Code != 410 {
		t.Fatal("oldest terminal retained")
	}
	restarted := New(Config{Store: store}, fakeProvider{})
	if w := pairAction(restarted, "cancel", "receipt-64", "verifier"); w.Code != 410 {
		t.Fatal("receipt persisted")
	}
}
func TestCapabilitiesAndPendingNeverExposePrivateAuthority(t *testing.T) {
	s, _ := newTest(t)
	status := request(s, "GET", "/v1/status", origin, "", nil)
	caps := obj(t, status)["capabilities"].([]any)
	if len(caps) != 1 || caps[0] != "pairing-cancel-v1" {
		t.Fatal("nil dispatcher advertised native UI")
	}
	id, v := pairing(t, s)
	dispatcher(t, s, func(string, func(error)) error { return nil })
	status = request(s, "GET", "/v1/status", origin, "", nil)
	caps = obj(t, status)["capabilities"].([]any)
	if len(caps) != 3 {
		t.Fatal("native capabilities missing")
	}
	w := pairAction(s, "claim", id, v)
	if obj(t, w)["approval_ui"] != "manual" {
		t.Fatal("manual fallback missing")
	}
	for _, r := range []*httptest.ResponseRecorder{status, w} {
		for _, private := range []string{"control_token", "capability", "launch=", local} {
			if strings.Contains(r.Body.String(), private) {
				t.Fatal("private authority leaked")
			}
		}
	}
}

func TestPairRequestAutomaticallyOpensConfiguredApproval(t *testing.T) {
	s, _ := newTest(t)
	urls := make(chan string, 1)
	dispatcher(t, s, func(u string, exited func(error)) error { urls <- u; return nil })
	verifier := "auto-verifier"
	w := request(s, "POST", "/v1/pair/request", origin, "", map[string]string{"challenge_sha256": hash(verifier)})
	if w.Code != 201 || obj(t, w)["approval_ui"] != "opening" {
		t.Fatalf("automatic approval opening: %d %s", w.Code, w.Body)
	}
	id := obj(t, w)["request_id"].(string)
	select {
	case u := <-urls:
		if !strings.Contains(u, "request_id="+id) {
			t.Fatal("automatic opening missing focus")
		}
	case <-time.After(time.Second):
		t.Fatal("automatic approval not dispatched")
	}
	if w := pairAction(s, "open", id, verifier); w.Code != 429 {
		t.Fatal("automatic dispatch bypassed retry cooldown")
	}
}

func TestCancelRacingSuccessfulClaimPreservesOtherGrant(t *testing.T) {
	for i := 0; i < 8; i++ {
		s, store := newTest(t)
		older := approveAndClaim(t, s)
		id, v := pairing(t, s)
		if w := admin(s, "/control/approve", control(t, s), map[string]string{"request_id": id}); w.Code != 200 {
			t.Fatal(w.Code)
		}
		start := make(chan struct{})
		claim := make(chan *httptest.ResponseRecorder, 1)
		cancel := make(chan *httptest.ResponseRecorder, 1)
		go func() { <-start; claim <- pairAction(s, "claim", id, v) }()
		go func() { <-start; cancel <- pairAction(s, "cancel", id, v) }()
		close(start)
		claimed, cancelled := <-claim, <-cancel
		if cancelled.Code != 200 || (claimed.Code != 200 && claimed.Code != 410) {
			t.Fatal("claim/cancel race returned unexpected status")
		}
		if len(store.View().Pairings) != 1 {
			t.Fatal("cancel race retained its grant or removed another")
		}
		if w := request(s, "GET", "/v1/snapshot", origin, older, nil); w.Code != 200 {
			t.Fatal("cancel race removed prior grant")
		}
	}
}
