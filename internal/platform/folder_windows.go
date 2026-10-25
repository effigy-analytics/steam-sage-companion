package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func ChooseSteamRoot(ctx context.Context) (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Find Steam installation'; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [Console]::Write($d.SelectedPath)}`
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := command.Output()
	if err != nil || len(out) == 0 {
		return "", errors.New("no folder selected")
	}
	return strings.TrimSpace(string(out)), nil
}
