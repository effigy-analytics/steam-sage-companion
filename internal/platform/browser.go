package platform

import "os/exec"

// OpenBrowser preserves the existing fire-and-forget API.
func OpenBrowser(url string) error { return OpenBrowserObserved(url, nil) }

func startBrowser(command *exec.Cmd, exited func(error)) error {
	if err := command.Start(); err != nil {
		return err
	}
	go func() {
		err := command.Wait()
		if exited != nil {
			exited(err)
		}
	}()
	return nil
}
