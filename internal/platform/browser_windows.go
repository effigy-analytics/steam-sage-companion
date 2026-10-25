package platform

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

var browserCommand = func(url string) *exec.Cmd { return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url) }

func OpenBrowserObserved(url string, exited func(error)) error {
	command := browserCommand(url)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	return startBrowser(command, exited)
}
