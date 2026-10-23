// Package app coordinates the selected account and its last-good local data.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
	"github.com/effigy-analytics/steam-sage-companion/internal/steam"
)

type App struct {
	mu       sync.Mutex
	store    *config.Store
	roots    []string
	accounts []steam.Account
	selected *steam.Account
	watch    *steam.Watcher
	ctx      context.Context
	cancel   context.CancelFunc
}

func New(store *config.Store, roots []string) (*App, error) {
	a := &App{store: store, roots: roots}
	if err := a.discover(); err != nil {
		return nil, err
	}
	id := store.View().SelectedAccount
	if id == "" {
		id = store.View().SelectedSteamID
	}
	if id == "" && len(a.accounts) == 1 {
		id = accountID(a.accounts[0])
	}
	if id != "" {
		for _, account := range a.accounts {
			if accountID(account) == id || account.SteamID == id {
				matches := 0
				for _, candidate := range a.accounts {
					if candidate.SteamID == id || accountID(candidate) == id {
						matches++
					}
				}
				if matches == 1 {
					if err := a.SelectAccount(id); err != nil {
						return nil, err
					}
				}
				break
			}
		}
	}
	return a, nil
}
func (a *App) discover() error {
	roots := append(append([]string{}, a.roots...), a.store.View().Roots...)
	accounts, err := steam.Discover(roots)
	if err != nil {
		return err
	}
	a.accounts = accounts
	return nil
}
func accountID(account steam.Account) string {
	sum := sha256.Sum256([]byte(account.Root + "\x00" + account.SteamID))
	return hex.EncodeToString(sum[:])
}
func (a *App) SelectAccount(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var found *steam.Account
	for _, account := range a.accounts {
		if accountID(account) == id || account.SteamID == id {
			if found != nil {
				return errors.New("choose a specific Steam installation for this account")
			}
			copy := account
			found = &copy
		}
	}
	if found == nil {
		return errors.New("choose a discovered Steam account")
	}
	key := found.Root + "\x00" + found.SteamID
	if err := a.store.Update(func(c *config.Config) error {
		if c.SelectedSteamID != found.SteamID || c.SelectedAccount != accountID(*found) {
			c.Pairings = []config.Pairing{}
		}
		c.SelectedSteamID = found.SteamID
		c.SelectedAccount = accountID(*found)
		if c.Sources[key] == "" {
			c.Sources[key] = rand.Text()
		}
		return nil
	}); err != nil {
		return err
	}
	if a.cancel != nil {
		a.cancel()
	}
	a.selected = found
	a.watch = steam.NewWatcher(*found, nil)
	a.startWatch()
	return nil
}
func (a *App) startWatch() {
	if a.ctx != nil && a.watch != nil {
		ctx, cancel := context.WithCancel(a.ctx)
		a.cancel = cancel
		watch := a.watch
		go watch.Run(ctx)
	}
}
func (a *App) Run(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.startWatch()
	a.mu.Unlock()
	<-ctx.Done()
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
}
func (a *App) Accounts() []bridge.AccountView {
	a.mu.Lock()
	defer a.mu.Unlock()
	views := []bridge.AccountView{}
	for _, account := range a.accounts {
		views = append(views, bridge.AccountView{ID: accountID(account), Location: account.Root, SteamID: account.SteamID, Selected: a.selected != nil && accountID(account) == accountID(*a.selected)})
	}
	return views
}
func (a *App) Snapshot(id string) protocol.SnapshotResponse {
	a.mu.Lock()
	watch := a.watch
	selected := a.selected
	a.mu.Unlock()
	if watch == nil || selected == nil || selected.SteamID != id {
		return protocol.SnapshotResponse{Protocol: 1, State: "unavailable", AttemptedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	}
	watch.Refresh()
	value := watch.Current()
	if value.Snapshot != nil {
		c := a.store.View()
		value.Snapshot.InstanceID = c.InstanceID
		value.Snapshot.SourceID = c.Sources[selected.Root+"\x00"+id]
	}
	return value
}
func (a *App) AddRoot(root string) error {
	if !filepath.IsAbs(root) || len(root) > 4096 {
		return errors.New("choose a Steam installation folder")
	}
	accounts, err := steam.Discover([]string{root})
	if err != nil || len(accounts) == 0 {
		return errors.New("no Steam accounts found in this installation folder")
	}
	a.mu.Lock()
	if err := a.store.Update(func(c *config.Config) error {
		for _, old := range c.Roots {
			if old == root {
				return nil
			}
		}
		if len(c.Roots) >= 16 {
			return errors.New("too many Steam folders")
		}
		c.Roots = append(c.Roots, root)
		return nil
	}); err != nil {
		a.mu.Unlock()
		return err
	}
	err = a.discover()
	selected := a.selected
	single := ""
	if selected == nil && len(a.accounts) == 1 {
		single = accountID(a.accounts[0])
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if single != "" {
		return a.SelectAccount(single)
	}
	return nil
}
