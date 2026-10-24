// Package bridge serves the paired, read-only loopback protocol.
package bridge

import (
	"crypto/rand"
	"crypto/sha256"
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
	OpenBrowser  func(url string, exited func(error)) error
	OnStop       func()
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
	receipts           map[string]*receipt
	settings           map[string]*opening
	openAttempts       []time.Time
	openGeneration     uint64
}

func New(cfg Config, provider SnapshotProvider) *Server {
	return &Server{cfg: cfg, provider: provider, now: time.Now, pending: map[string]*pending{}, receipts: map[string]*receipt{}, settings: map[string]*opening{}, proofs: map[string]proofChallenge{}, launches: map[string]time.Time{}, controls: map[string]time.Time{}}
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
	return s.newControlURLLocked("")
}
func (s *Server) newControlURLLocked(requestID string) string {
	cap := secret()
	s.launches[hash(cap)] = s.now().Add(5 * time.Minute)
	u := LocalOrigin + "/#launch=" + cap
	if requestID != "" {
		u += "&request_id=" + requestID
	}
	return u
}
func (s *Server) reap() {
	now := s.now()
	s.reapOpenings(now)
	for id, p := range s.proofs {
		if !now.Before(p.Expires) {
			delete(s.proofs, id)
		}
	}
	for id, p := range s.pending {
		if !now.Before(p.Expires) {
			s.finishPairing(id, p, "expired", "")
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
	switch r.Method + " " + r.URL.Path {
	case "GET /v1/status":
		caps := []string{"pairing-cancel-v1"}
		if s.cfg.OpenBrowser != nil {
			caps = append(caps, "pairing-ui-v1", "settings-ui-v1")
		}
		reply(w, 200, map[string]any{"protocol": 1, "helper_version": s.cfg.Version, "instance_id": s.cfg.Store.View().InstanceID, "capabilities": caps})
	case "POST /v1/challenge":
		s.challenge(w, r)
	case "POST /v1/pair/request", "POST /v1/pair/claim", "POST /v1/pair/open", "POST /v1/pair/cancel":
		s.pairing(w, r)
	case "GET /v1/settings/open", "POST /v1/settings/open":
		s.settingsOpen(w, r)
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
