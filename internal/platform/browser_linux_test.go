package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserObservedReportsImmediateFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := OpenBrowserObserved("https://example.invalid", nil); err == nil {
		t.Fatal("missing browser dispatcher was accepted")
	}
}
func TestBrowserObservedReportsLateFailureAndDoesNotKillSlowBrowser(t *testing.T) {
	tools := t.TempDir()
	t.Setenv("PATH", tools)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "disabled:")
	completed := filepath.Join(tools, "completed")
	t.Setenv("STEAM_SAGE_TEST_COMPLETED", completed)
	if err := os.WriteFile(filepath.Join(tools, "xdg-open"), []byte("#!/bin/sh\n/bin/sleep 2.1\nprintf done > \"$STEAM_SAGE_TEST_COMPLETED\"\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	start := time.Now()
	if err := OpenBrowserObserved("https://example.invalid", func(err error) { exited <- err }); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("browser dispatch waited for a slow browser")
	}
	select {
	case err := <-exited:
		if err == nil {
			t.Fatal("late browser failure was discarded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("browser exit was not observed")
	}
	if data, err := os.ReadFile(completed); err != nil || string(data) != "done" {
		t.Fatal("slow browser was killed")
	}
}
