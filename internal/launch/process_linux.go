package launch

import (
	"os/exec"
	"syscall"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
)

func configureChild(command *exec.Cmd)      { command.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func requestReady(options Options) error    { return app.RequestReady(options.ConfigDir) }
func requestSettings(options Options) error { return app.RequestOpen(options.ConfigDir) }
