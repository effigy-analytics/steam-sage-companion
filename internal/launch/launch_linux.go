// Package launch starts or reuses the owned helper before returning to Sage.
package launch

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
)

type Options struct{ Executable, ConfigDir, ExtraOrigin string }

// Run never treats an HTTP listener as proof of an owned companion.
func Run(options Options) error {
	if !filepath.IsAbs(options.Executable) || !filepath.IsAbs(options.ConfigDir) {
		return errors.New("launch needs absolute executable and settings paths")
	}
	if app.RequestReady(options.ConfigDir) == nil {
		return openSage()
	}
	args := []string{"serve", "--background", "--config-dir", options.ConfigDir}
	if options.ExtraOrigin != "" {
		args = append(args, "--allow-origin", options.ExtraOrigin)
	}
	command := exec.Command(options.Executable, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Nil streams detach from the installer's terminal and output pipes.
	if err := command.Start(); err != nil {
		return errors.New("cannot start the installed companion; reinstall it and try again")
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	probes := time.NewTicker(50 * time.Millisecond)
	defer probes.Stop()
	for {
		select {
		case <-exited:
			// Another simultaneous launch may have won the activation socket.
			if app.RequestReady(options.ConfigDir) == nil {
				return openSage()
			}
			return errors.New("companion could not start; its local port may be occupied or its settings unavailable")
		case <-timeout.C:
			// Only this launcher's child may be terminated, never an existing listener.
			command.Process.Kill()
			<-exited
			return errors.New("companion did not become ready; check its settings or local port and try again")
		case <-probes.C:
			if app.RequestReady(options.ConfigDir) == nil {
				return openSage()
			}
		}
	}
}
func openSage() error {
	if err := platform.OpenBrowser("https://steam-sage.effigy-analytics.com/companion?installed=1"); err != nil {
		return errors.New("companion is ready, but Sage could not open; open https://steam-sage.effigy-analytics.com/companion in your browser")
	}
	return nil
}
