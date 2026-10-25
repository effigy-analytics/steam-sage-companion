// Package launch starts or reuses the owned helper before returning to Sage.
package launch

import (
	"errors"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
)

// Run never treats an HTTP listener as proof of an owned companion.
func Run(options Options) error { return run(options, platform.OpenBrowser) }

func run(options Options, openBrowser func(string) error) error {
	if !filepath.IsAbs(options.Executable) || !filepath.IsAbs(options.ConfigDir) {
		return errors.New("launch needs absolute executable and settings paths")
	}
	if options.Destination > None {
		return errors.New("unknown launch destination")
	}
	if requestReady(options) == nil {
		return openDestination(options, openBrowser)
	}
	args := []string{"serve", "--background", "--config-dir", options.ConfigDir}
	if options.ExtraOrigin != "" {
		args = append(args, "--allow-origin", options.ExtraOrigin)
	}
	command := exec.Command(options.Executable, args...)
	configureChild(command)
	// Nil streams detach from the installer's terminal and output pipes.
	if err := command.Start(); err != nil {
		return errors.New("cannot start the installed companion; reinstall it and try again")
	}
	exited := make(chan error, 1)
	go func(result chan<- error) { result <- command.Wait() }(exited)
	childExited := false
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	probes := time.NewTicker(50 * time.Millisecond)
	defer probes.Stop()
	for {
		select {
		case <-exited:
			childExited = true
			exited = nil
			// Another simultaneous launch may have won the activation socket.
			if requestReady(options) == nil {
				return openDestination(options, openBrowser)
			}
			if !waitAfterChildExit {
				return errors.New("companion could not start; its local port may be occupied or its settings unavailable")
			}
			// Windows can reject the losing child while the owned winner is
			// still initializing. Keep probing within the original timer budget.
		case <-timeout.C:
			if childExited {
				return errors.New("companion could not start; its local port may be occupied or its settings unavailable")
			}
			// Only this launcher's child may be terminated, never an existing listener.
			command.Process.Kill()
			<-exited
			return errors.New("companion did not become ready; check its settings or local port and try again")
		case <-probes.C:
			if requestReady(options) == nil {
				return openDestination(options, openBrowser)
			}
		}
	}
}
func openDestination(options Options, openBrowser func(string) error) error {
	switch options.Destination {
	case None:
		return nil
	case Settings:
		if err := requestSettings(options); err != nil {
			return errors.New("companion is ready, but private settings could not open; try the Companion Settings shortcut again")
		}
		return nil
	}
	if err := openBrowser("https://steam-sage.effigy-analytics.com/companion?installed=1"); err != nil {
		return errors.New("companion is ready, but Sage could not open; open https://steam-sage.effigy-analytics.com/companion in your browser")
	}
	return nil
}
