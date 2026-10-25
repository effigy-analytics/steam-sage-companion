package main

import (
	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
)

func setStartupPreference(enabled bool, executable, dir string, store *config.Store) error {
	return platform.SetStartupTransaction(enabled, executable, dir, func() error {
		return store.Update(func(c *config.Config) error { c.Startup = enabled; return nil })
	})
}
