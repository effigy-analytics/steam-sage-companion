package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func DesktopExec(executable string, args ...string) string {
	quote := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`, `%`, `%%`).Replace(s) + `"`
	}
	parts := []string{quote(executable)}
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}
func SetStartup(enabled bool, executable string, configDir ...string) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(base, "autostart")
	file := filepath.Join(dir, "steam-sage-companion.desktop")
	if old, err := os.ReadFile(file); err == nil && !strings.Contains(string(old), "# Managed by Steam Sage Companion") {
		return errors.New("startup entry was changed; leaving it in place")
	}
	if !enabled {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return errors.New("cannot remove companion startup entry")
		}
		return nil
	}
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\n\r") {
		return errors.New("invalid companion executable")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	args := []string{"serve", "--background"}
	if len(configDir) > 0 {
		if strings.ContainsAny(configDir[0], "\n\r") {
			return errors.New("invalid settings path")
		}
		args = append(args, "--config-dir", configDir[0])
	}
	return os.WriteFile(file, []byte("# Managed by Steam Sage Companion\n[Desktop Entry]\nType=Application\nName=Steam Sage Companion\nExec="+DesktopExec(executable, args...)+"\nTerminal=false\n"), 0600)
}
