package platform

import (
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func SetStartup(enabled bool, executable string, configDir ...string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err = key.DeleteValue("SteamSageCompanion")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	args := syscall.EscapeArg(executable) + " serve --background"
	if len(configDir) > 0 {
		args += " --config-dir " + syscall.EscapeArg(configDir[0])
	}
	return key.SetStringValue("SteamSageCompanion", args)
}
