package platform

import (
	"os/exec"
	"syscall"
)

// OpenBrowserObserved reports dispatch and eventual OS dispatcher exit separately.
// A slow default browser is never killed or treated as proof of window visibility.
func OpenBrowserObserved(url string, exited func(error)) error {
	command := exec.Command("xdg-open", url)
	// A launcher may return just before its terminal closes. Keep the OS
	// dispatcher out of that terminal session while still observing its exit.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return startBrowser(command, exited)
}
