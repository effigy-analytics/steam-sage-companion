package main

import "github.com/effigy-analytics/steam-sage-companion/internal/app"

func stopCompanion(dir, executable string) error { return app.RequestStop(dir) }
