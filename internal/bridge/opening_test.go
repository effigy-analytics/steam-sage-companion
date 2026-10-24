package bridge

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func assertProof(t *testing.T, w *httptest.ResponseRecorder, token string, context []string) {
	t.Helper()
	parts := append([]string{"steam-sage-companion:v1:response"}, context...)
	parts = append(parts, strconv.Itoa(w.Code), hash(w.Body.String()))
	if w.Header().Get("X-Sage-Proof") != testMAC(hash(token), parts...) {
		t.Fatal("response proof missing or wrong")
	}
}
func TestSettingsOpeningProofs(t *testing.T) {
	s, _ := newTest(t)
	token := approveAndClaim(t, s)
	if w := signedRequest(s, "POST", "/v1/settings/open", ""); w.Code != 401 {
		t.Fatalf("anonymous settings: %d", w.Code)
	}
	auth, ctx := challenge(t, s, token, "POST", "/v1/settings/open")
	if w := signedRequest(s, "GET", "/v1/settings/open", auth); w.Code != 401 {
		t.Fatal("method substituted")
	}
	r := httptest.NewRequest("POST", local+"/v1/settings/open", bytes.NewBufferString("{}"))
	r.Host = Address
	r.Header.Set("Origin", origin)
	r.Header.Set("Authorization", auth)
	bad := httptest.NewRecorder()
	s.ServeHTTP(bad, r)
	if bad.Code != 401 {
		t.Fatal("body substituted")
	}
	w := signedRequest(s, "POST", "/v1/settings/open", auth)
	if w.Code != 202 || obj(t, w)["state"] != "manual" {
		t.Fatalf("nil opening: %d %s", w.Code, w.Body)
	}
	assertProof(t, w, token, ctx)
	if w := signedRequest(s, "POST", "/v1/settings/open", auth); w.Code != 401 {
		t.Fatal("replayed proof")
	}
	dispatcher(t, s, func(string, func(error)) error { return errors.New("dispatch failed") })
	auth, ctx = challenge(t, s, token, "POST", "/v1/settings/open")
	w = signedRequest(s, "POST", "/v1/settings/open", auth)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	assertProof(t, w, token, ctx)
	deadline := time.Now().Add(time.Second)
	for {
		auth, ctx = challenge(t, s, token, "GET", "/v1/settings/open")
		w = signedRequest(s, "GET", "/v1/settings/open", auth)
		assertProof(t, w, token, ctx)
		if obj(t, w)["state"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed dispatch not observed")
		}
		time.Sleep(time.Millisecond)
	}
	auth, ctx = challenge(t, s, token, "POST", "/v1/settings/open")
	w = signedRequest(s, "POST", "/v1/settings/open", auth)
	if w.Code != 429 {
		t.Fatalf("settings cooldown: %d", w.Code)
	}
	assertProof(t, w, token, ctx)
	for _, method := range []string{"PUT", "DELETE"} {
		if w := request(s, "POST", "/v1/challenge", origin, "", map[string]string{"pairing_id": hash(hash(token)), "client_nonce": secret(), "method": method, "path": "/v1/settings/open", "body_sha256": hash("")}); w.Code != 400 {
			t.Fatal("extra method allowlisted")
		}
	}
}

func TestSettingsGenerationAndSharedOpeningBudget(t *testing.T) {
	s, _ := newTest(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	id, v := pairing(t, s)
	token := approveAndClaim(t, s)
	callbacks := make(chan func(error), 5)
	dispatcher(t, s, func(_ string, exited func(error)) error { callbacks <- exited; return nil })
	auth, _ := challenge(t, s, token, "POST", "/v1/settings/open")
	signedRequest(s, "POST", "/v1/settings/open", auth)
	old := receiveOpening(t, callbacks)
	s.mu.Lock()
	now = now.Add(5 * time.Second)
	s.mu.Unlock()
	auth, _ = challenge(t, s, token, "POST", "/v1/settings/open")
	signedRequest(s, "POST", "/v1/settings/open", auth)
	latest := receiveOpening(t, callbacks)
	latest(nil)
	old(errors.New("stale exit"))
	auth, ctx := challenge(t, s, token, "GET", "/v1/settings/open")
	w := signedRequest(s, "GET", "/v1/settings/open", auth)
	if obj(t, w)["state"] != "dispatched" {
		t.Fatal("stale settings callback changed latest state")
	}
	assertProof(t, w, token, ctx)
	for i := 0; i < 3; i++ {
		s.mu.Lock()
		now = now.Add(5 * time.Second)
		s.mu.Unlock()
		if w := pairAction(s, "open", id, v); w.Code != 202 {
			t.Fatal(w.Code)
		}
		receiveOpening(t, callbacks)
	}
	s.mu.Lock()
	now = now.Add(5 * time.Second)
	s.mu.Unlock()
	auth, ctx = challenge(t, s, token, "POST", "/v1/settings/open")
	w = signedRequest(s, "POST", "/v1/settings/open", auth)
	if w.Code != 429 {
		t.Fatal("settings did not share pairing global opening budget")
	}
	assertProof(t, w, token, ctx)
	s.mu.Lock()
	now = now.Add(120 * time.Second)
	s.mu.Unlock()
	latest(errors.New("expired callback"))
	auth, _ = challenge(t, s, token, "GET", "/v1/settings/open")
	w = signedRequest(s, "GET", "/v1/settings/open", auth)
	if obj(t, w)["state"] != "manual" || len(s.settings) != 0 {
		t.Fatal("expired opening retained")
	}
}

func TestSettingsRecordCap(t *testing.T) {
	s, _ := newTest(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	token := approveAndClaim(t, s)
	for i := 0; i < 64; i++ {
		s.settings[strconv.Itoa(i)] = &opening{State: Dispatched, Expires: now.Add(time.Duration(i+1) * time.Second)}
	}
	dispatcher(t, s, func(string, func(error)) error { return nil })
	auth, _ := challenge(t, s, token, "POST", "/v1/settings/open")
	w := signedRequest(s, "POST", "/v1/settings/open", auth)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.settings) != 64 || s.settings["0"] != nil || s.settings[hash(token)] == nil {
		t.Fatal("opening records failed bounded oldest eviction")
	}
}

// Bound asynchronous dispatcher observations so a regression fails this test,
// rather than hanging the entire suite until its outer timeout.
func receiveOpening[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("browser dispatch observation timed out")
		var zero T
		return zero
	}
}
