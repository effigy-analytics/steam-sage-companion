package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var nativeErrorMessage = func(message string) {
	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, err := windows.UTF16PtrFromString("Steam Sage Companion")
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

func showCommandError(command string, background bool, message string) {
	// Maintenance, startup, and CI use stderr/reports and exit codes. A blocking
	// desktop dialog belongs only to an explicit interactive launch/settings/open.
	if !interactiveCommand(command, background) || os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") == "true" {
		return
	}
	nativeErrorMessage(message)
}
