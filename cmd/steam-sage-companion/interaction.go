package main

import (
	"fmt"
	"os"

	"github.com/effigy-analytics/steam-sage-companion/internal/launch"
)

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
