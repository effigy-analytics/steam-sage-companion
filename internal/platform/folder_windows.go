package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func ChooseSteamRoot(ctx context.Context) (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Find Steam installation'; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; [Console]::Write($d.SelectedPath)}`
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script).Output()
	if err != nil || len(out) == 0 {
		return "", errors.New("no folder selected")
	}
	return strings.TrimSpace(string(out)), nil
}
func OpenBrowser(url string) error {
	command := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}
