package platform

import "os/exec"

func OpenBrowserObserved(url string, exited func(error)) error {
	return startBrowser(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url), exited)
}
