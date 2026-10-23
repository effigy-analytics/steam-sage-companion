package install

import (
	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallPreservesChangedAndUnrelatedFiles(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "isolated-config"))
	paths := Paths{Executable: filepath.Join(base, "bin", "companion"), Desktop: filepath.Join(base, "applications", "companion.desktop"), ConfigDir: filepath.Join(base, "config")}
	source := filepath.Join(base, "source")
	os.WriteFile(source, []byte("synthetic binary"), 0700)
	if err := Install(source, paths); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(paths.Desktop, []byte("my edited launcher"), 0600)
	keep := filepath.Join(paths.ConfigDir, "keep.txt")
	os.WriteFile(keep, []byte("unrelated"), 0600)
	kept, err := Uninstall(paths, true)
	if err != nil || !kept {
		t.Fatalf("changed files not reported: %v", err)
	}
	for _, file := range []string{paths.Desktop, keep} {
		if _, err := os.Stat(file); err != nil {
			t.Fatal("uninstall removed an unowned file")
		}
	}
	if _, err := os.Stat(paths.Executable); !os.IsNotExist(err) {
		t.Fatal("owned executable was retained")
	}
}
func TestInstallRefusesToOverwriteAnUnownedExecutable(t *testing.T) {
	base := t.TempDir()
	paths := Paths{Executable: filepath.Join(base, "existing"), Desktop: filepath.Join(base, "launcher"), ConfigDir: filepath.Join(base, "config")}
	os.WriteFile(paths.Executable, []byte("someone else's tool"), 0700)
	source := filepath.Join(base, "source")
	os.WriteFile(source, []byte("our helper"), 0700)
	if Install(source, paths) == nil {
		t.Fatal("overwrote an unowned executable")
	}
	data, _ := os.ReadFile(paths.Executable)
	if string(data) != "someone else's tool" {
		t.Fatal("unowned executable changed")
	}
}

func TestUpgradeRepairsEnabledStartupToInstalledExecutable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	paths := Paths{Executable: filepath.Join(base, "bin", "companion"), Desktop: filepath.Join(base, "apps", "companion.desktop"), ConfigDir: filepath.Join(base, "config", "companion")}
	source := filepath.Join(base, "download", "helper")
	os.MkdirAll(filepath.Dir(source), 0700)
	os.WriteFile(source, []byte("binary"), 0700)
	store, _ := config.Open(paths.ConfigDir)
	store.Update(func(c *config.Config) error { c.Startup = true; return nil })
	platform.SetStartup(true, source, paths.ConfigDir)
	if err := Install(source, paths); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(base, "config", "autostart", "steam-sage-companion.desktop"))
	if strings.Contains(string(data), source) || !strings.Contains(string(data), paths.Executable) {
		t.Fatal("upgrade retained downloaded executable in startup")
	}
}

func TestControlsFromDownloadUseOwnedInstalledStartupPath(t *testing.T) {
	base := t.TempDir()
	paths := Paths{Executable: filepath.Join(base, "bin", "companion"), Desktop: filepath.Join(base, "apps", "companion.desktop"), ConfigDir: filepath.Join(base, "config")}
	source := filepath.Join(base, "download")
	os.WriteFile(source, []byte("binary"), 0700)
	if err := Install(source, paths); err != nil {
		t.Fatal(err)
	}
	if StartupExecutable(source, paths) != paths.Executable {
		t.Fatal("controls retained download path")
	}
}
