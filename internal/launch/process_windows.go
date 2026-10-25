package launch

import (
	"os/exec"
	"syscall"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"golang.org/x/sys/windows"
)

const waitAfterChildExit = true

func configureChild(command *exec.Cmd) {
	// No console is inherited or created; the daemon also has its own process
	// group, independent of the launcher's lifetime. Nil streams stay detached.
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
}
func requestReady(options Options) error {
	return app.RequestReadyFor(options.ConfigDir, options.Executable)
}
func requestSettings(options Options) error {
	return app.RequestOpenFor(options.ConfigDir, options.Executable)
}
