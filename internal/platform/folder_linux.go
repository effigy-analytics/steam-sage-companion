package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func ChooseSteamRoot(ctx context.Context) (string, error) {
	path, err := exec.LookPath("zenity")
	if err != nil {
		return "", errors.New("folder picker unavailable; enter the Steam installation folder in local controls")
	}
	out, err := exec.CommandContext(ctx, path, "--file-selection", "--directory", "--title=Find Steam installation").Output()
	if err != nil {
		return "", errors.New("no folder selected")
	}
	return strings.TrimSpace(string(out)), nil
}
