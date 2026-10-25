package main

import "github.com/effigy-analytics/steam-sage-companion/internal/launch"

func launchDestination(command string) (launch.Destination, bool) {
	return launch.Sage, command == "launch"
}

func interactiveCommand(string, bool) bool { return false }
