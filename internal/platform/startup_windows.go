package platform

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

var startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
var startupMu sync.Mutex

const startupValueName = "SteamSageCompanion"

func startupCommand(executable string, configDir ...string) (string, error) {
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n") {
		return "", errors.New("invalid companion executable")
	}
	command := syscall.EscapeArg(executable) + " serve --background"
	if len(configDir) > 0 {
		if !filepath.IsAbs(configDir[0]) || strings.ContainsAny(configDir[0], "\x00\r\n") {
			return "", errors.New("invalid companion settings path")
		}
		command += " --config-dir " + syscall.EscapeArg(configDir[0])
	}
	return command, nil
}

type startupRegistration struct {
	present bool
	command string
}

func readStartup(key registry.Key) (startupRegistration, error) {
	value, kind, err := key.GetStringValue(startupValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return startupRegistration{}, nil
	}
	if err != nil {
		return startupRegistration{}, err
	}
	if kind != registry.SZ {
		return startupRegistration{}, errors.New("startup entry uses another registry value type; leaving it in place")
	}
	return startupRegistration{present: true, command: value}, nil
}
func writeStartup(key registry.Key, value startupRegistration) error {
	if value.present {
		return key.SetStringValue(startupValueName, value.command)
	}
	if err := key.DeleteValue(startupValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func SetStartup(enabled bool, executable string, configDir ...string) error {
	expected, err := startupCommand(executable, configDir...)
	if err != nil {
		return err
	}
	return setStartupTransaction(enabled, expected, func() error { return nil })
}

// SetStartupTransaction restores only the registration this call changed when
// settings persistence fails. An external change is preserved and reported.
func SetStartupTransaction(enabled bool, executable, configDir string, persist func() error) error {
	expected, err := startupCommand(executable, configDir)
	if err != nil {
		return err
	}
	return setStartupTransaction(enabled, expected, persist)
}
func setStartupTransaction(enabled bool, expected string, persist func() error) error {
	if persist == nil {
		return errors.New("startup settings persistence is unavailable")
	}
	startupMu.Lock()
	defer startupMu.Unlock()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	before, err := readStartup(key)
	if err != nil {
		return err
	}
	if before.present && before.command != expected {
		return errors.New("startup entry was changed; leaving it in place")
	}
	after := startupRegistration{present: enabled}
	if enabled {
		after.command = expected
	}
	if before != after {
		if err := writeStartup(key, after); err != nil {
			return err
		}
	}
	if err := persist(); err != nil {
		if before == after {
			return err
		}
		current, readErr := readStartup(key)
		if readErr != nil {
			return errors.Join(err, errors.New("cannot verify startup entry for rollback"), readErr)
		}
		if current != after {
			return errors.Join(err, errors.New("startup entry changed during settings save; leaving it in place"))
		}
		return errors.Join(err, writeStartup(key, before))
	}
	return nil
}
