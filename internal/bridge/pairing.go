package bridge

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
)

type pending struct {
	Origin, Challenge, SteamID string
	Expires                    time.Time
	Opening                    opening
}
type receipt struct {
	Origin, Challenge, PairingHash, State string
	Expires, Created                      time.Time
}

func (s *Server) finishPairing(id string, p *pending, state, pairingHash string) {
	if len(s.receipts) >= 64 {
		var oldest string
		for key, r := range s.receipts {
			if oldest == "" || r.Created.Before(s.receipts[oldest].Created) {
				oldest = key
			}
		}
		delete(s.receipts, oldest)
	}
	s.receipts[id] = &receipt{Origin: p.Origin, Challenge: p.Challenge, PairingHash: pairingHash, State: state, Created: s.now(), Expires: s.now().Add(120 * time.Second)}
	delete(s.pending, id)
}
func pairingGone(w http.ResponseWriter, state string) {
	reply(w, 410, map[string]string{"error": state})
}
func verifierMatches(origin, challenge, from, verifier string) bool {
	return origin == from && subtle.ConstantTimeCompare([]byte(hash(verifier)), []byte(challenge)) == 1
}
func (s *Server) pairing(w http.ResponseWriter, r *http.Request) {
	from := r.Header.Get("Origin")
	if r.URL.Path == "/v1/pair/request" {
		var body struct {
			Challenge string `json:"challenge_sha256"`
		}
		if !decode(w, r, &body) {
			return
		}
		v, err := hex.DecodeString(body.Challenge)
		if err != nil || len(v) != 32 {
			fail(w, 400)
			return
		}
		if len(s.pending) >= 5 || len(s.attempts) >= 5 {
			fail(w, 429)
			return
		}
		id := secret()
		expires := s.now().Add(120 * time.Second)
		s.pending[id] = &pending{Origin: from, Challenge: strings.ToLower(body.Challenge), Expires: expires, Opening: opening{State: Manual}}
		s.attempts = append(s.attempts, s.now())
		p := s.pending[id]
		if s.cfg.OpenBrowser != nil && s.openRetry(&p.Opening) == 0 {
			s.scheduleOpen(id, "", &p.Opening, expires)
		}
		reply(w, 201, map[string]any{"request_id": id, "expires_at": expires.UTC().Format(time.RFC3339), "approval_ui": p.Opening.State})
		return
	}
	var body struct {
		ID       string `json:"request_id"`
		Verifier string `json:"verifier"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.pending[body.ID]
	if !ok {
		rec, exists := s.receipts[body.ID]
		if !exists {
			pairingGone(w, "expired")
			return
		}
		if !verifierMatches(rec.Origin, rec.Challenge, from, body.Verifier) {
			fail(w, 403)
			return
		}
		if r.URL.Path == "/v1/pair/cancel" {
			if rec.PairingHash != "" {
				if err := s.withdraw(rec.PairingHash); err != nil {
					fail(w, 500)
					return
				}
				rec.PairingHash = ""
			}
			rec.State = "cancelled"
			reply(w, 200, map[string]bool{"cancelled": true})
			return
		}
		state := rec.State
		if state == "claimed" {
			state = "expired"
		}
		pairingGone(w, state)
		return
	}
	if !verifierMatches(p.Origin, p.Challenge, from, body.Verifier) {
		fail(w, 403)
		return
	}
	switch r.URL.Path {
	case "/v1/pair/cancel":
		s.finishPairing(body.ID, p, "cancelled", "")
		reply(w, 200, map[string]bool{"cancelled": true})
	case "/v1/pair/open":
		if s.cfg.OpenBrowser == nil {
			reply(w, 202, map[string]ApprovalUI{"state": Manual})
			return
		}
		if retry := s.openRetry(&p.Opening); retry > 0 {
			openingThrottle(w, proofChallenge{}, retry)
			return
		}
		s.scheduleOpen(body.ID, "", &p.Opening, p.Expires)
		reply(w, 202, map[string]ApprovalUI{"state": Opening})
	case "/v1/pair/claim":
		if p.SteamID == "" {
			reply(w, 202, map[string]any{"state": "pending", "approval_ui": p.Opening.State})
			return
		}
		if p.SteamID != s.cfg.Store.View().SelectedSteamID {
			delete(s.pending, body.ID)
			pairingGone(w, "expired")
			return
		}
		token := secret()
		err := s.cfg.Store.Update(func(c *config.Config) error {
			c.Pairings = append(c.Pairings, config.Pairing{Hash: hash(token), Origin: from, SteamID: p.SteamID})
			return nil
		})
		if err != nil {
			fail(w, 500)
			return
		}
		s.finishPairing(body.ID, p, "claimed", hash(token))
		reply(w, 200, map[string]string{"token": token, "steam_id": p.SteamID, "instance_id": s.cfg.Store.View().InstanceID})
	}
}
func (s *Server) withdraw(pairingHash string) error {
	return s.cfg.Store.Update(func(c *config.Config) error {
		keep := []config.Pairing{}
		for _, p := range c.Pairings {
			if p.Hash != pairingHash {
				keep = append(keep, p)
			}
		}
		c.Pairings = keep
		return nil
	})
}
