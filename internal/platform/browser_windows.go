package platform

import "os/exec"

var browserCommand = func(url string) *exec.Cmd { return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url) }

func OpenBrowserObserved(url string, exited func(error)) error {
	return startBrowser(browserCommand(url), exited)
}
