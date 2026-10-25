package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
)

const origin = "https://steam-sage.effigy-analytics.com"
const local = "http://127.0.0.1:43871"

type fakeProvider struct{}

func (fakeProvider) Snapshot(id string) protocol.SnapshotResponse {
	return protocol.SnapshotResponse{Protocol: 1, State: "ready", Snapshot: &protocol.Snapshot{Protocol: 1, SteamID: id, Collections: []protocol.Collection{}}}
}
func newTest(t *testing.T) (*Server, *config.Store) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "settings"))
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(c *config.Config) error { c.SelectedSteamID = "76561198000000001"; return nil })
	return New(Config{Store: store, Version: "test"}, fakeProvider{}), store
}
func request(s http.Handler, method, path, from, token string, body any) *httptest.ResponseRecorder {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	authorization := ""
	if token != "" && (path == "/v1/snapshot" || path == "/v1/pairing") {
		clientNonce := secret()
		key := hash(token)
		challenge := request(s, "POST", "/v1/challenge", from, "", map[string]string{"pairing_id": hash(key), "client_nonce": clientNonce, "method": method, "path": path, "body_sha256": hash(string(data))})
		if challenge.Code != 201 {
			return challenge
		}
		var value map[string]string
		json.Unmarshal(challenge.Body.Bytes(), &value)
		parts := []string{from, value["challenge_id"], clientNonce, value["server_nonce"], method, path, hash(string(data))}
		if value["proof"] != testMAC(key, append([]string{"steam-sage-companion:v1:server"}, parts...)...) {
			panic("invalid server proof")
		}
		authorization = "SageProof " + value["challenge_id"] + "." + testMAC(key, append([]string{"steam-sage-companion:v1:client"}, parts...)...)
	}
	r := httptest.NewRequest(method, local+path, bytes.NewReader(data))
	r.Host = "127.0.0.1:43871"
	if from != "" {
		r.Header.Set("Origin", from)
	}
	if token != "" {
		r.Header.Set("Authorization", authorization)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func obj(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if json.Unmarshal(w.Body.Bytes(), &v) != nil {
		t.Fatalf("not JSON: %s", w.Body)
	}
	return v
}
func control(t *testing.T, s *Server) string {
	t.Helper()
	u, _ := url.Parse(s.NewControlURL())
	cap := strings.TrimPrefix(u.Fragment, "launch=")
	w := request(s, "POST", "/control/session", local, "", map[string]string{"capability": cap})
	if w.Code != 200 {
		t.Fatalf("control %d %s", w.Code, w.Body)
	}
	return obj(t, w)["control_token"].(string)
}
func admin(s *Server, path, token string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", local+path, bytes.NewReader(data))
	r.Host = "127.0.0.1:43871"
	r.Header.Set("Origin", local)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Companion-Control", token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func pairing(t *testing.T, s *Server) (string, string) {
	t.Helper()
	verifier := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	sum := sha256.Sum256([]byte(verifier))
	w := request(s, "POST", "/v1/pair/request", origin, "", map[string]string{"challenge_sha256": hex.EncodeToString(sum[:])})
	if w.Code != 201 {
		t.Fatalf("pair request: %d %s", w.Code, w.Body)
	}
	return obj(t, w)["request_id"].(string), verifier
}
func approveAndClaim(t *testing.T, s *Server) string {
	t.Helper()
	id, v := pairing(t, s)
	w := admin(s, "/control/approve", control(t, s), map[string]string{"request_id": id})
	if w.Code != 200 {
		t.Fatalf("approve %d", w.Code)
	}
	w = request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": v})
	if w.Code != 200 {
		t.Fatalf("claim %d %s", w.Code, w.Body)
	}
	return obj(t, w)["token"].(string)
}

func TestPairRequiresLocalApprovalAndVerifier(t *testing.T) {
	s, _ := newTest(t)
	id, v := pairing(t, s)
	if w := request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": v}); w.Code != 202 {
		t.Fatalf("unapproved claim: %d", w.Code)
	}
	if w := admin(s, "/control/approve", control(t, s), map[string]string{"request_id": id}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": "wrong"}); w.Code != 403 {
		t.Fatalf("wrong verifier: %d", w.Code)
	}
	w := request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": v})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	token := obj(t, w)["token"].(string)
	if w = request(s, "GET", "/v1/snapshot", origin, token, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": v}); w.Code != 410 {
		t.Fatalf("replayed claim: %d", w.Code)
	}
}
func TestRevokeAndRestart(t *testing.T) {
	s, store := newTest(t)
	token := approveAndClaim(t, s)
	for _, p := range store.View().Pairings {
		if p.Hash == token {
			t.Fatal("raw token persisted")
		}
	}
	restarted := New(Config{Store: store, Version: "test"}, fakeProvider{})
	if w := request(restarted, "GET", "/v1/snapshot", origin, token, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(restarted, "DELETE", "/v1/pairing", origin, token, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(New(Config{Store: store}, fakeProvider{}), "GET", "/v1/snapshot", origin, token, nil); w.Code != 401 {
		t.Fatal("revocation did not persist")
	}
}
func TestOriginHostAndBodyBoundaries(t *testing.T) {
	s, _ := newTest(t)
	for _, from := range []string{"https://evil.invalid", "null", "", origin + ".evil.invalid"} {
		if w := request(s, "GET", "/v1/status", from, "", nil); w.Code != 403 {
			t.Fatalf("origin %q: %d", from, w.Code)
		}
	}
	r := httptest.NewRequest("GET", local+"/v1/status", nil)
	r.Host = "attacker.invalid:43871"
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("forged Host accepted")
	}
	if w := request(s, "GET", "/v1/snapshot", origin, "", nil); w.Code != 401 {
		t.Fatal("anonymous snapshot")
	}
	if w := request(s, "POST", "/v1/pair/request", origin, "", map[string]string{"challenge_sha256": strings.Repeat("a", 9000)}); w.Code != 413 {
		t.Fatalf("oversize body: %d", w.Code)
	}
	if w := admin(s, "/control/approve", "wrong", map[string]string{"request_id": "x"}); w.Code != 401 {
		t.Fatal("unguarded control mutation")
	}
}
func TestRateLimitPreflightAndExpiry(t *testing.T) {
	s, _ := newTest(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		pairing(t, s)
	}
	if w := request(s, "POST", "/v1/pair/request", origin, "", map[string]string{"challenge_sha256": strings.Repeat("a", 64)}); w.Code != 429 {
		t.Fatalf("rate limit: %d", w.Code)
	}
	r := httptest.NewRequest("OPTIONS", local+"/v1/snapshot", nil)
	r.Host = "127.0.0.1:43871"
	r.Header.Set("Origin", origin)
	r.Header.Set("Access-Control-Request-Method", "GET")
	r.Header.Set("Access-Control-Request-Headers", "authorization")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("preflight %d %v", w.Code, w.Header())
	}
	now = now.Add(121 * time.Second)
	id, v := pairing(t, s)
	now = now.Add(121 * time.Second)
	if w := request(s, "POST", "/v1/pair/claim", origin, "", map[string]string{"request_id": id, "verifier": v}); w.Code != 410 {
		t.Fatalf("expired claim: %d", w.Code)
	}
	u, _ := url.Parse(s.NewControlURL())
	cap := strings.TrimPrefix(u.Fragment, "launch=")
	now = now.Add(301 * time.Second)
	if w := request(s, "POST", "/control/session", local, "", map[string]string{"capability": cap}); w.Code != 403 {
		t.Fatal("expired launch capability")
	}
}
