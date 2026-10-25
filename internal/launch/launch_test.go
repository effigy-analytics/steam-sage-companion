package launch

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
)

func TestLaunchReusesReadinessAndOnlyOpensRequestedDestination(t *testing.T) {
	for _, destination := range []Destination{Sage, Settings, None} {
		t.Run(map[Destination]string{Sage: "Sage", Settings: "Settings", None: "None"}[destination], func(t *testing.T) {
			dir := t.TempDir()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var controls atomic.Int32
			listener, err := app.ListenActivation(ctx, dir, func() error { controls.Add(1); return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			var urls []string
			if err = run(Options{Executable: executable, ConfigDir: dir, Destination: destination}, func(url string) error { urls = append(urls, url); return nil }); err != nil {
				t.Fatal(err)
			}
			if destination == Settings {
				if controls.Load() != 1 {
					t.Fatal("Settings did not open exactly one private control")
				}
			} else if controls.Load() != 0 {
				t.Fatal("readiness opened controls")
			}
			if destination == Sage {
				if len(urls) != 1 || urls[0] != "https://steam-sage.effigy-analytics.com/companion?installed=1" {
					t.Fatalf("incorrect Sage destination: %q", urls)
				}
			} else if len(urls) != 0 {
				t.Fatal("Settings or None opened a website")
			}
		})
	}
}
func TestLaunchBrowserDispatchFailureLeavesOwnerReady(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := app.ListenActivation(ctx, dir, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	options := Options{Executable: executable, ConfigDir: dir}
	if err = run(options, func(string) error { return errors.New("synthetic dispatch failure") }); err == nil {
		t.Fatal("browser failure was lost")
	}
	if err = requestReady(options); err != nil {
		t.Fatal("browser failure stopped an existing owner:", err)
	}
}
