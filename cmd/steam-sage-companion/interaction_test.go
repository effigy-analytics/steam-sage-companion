package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/effigy-analytics/steam-sage-companion/internal/launch"
)

func TestExplicitConfigDoesNotResolveAmbientProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "owned synthetic config")
	got, err := commandConfigDir(dir, func() (string, error) { return "", errors.New("ambient profile unavailable") })
	if err != nil || got != dir {
		t.Fatal("explicit config still depended on ambient profile:", err)
	}
	base := t.TempDir()
	got, err = commandConfigDir("", func() (string, error) { return base, nil })
	if err != nil || got != filepath.Join(base, "steam-sage-companion") {
		t.Fatal("default per-user config changed")
	}
}

func TestSettingsUsesDetachedLaunchDestination(t *testing.T) {
	for _, tc := range []struct {
		command     string
		destination launch.Destination
		detached    bool
	}{{"launch", launch.Sage, true}, {"settings", launch.Settings, true}, {"open", launch.Sage, false}, {"serve", launch.Sage, false}, {"stop", launch.Sage, false}} {
		destination, ok := launchDestination(tc.command)
		if ok != tc.detached || destination != tc.destination {
			t.Errorf("%s: destination %d detached %v", tc.command, destination, ok)
		}
	}
}
func TestOnlyInteractiveCommandsPresentNativeErrors(t *testing.T) {
	for _, tc := range []struct {
		command          string
		background, want bool
	}{{"launch", false, true}, {"settings", false, true}, {"open", false, true}, {"launch", true, false}, {"settings", true, false}, {"serve", false, false}, {"serve", true, false}, {"stop", false, false}, {"package-check", false, false}, {"inspect", false, false}, {"version", false, false}} {
		if got := interactiveCommand(tc.command, tc.background); got != tc.want {
			t.Errorf("command %s background %v: show error %v", tc.command, tc.background, got)
		}
	}
}
