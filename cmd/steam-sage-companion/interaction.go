package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/effigy-analytics/steam-sage-companion/internal/launch"
)

func commandConfigDir(dir string, userConfigDir func() (string, error)) (string, error) {
	if dir != "" {
		return dir, nil
	}
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "steam-sage-companion"), nil
}

func launchDestination(command string) (launch.Destination, bool) {
	if command == "settings" {
		return launch.Settings, true
	}
	return launch.Sage, command == "launch"
}

func interactiveCommand(command string, background bool) bool {
	return !background && (command == "launch" || command == "settings" || command == "open")
}
func reportCommandError(command string, background bool, err error) {
	fmt.Fprintln(os.Stderr, err)
	showCommandError(command, background, err.Error())
}
