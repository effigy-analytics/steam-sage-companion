// Package bridge serves the paired, read-only loopback protocol.
package bridge

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
)

const Address = "127.0.0.1:43871"
const LocalOrigin = "http://" + Address
const ProductionOrigin = "https://steam-sage.effigy-analytics.com"

//go:embed web/*
var assets embed.FS

type SnapshotProvider interface {
	Snapshot(steamID string) protocol.SnapshotResponse
}
type AccountView struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	SteamID  string `json:"steam_id"`
	Selected bool   `json:"selected"`
}
type Config struct {
	Store        *config.Store
	Version      string
	ExtraOrigins []string
	Accounts     func() []AccountView
	OnSelect     func(string) error
	OnStartup    func(bool) error
	OnChooseRoot func() error
	OnSetRoot    func(string) error
	OnStop       func()
}
type pending struct {
	Origin, Challenge, SteamID string
	Expires                    time.Time
}
type Server struct {
	mu                 sync.Mutex
	cfg                Config
	provider           SnapshotProvider
	now                func() time.Time
	pending            map[string]*pending
	launches, controls map[string]time.Time
	attempts           []time.Time
	proofs             map[string]proofChallenge
}

func New(cfg Config, provider SnapshotProvider) *Server {
	return &Server{cfg: cfg, provider: provider, now: time.Now, pending: map[string]*pending{}, proofs: map[string]proofChallenge{}, launches: map[string]time.Time{}, controls: map[string]time.Time{}}
}
func secret() string {
	data := make([]byte, 32)
	rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}
func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func (s *Server) NewControlURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reap()
	cap := secret()
	s.launches[hash(cap)] = s.now().Add(5 * time.Minute)
	return LocalOrigin + "/#launch=" + cap
}
func (s *Server) reap() {
	now := s.now()
	for id, p := range s.proofs {
		if !now.Before(p.Expires) {
			delete(s.proofs, id)
		}
	}
	for id, p := range s.pending {
		if !now.Before(p.Expires) {
			delete(s.pending, id)
		}
	}
	for key, t := range s.launches {
		if !now.Before(t) {
			delete(s.launches, key)
		}
	}
	for key, t := range s.controls {
		if !now.Before(t) {
			delete(s.controls, key)
		}
	}
	var recent []time.Time
	for _, t := range s.attempts {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	s.attempts = recent
}
func (s *Server) allowed(origin string) bool {
	if origin == ProductionOrigin {
		return true
	}
	for _, v := range s.cfg.ExtraOrigins {
		if v == origin && v != "" && v != "null" {
			return true
		}
	}
	return false
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int) {
	reply(w, status, map[string]string{"error": http.StatusText(status)})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		fail(w, 415)
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8193))
	if err != nil {
		fail(w, 400)
		return false
	}
	if len(data) > 8192 {
		fail(w, 413)
		return false
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400)
		return false
	}
	return true
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != Address {
		fail(w, 403)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reap()
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		from := r.Header.Get("Origin")
		if !s.allowed(from) {
			fail(w, 403)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", from)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Expose-Headers", "X-Sage-Proof")
		if r.Method == "OPTIONS" {
			method := r.Header.Get("Access-Control-Request-Method")
			if method != "GET" && method != "POST" && method != "DELETE" {
				fail(w, 403)
				return
			}
			for _, h := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				h = strings.TrimSpace(strings.ToLower(h))
				if h != "" && h != "authorization" && h != "content-type" {
					fail(w, 403)
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(204)
			return
		}
		s.api(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/control/") {
		s.control(w, r)
		return
	}
	if r.Method != "GET" {
		fail(w, 405)
		return
	}
	name := map[string]string{"/": "web/index.html", "/control.js": "web/control.js", "/style.css": "web/style.css"}[r.URL.Path]
	if name == "" {
		fail(w, 404)
		return
	}
	data, err := assets.ReadFile(name)
	if err != nil {
		fail(w, 404)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.Write(data)
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	from := r.Header.Get("Origin")
	switch r.Method + " " + r.URL.Path {
	case "GET /v1/status":
		reply(w, 200, map[string]any{"protocol": 1, "helper_version": s.cfg.Version, "instance_id": s.cfg.Store.View().InstanceID})
	case "POST /v1/challenge":
		s.challenge(w, r)
	case "POST /v1/pair/request":
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
		s.pending[id] = &pending{Origin: from, Challenge: strings.ToLower(body.Challenge), Expires: expires}
		s.attempts = append(s.attempts, s.now())
		reply(w, 201, map[string]any{"request_id": id, "expires_at": expires.UTC().Format(time.RFC3339)})
	case "POST /v1/pair/claim":
		var body struct {
			ID       string `json:"request_id"`
			Verifier string `json:"verifier"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, ok := s.pending[body.ID]
		if !ok {
			fail(w, 410)
			return
		}
		if p.Origin != from || subtle.ConstantTimeCompare([]byte(hash(body.Verifier)), []byte(p.Challenge)) != 1 {
			fail(w, 403)
			return
		}
		if p.SteamID == "" {
			reply(w, 202, map[string]string{"state": "pending"})
			return
		}
		if p.SteamID != s.cfg.Store.View().SelectedSteamID {
			delete(s.pending, body.ID)
			fail(w, 410)
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
		delete(s.pending, body.ID)
		reply(w, 200, map[string]string{"token": token, "steam_id": p.SteamID, "instance_id": s.cfg.Store.View().InstanceID})
	case "GET /v1/snapshot", "DELETE /v1/pairing":
		pair, proof, ok := s.authorized(r)
		if !ok {
			fail(w, 401)
			return
		}
		if r.Method == "DELETE" {
			err := s.cfg.Store.Update(func(c *config.Config) error {
				var keep []config.Pairing
				for _, p := range c.Pairings {
					if p.Hash != pair.Hash {
						keep = append(keep, p)
					}
				}
				c.Pairings = keep
				return nil
			})
			if err != nil {
				fail(w, 500)
				return
			}
			proofWrite(w, proof, 200, []byte(`{"revoked":true}`))
			return
		}
		value := s.provider.Snapshot(pair.SteamID)
		if value.Snapshot != nil {
			if value.Snapshot.SteamID != pair.SteamID {
				fail(w, 500)
				return
			}
			copy := *value.Snapshot
			copy.InstanceID = s.cfg.Store.View().InstanceID
			value.Snapshot = &copy
		}
		data, err := json.Marshal(value)
		if err != nil || len(data) > protocol.MaxBytes {
			fail(w, 500)
			return
		}
		proofWrite(w, proof, 200, data)
	default:
		fail(w, 404)
	}
}
