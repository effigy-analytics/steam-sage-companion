// Package install manages only the companion's own per-user files.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
)

type Paths struct{ Executable, Desktop, ConfigDir string }
type manifest struct {
	Kind           string `json:"kind"`
	ExecutableHash string `json:"executable_hash"`
	DesktopHash    string `json:"desktop_hash"`
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(f, 64*1024*1024)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func readManifest(dir string) (manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "install.json"))
	if os.IsNotExist(err) {
		return manifest{}, nil
	}
	var m manifest
	if err != nil || len(raw) > 8192 || json.Unmarshal(raw, &m) != nil || m.Kind != "steam-sage-companion" {
		return m, errors.New("installation record is damaged; no files were removed")
	}
	return m, nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("cannot create installation directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".companion-*")
	if err != nil {
		return errors.New("cannot create installation file")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
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
		return errors.New("cannot write installation file")
	}
	if os.Rename(tmp, path) != nil {
		return errors.New("cannot replace installation file; stop the helper and run installation from the downloaded copy")
	}
	return nil
}
func stopOwned(dir string) {
	if app.RequestStop(dir) == nil {
		time.Sleep(100 * time.Millisecond)
	}
}
func Install(source string, paths Paths) error {
	previous, err := readManifest(paths.ConfigDir)
	if err != nil {
		return err
	}
	settings, err := config.Load(paths.ConfigDir)
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(source)
	if err != nil || len(binary) > 64*1024*1024 {
		return errors.New("cannot read companion executable")
	}
	desktop := desktopEntry(paths)
	for _, file := range []struct{ path, old, wanted string }{{paths.Executable, previous.ExecutableHash, digest(binary)}, {paths.Desktop, previous.DesktopHash, digest(desktop)}} {
		if current, err := hashFile(file.path); err == nil && current != file.old && current != file.wanted {
			return errors.New("an installation file was changed; leaving it in place")
		} else if err != nil && !os.IsNotExist(err) {
			return errors.New("cannot inspect installation files")
		}
	}
	stopOwned(paths.ConfigDir)
	if err := atomicWrite(paths.Executable, binary, 0755); err != nil {
		return err
	}
	if err := atomicWrite(paths.Desktop, desktop, 0644); err != nil {
		return err
	}
	if settings.Startup {
		if err := platform.SetStartup(true, paths.Executable, paths.ConfigDir); err != nil {
			return err
		}
	}
	record, _ := json.Marshal(manifest{Kind: "steam-sage-companion", ExecutableHash: digest(binary), DesktopHash: digest(desktop)})
	return atomicWrite(filepath.Join(paths.ConfigDir, "install.json"), record, 0600)
}
func Uninstall(paths Paths, purge bool) (bool, error) {
	m, err := readManifest(paths.ConfigDir)
	if err != nil {
		return false, err
	}
	stopOwned(paths.ConfigDir)
	kept := false
	if err := platform.SetStartup(false, paths.Executable, paths.ConfigDir); err != nil {
		kept = true
	}
	for _, file := range []struct{ path, hash string }{{paths.Executable, m.ExecutableHash}, {paths.Desktop, m.DesktopHash}} {
		current, err := hashFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || file.hash == "" || current != file.hash {
			kept = true
			continue
		}
		if os.Remove(file.path) != nil {
			kept = true
		}
	}
	if m.Kind != "" && !kept {
		os.Remove(filepath.Join(paths.ConfigDir, "install.json"))
	}
	if purge {
		file := filepath.Join(paths.ConfigDir, "settings.json")
		if data, err := os.ReadFile(file); err == nil {
			var settings config.Config
			if len(data) <= 1024*1024 && json.Unmarshal(data, &settings) == nil && settings.Kind == "steam-sage-companion" {
				if os.Remove(file) != nil {
					kept = true
				}
			} else {
				kept = true
			}
		}
		os.Remove(paths.ConfigDir)
	}
	return kept, nil
}

// StartupExecutable uses the owned installed copy even when controls were opened
// from an extracted archive. Portable, uninstalled runs keep their own path.
func StartupExecutable(current string, paths Paths) string {
	m, err := readManifest(paths.ConfigDir)
	if err == nil && m.ExecutableHash != "" {
		if hash, err := hashFile(paths.Executable); err == nil && hash == m.ExecutableHash {
			return paths.Executable
		}
	}
	return current
}
