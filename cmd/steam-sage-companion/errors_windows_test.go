package main

import "testing"

func TestWindowsErrorPresentationIsSilentForBackgroundAndCI(t *testing.T) {
	old := nativeErrorMessage
	defer func() { nativeErrorMessage = old }()
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	var messages []string
	nativeErrorMessage = func(s string) { messages = append(messages, s) }
	showCommandError("settings", false, "synthetic interactive failure")
	if len(messages) != 1 || messages[0] != "synthetic interactive failure" {
		t.Fatal("interactive GUI failure was not presented")
	}
	for _, command := range []string{"launch", "settings", "open", "serve", "stop", "package-check"} {
		showCommandError(command, true, "silent failure")
	}
	showCommandError("serve", false, "serve failure")
	showCommandError("stop", false, "stop failure")
	showCommandError("package-check", false, "inspection failure")
	if len(messages) != 1 {
		t.Fatal("silent operation opened blocking error dialog")
	}
	t.Setenv("CI", "true")
	showCommandError("settings", false, "CI failure")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	showCommandError("launch", false, "CI failure")
	if len(messages) != 1 {
		t.Fatal("CI helper operation opened blocking error dialog")
	}
}
