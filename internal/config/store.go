// Package config persists per-user companion settings and derived pairing credentials.
package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Pairing struct {
	Hash    string `json:"hash"`
	Origin  string `json:"origin"`
	SteamID string `json:"steam_id"`
}
type Config struct {
	Kind            string            `json:"kind"`
	InstanceID      string            `json:"instance_id"`
	SelectedAccount string            `json:"selected_account"`
	SelectedSteamID string            `json:"selected_steam_id"`
	Roots           []string          `json:"roots,omitempty"`
	Sources         map[string]string `json:"sources"`
	Pairings        []Pairing         `json:"pairings"`
	Startup         bool              `json:"startup"`
}

func Load(dir string) (Config, error) {
	path := filepath.Join(dir, "settings.json")
	if err := validatePrivateLoad(dir, path); err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c := Config{Kind: "steam-sage-companion", InstanceID: rand.Text(), Sources: map[string]string{}, Pairings: []Pairing{}}
		return c, Save(dir, c)
	}
	if err != nil || len(data) > 1024*1024 {
		return Config{}, errors.New("cannot read companion settings")
	}
	var c Config
	if json.Unmarshal(data, &c) != nil || c.InstanceID == "" || c.Kind != "steam-sage-companion" {
		return Config{}, errors.New("companion settings are damaged; restore or explicitly reset them")
	}
	if c.Sources == nil {
		c.Sources = map[string]string{}
	}
	return c, nil
}
func Save(dir string, c Config) error {
	c.Kind = "steam-sage-companion"
	if err := ensurePrivateDir(dir); err != nil {
		return errors.New("cannot create settings directory")
	}
	if err := validatePrivateLoad(dir, filepath.Join(dir, "settings.json")); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return errors.New("cannot save companion settings")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = protectPrivateFile(f); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.New("cannot save companion settings")
	}
	if err = replacePrivateFile(tmp, filepath.Join(dir, "settings.json")); err != nil {
		return errors.New("cannot replace companion settings")
	}
	return nil
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data Config
}

func Open(dir string) (*Store, error) {
	c, err := Load(dir)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, data: c}, nil
}
func clone(c Config) Config {
	data, _ := json.Marshal(c)
	var copy Config
	json.Unmarshal(data, &copy)
	return copy
}
func (s *Store) View() Config { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.data) }
func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.data)
	if err := fn(&next); err != nil {
		return err
	}
	if err := Save(s.dir, next); err != nil {
		return err
	}
	s.data = next
	return nil
}
