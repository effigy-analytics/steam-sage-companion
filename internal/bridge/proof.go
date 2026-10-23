package bridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
)

type proofChallenge struct {
	Pair                                                 config.Pairing
	ID, ClientNonce, ServerNonce, Method, Path, BodyHash string
	Expires                                              time.Time
}

func proofMAC(key string, parts ...string) string {
	data, _ := json.Marshal(parts)
	m := hmac.New(sha256.New, []byte(key))
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}
func (p proofChallenge) parts(domain string) []string {
	return []string{"steam-sage-companion:v1:" + domain, p.Pair.Origin, p.ID, p.ClientNonce, p.ServerNonce, p.Method, p.Path, p.BodyHash}
}
func nonce(s string) bool {
	v, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(v) == 32 && len(s) == 43
}
func (s *Server) challenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PairingID   string `json:"pairing_id"`
		ClientNonce string `json:"client_nonce"`
		Method      string `json:"method"`
		Path        string `json:"path"`
		BodyHash    string `json:"body_sha256"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !nonce(body.ClientNonce) || body.BodyHash != hash("") || !((body.Method == "GET" && body.Path == "/v1/snapshot") || (body.Method == "DELETE" && body.Path == "/v1/pairing")) {
		fail(w, 400)
		return
	}
	c := s.cfg.Store.View()
	for _, pair := range c.Pairings {
		if pair.Origin != r.Header.Get("Origin") || pair.SteamID != c.SelectedSteamID || subtle.ConstantTimeCompare([]byte(hash(pair.Hash)), []byte(body.PairingID)) != 1 {
			continue
		}
		if len(s.proofs) >= 128 {
			fail(w, 429)
			return
		}
		p := proofChallenge{Pair: pair, ID: secret(), ClientNonce: body.ClientNonce, ServerNonce: secret(), Method: body.Method, Path: body.Path, BodyHash: body.BodyHash, Expires: s.now().Add(30 * time.Second)}
		s.proofs[p.ID] = p
		reply(w, 201, map[string]string{"challenge_id": p.ID, "server_nonce": p.ServerNonce, "proof": proofMAC(pair.Hash, p.parts("server")...)})
		return
	}
	fail(w, 401)
}
func (s *Server) authorized(r *http.Request) (config.Pairing, proofChallenge, bool) {
	fail := func() (config.Pairing, proofChallenge, bool) { return config.Pairing{}, proofChallenge{}, false }
	if !strings.HasPrefix(r.Header.Get("Authorization"), "SageProof ") {
		return fail()
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "SageProof ")
	fields := strings.Split(auth, ".")
	if len(fields) != 2 {
		return fail()
	}
	p, ok := s.proofs[fields[0]]
	if !ok || !s.now().Before(p.Expires) || p.Pair.Origin != r.Header.Get("Origin") || p.Method != r.Method || p.Path != r.URL.Path || r.URL.RawQuery != "" {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8193))
	if err != nil || len(data) > 8192 || hash(string(data)) != p.BodyHash || subtle.ConstantTimeCompare([]byte(fields[1]), []byte(proofMAC(p.Pair.Hash, p.parts("client")...))) != 1 {
		return fail()
	}
	c := s.cfg.Store.View()
	for _, pair := range c.Pairings {
		if pair == p.Pair && pair.SteamID == c.SelectedSteamID {
			delete(s.proofs, p.ID)
			return pair, p, true
		}
	}
	return fail()
}
func proofWrite(w http.ResponseWriter, p proofChallenge, status int, data []byte) {
	parts := append(p.parts("response"), strconv.Itoa(status), hash(string(data)))
	w.Header().Set("X-Sage-Proof", proofMAC(p.Pair.Hash, parts...))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}
