package bridge

import (
	"net/http"
	"sort"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/config"
)

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	from := r.Header.Get("Origin")
	if from != LocalOrigin && !(r.Method == "GET" && from == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin") {
		fail(w, 403)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/control/session" {
		var body struct {
			Capability string `json:"capability"`
		}
		if !decode(w, r, &body) {
			return
		}
		key := hash(body.Capability)
		expires, ok := s.launches[key]
		if !ok || !s.now().Before(expires) {
			fail(w, 403)
			return
		}
		delete(s.launches, key)
		token := secret()
		s.controls[hash(token)] = s.now().Add(30 * time.Minute)
		reply(w, 200, map[string]string{"control_token": token})
		return
	}
	expires, ok := s.controls[hash(r.Header.Get("X-Companion-Control"))]
	if !ok || !s.now().Before(expires) {
		fail(w, 401)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /control/state":
		focus := r.URL.Query().Get("request_id")
		if len(focus) > 64 {
			fail(w, 400)
			return
		}
		type item struct {
			ID      string `json:"request_id"`
			Origin  string `json:"origin"`
			Expires string `json:"expires_at"`
		}
		items := []item{}
		for id, p := range s.pending {
			if p.SteamID == "" {
				items = append(items, item{id, p.Origin, p.Expires.UTC().Format(time.RFC3339)})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		c := s.cfg.Store.View()
		accounts := []AccountView{}
		if s.cfg.Accounts != nil {
			accounts = s.cfg.Accounts()
		}
		origins := []string{}
		for _, p := range c.Pairings {
			origins = append(origins, p.Origin)
		}
		value := map[string]any{"pending": items, "accounts": accounts, "selected_steam_id": c.SelectedSteamID, "paired_origins": origins, "startup": c.Startup}
		if focus != "" {
			type focused struct {
				ID      string `json:"request_id"`
				Origin  string `json:"origin"`
				Expires string `json:"expires_at"`
				State   string `json:"state"`
			}
			if p := s.pending[focus]; p != nil {
				state := "pending"
				if p.SteamID != "" {
					state = "approved"
				}
				value["focused_request"] = focused{focus, p.Origin, p.Expires.UTC().Format(time.RFC3339), state}
			} else if rec := s.receipts[focus]; rec != nil {
				value["focused_request"] = focused{focus, rec.Origin, rec.RequestExpires.UTC().Format(time.RFC3339), rec.State}
			}
		}
		if c.SelectedSteamID != "" {
			value["source"] = s.provider.Snapshot(c.SelectedSteamID)
		}
		reply(w, 200, value)
	case "POST /control/decline":
		var body struct {
			ID string `json:"request_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, ok := s.pending[body.ID]
		if !ok {
			fail(w, 410)
			return
		}
		s.finishPairing(body.ID, p, "declined", "")
		reply(w, 200, map[string]bool{"declined": true})
	case "POST /control/approve":
		var body struct {
			ID string `json:"request_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, ok := s.pending[body.ID]
		if !ok {
			fail(w, 410)
			return
		}
		id := s.cfg.Store.View().SelectedSteamID
		if id == "" {
			fail(w, 409)
			return
		}
		p.SteamID = id
		reply(w, 200, map[string]bool{"approved": true})
	case "POST /control/revoke":
		var body struct {
			Origin string `json:"origin"`
		}
		if !decode(w, r, &body) {
			return
		}
		err := s.cfg.Store.Update(func(c *config.Config) error {
			keep := []config.Pairing{}
			for _, p := range c.Pairings {
				if p.Origin != body.Origin {
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
		reply(w, 200, map[string]bool{"revoked": true})
	case "POST /control/select":
		var body struct {
			AccountID string `json:"account_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.cfg.OnSelect == nil || s.cfg.OnSelect(body.AccountID) != nil {
			fail(w, 400)
			return
		}
		reply(w, 200, map[string]bool{"selected": true})
	case "POST /control/startup":
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.cfg.OnStartup == nil || s.cfg.OnStartup(body.Enabled) != nil {
			fail(w, 500)
			return
		}
		reply(w, 200, map[string]bool{"saved": true})
	case "POST /control/choose-root":
		if s.cfg.OnChooseRoot == nil || s.cfg.OnChooseRoot() != nil {
			fail(w, 400)
			return
		}
		reply(w, 200, map[string]bool{"selected": true})
	case "POST /control/root":
		var body struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.cfg.OnSetRoot == nil || s.cfg.OnSetRoot(body.Path) != nil {
			fail(w, 400)
			return
		}
		reply(w, 200, map[string]bool{"selected": true})
	case "POST /control/stop":
		reply(w, 200, map[string]bool{"stopping": true})
		if s.cfg.OnStop != nil {
			go s.cfg.OnStop()
		}
	default:
		fail(w, 404)
	}
}
