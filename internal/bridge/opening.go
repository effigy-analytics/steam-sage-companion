package bridge

import (
	"encoding/json"
	"net/http"
	"time"
)

type ApprovalUI string

const (
	Manual     ApprovalUI = "manual"
	Opening    ApprovalUI = "opening"
	Dispatched ApprovalUI = "dispatched"
	Failed     ApprovalUI = "failed"
)

type opening struct {
	State         ApprovalUI
	Generation    uint64
	Last, Expires time.Time
}

func (s *Server) reapOpenings(now time.Time) {
	for id, r := range s.receipts {
		if !now.Before(r.Expires) {
			delete(s.receipts, id)
		}
	}
	for key, o := range s.settings {
		if !now.Before(o.Expires) {
			delete(s.settings, key)
		}
	}
	recent := s.openAttempts[:0]
	for _, t := range s.openAttempts {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	s.openAttempts = recent
}
func (s *Server) openRetry(o *opening) time.Duration {
	var retry time.Duration
	if !o.Last.IsZero() {
		retry = o.Last.Add(5 * time.Second).Sub(s.now())
	}
	if len(s.openAttempts) >= 5 {
		global := s.openAttempts[0].Add(time.Minute).Sub(s.now())
		if global > retry {
			retry = global
		}
	}
	if retry < 0 {
		return 0
	}
	return retry
}
func openingThrottle(w http.ResponseWriter, proof proofChallenge, retry time.Duration) {
	ms := (retry + time.Millisecond - 1) / time.Millisecond
	if ms > 60000 {
		ms = 60000
	}
	if ms < 1 {
		ms = 1
	}
	value := map[string]any{"error": "Too Many Requests", "retry_after_ms": ms}
	if proof.ID == "" {
		reply(w, 429, value)
		return
	}
	signedJSON(w, proof, 429, value)
}
func signedJSON(w http.ResponseWriter, proof proofChallenge, status int, value any) {
	data, _ := json.Marshal(value)
	proofWrite(w, proof, status, data)
}
func (s *Server) settingsOpen(w http.ResponseWriter, r *http.Request) {
	pair, proof, ok := s.authorized(r)
	if !ok {
		fail(w, 401)
		return
	}
	o := s.settings[pair.Hash]
	if r.Method == "GET" {
		state := Manual
		if o != nil {
			state = o.State
		}
		signedJSON(w, proof, 200, map[string]ApprovalUI{"state": state})
		return
	}
	if s.cfg.OpenBrowser == nil {
		signedJSON(w, proof, 202, map[string]ApprovalUI{"state": Manual})
		return
	}
	if o == nil {
		o = &opening{State: Manual}
	}
	if retry := s.openRetry(o); retry > 0 {
		openingThrottle(w, proof, retry)
		return
	}
	if s.settings[pair.Hash] == nil && len(s.settings) >= 64 {
		var oldest string
		for key, record := range s.settings {
			if oldest == "" || record.Expires.Before(s.settings[oldest].Expires) {
				oldest = key
			}
		}
		delete(s.settings, oldest)
	}
	s.settings[pair.Hash] = o
	s.scheduleOpen("", pair.Hash, o, s.now().Add(120*time.Second))
	signedJSON(w, proof, 202, map[string]ApprovalUI{"state": Opening})
}

// Called with the server lock held. Dispatch workers first reacquire that lock,
// so the HTTP handler always releases it before any browser code can run.
func (s *Server) scheduleOpen(requestID, pairHash string, o *opening, expires time.Time) {
	s.openGeneration++
	gen := s.openGeneration
	o.Generation = gen
	o.State = Opening
	o.Last = s.now()
	o.Expires = expires
	s.openAttempts = append(s.openAttempts, s.now())
	go func() {
		s.mu.Lock()
		current := func() bool {
			if !s.now().Before(expires) {
				return false
			}
			if requestID != "" {
				p := s.pending[requestID]
				return p != nil && &p.Opening == o && o.Generation == gen
			}
			if s.settings[pairHash] != o || o.Generation != gen {
				return false
			}
			for _, p := range s.cfg.Store.View().Pairings {
				if p.Hash == pairHash && p.SteamID == s.cfg.Store.View().SelectedSteamID {
					return true
				}
			}
			return false
		}
		if !current() {
			s.mu.Unlock()
			return
		}
		u := s.newControlURLLocked(requestID)
		dispatch := s.cfg.OpenBrowser
		s.mu.Unlock()
		finish := func(err error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if current() {
				if err != nil {
					o.State = Failed
				} else if o.State != Failed {
					o.State = Dispatched
				}
			}
		}
		err := dispatch(u, finish)
		finish(err)
	}()
}
