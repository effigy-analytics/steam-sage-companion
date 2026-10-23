package bridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testMAC(key string, parts ...string) string {
	data, _ := json.Marshal(parts)
	m := hmac.New(sha256.New, []byte(key))
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}
func challenge(t *testing.T, s *Server, token, method, path string) (string, []string) {
	t.Helper()
	key := hash(token)
	nonce := secret()
	w := request(s, "POST", "/v1/challenge", origin, "", map[string]string{"pairing_id": hash(key), "client_nonce": nonce, "method": method, "path": path, "body_sha256": hash("")})
	if w.Code != 201 {
		t.Fatalf("challenge failed: %d", w.Code)
	}
	v := obj(t, w)
	id := v["challenge_id"].(string)
	serverNonce := v["server_nonce"].(string)
	context := []string{origin, id, nonce, serverNonce, method, path, hash("")}
	if v["proof"] != testMAC(key, append([]string{"steam-sage-companion:v1:server"}, context...)...) {
		t.Fatal("unverified server")
	}
	return "SageProof " + id + "." + testMAC(key, append([]string{"steam-sage-companion:v1:client"}, context...)...), context
}
func signedRequest(s *Server, method, path, authorization string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, local+path, nil)
	r.Host = Address
	r.Header.Set("Origin", origin)
	r.Header.Set("Authorization", authorization)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestPersistentBearerSecretIsRejected(t *testing.T) {
	s, _ := newTest(t)
	token := approveAndClaim(t, s)
	if w := signedRequest(s, "GET", "/v1/snapshot", "Bearer "+token); w.Code != 401 {
		t.Fatal("persistent secret accepted on loopback")
	}
}
func TestMutualProofBindsOperationAndResponseAndCannotReplay(t *testing.T) {
	s, store := newTest(t)
	token := approveAndClaim(t, s)
	auth, context := challenge(t, s, token, "GET", "/v1/snapshot")
	if strings.Contains(auth, token) || strings.Contains(auth, hash(token)) {
		t.Fatal("credential sent to listener")
	}
	if w := signedRequest(s, "DELETE", "/v1/pairing", auth); w.Code != 401 {
		t.Fatal("method/path proof substituted")
	}
	w := signedRequest(s, "GET", "/v1/snapshot", auth)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	parts := append([]string{"steam-sage-companion:v1:response"}, context...)
	parts = append(parts, "200", hash(w.Body.String()))
	if w.Header().Get("X-Sage-Proof") != testMAC(hash(token), parts...) {
		t.Fatal("unsigned snapshot response")
	}
	if w := signedRequest(s, "GET", "/v1/snapshot", auth); w.Code != 401 {
		t.Fatal("replay accepted")
	}
	auth, _ = challenge(t, s, token, "GET", "/v1/snapshot")
	restarted := New(Config{Store: store}, fakeProvider{})
	if w := signedRequest(restarted, "GET", "/v1/snapshot", auth); w.Code != 401 {
		t.Fatal("challenge survived restart")
	}
	s.now = func() time.Time { return time.Now().Add(time.Minute) }
	if w := signedRequest(s, "GET", "/v1/snapshot", auth); w.Code != 401 {
		t.Fatal("expired proof accepted")
	}
}

func TestProofChallengeMemoryIsBoundedAndReaped(t *testing.T) {
	s, _ := newTest(t)
	token := approveAndClaim(t, s)
	body := map[string]string{"pairing_id": hash(hash(token)), "client_nonce": secret(), "method": "GET", "path": "/v1/snapshot", "body_sha256": hash("")}
	for i := 0; i < 128; i++ {
		if w := request(s, "POST", "/v1/challenge", origin, "", body); w.Code != 201 {
			t.Fatal("challenge rejected before limit")
		}
	}
	if w := request(s, "POST", "/v1/challenge", origin, "", body); w.Code != 429 {
		t.Fatal("unbounded challenges")
	}
	s.now = func() time.Time { return time.Now().Add(31 * time.Second) }
	if w := request(s, "POST", "/v1/challenge", origin, "", body); w.Code != 201 {
		t.Fatal("expired challenges not reaped")
	}
}
