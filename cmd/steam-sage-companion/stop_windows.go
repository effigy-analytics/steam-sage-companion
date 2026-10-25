package main

import (
	"errors"
	"github.com/effigy-analytics/steam-sage-companion/internal/app"
)

func stopCompanion(dir, executable string) error { return app.RequestStopAndWait(dir, executable) }

func stopExitCode(err error) int {
	if errors.Is(err, app.ErrNotRunning) {
		return 3
	}
	return 1
}
