package platform

import "os/exec"

// OpenBrowserObserved reports dispatch and eventual OS dispatcher exit separately.
// A slow default browser is never killed or treated as proof of window visibility.
func OpenBrowserObserved(url string, exited func(error)) error {
	return startBrowser(exec.Command("xdg-open", url), exited)
}
