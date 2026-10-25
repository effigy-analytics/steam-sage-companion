package winuser

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsCurrentSIDMatchesProcessToken(t *testing.T) {
	got, err := CurrentSID()
	if err != nil {
		t.Fatal(err)
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if got != u.User.Sid.String() {
		t.Fatalf("wrong identity: %q", got)
	}
}
func TestWindowsRejectOtherSIDAndBroadDescriptors(t *testing.T) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := u.User.Sid.String()
	for _, tc := range []struct {
		name, sddl string
		valid      bool
	}{{"private", "O:" + sid + "D:P(A;;FA;;;" + sid + ")", true}, {"other owner", "O:SYD:P(A;;FA;;;" + sid + ")", false}, {"broad ACE", "O:" + sid + "D:P(A;;FA;;;WD)", false}, {"inherited", "O:" + sid + "D:(A;;FA;;;" + sid + ")", false}, {"null DACL", "O:" + sid, false}} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDescriptor(sd, sid); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
func TestWindowsCanonicalPathsAndLayoutUseKnownFolders(t *testing.T) {
	root := t.TempDir()
	// Windows CI's temp root may contain RUNNER~1. Resolve the fixture's existing
	// root independently, while passing its original alias into the real layout.
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{root, strings.ToLower(root)} {
		got, err := CanonicalDir(alias)
		if err != nil || got != physicalRoot {
			t.Fatalf("canonical fixture root: input %q got %q want %q; error %v", alias, got, physicalRoot, err)
		}
	}
	old := knownFolderPath
	t.Cleanup(func() { knownFolderPath = old })
	local := filepath.Join(root, "用户 Local")
	programs := filepath.Join(root, "用户 Programs")
	roaming := filepath.Join(root, "用户 Roaming")
	knownFolderPath = func(id *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		switch id {
		case windows.FOLDERID_LocalAppData:
			return local, nil
		case windows.FOLDERID_Programs:
			return programs, nil
		case windows.FOLDERID_RoamingAppData:
			return roaming, nil
		}
		t.Fatalf("unexpected known folder")
		return "", nil
	}
	layout, err := LayoutFor("")
	if err != nil {
		t.Fatal(err)
	}
	install := filepath.Join(physicalRoot, "用户 Local", "Programs", "Steam Sage Companion")

	for _, tc := range []struct{ name, got, want string }{
		{"InstallDir", layout.InstallDir, install},
		{"Executable", layout.Executable, filepath.Join(install, "steam-sage-companion.exe")},
		{"Uninstaller", layout.Uninstaller, filepath.Join(install, "Uninstall.exe")},
		{"Ledger", layout.Ledger, filepath.Join(install, "package.ini")},
		{"StartMenuDir", layout.StartMenuDir, filepath.Join(physicalRoot, "用户 Programs", "Steam Sage Companion")},
		{"LaunchShortcut", layout.LaunchShortcut, filepath.Join(physicalRoot, "用户 Programs", "Steam Sage Companion", "Steam Sage Companion.lnk")},
		{"SettingsShortcut", layout.SettingsShortcut, filepath.Join(physicalRoot, "用户 Programs", "Steam Sage Companion", "Companion Settings.lnk")},
		{"ConfigDir", layout.ConfigDir, filepath.Join(physicalRoot, "用户 Roaming", "steam-sage-companion")},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %q; want %q (fixture root %q)", tc.name, tc.got, tc.want, root)
		}
	}

	custom := filepath.Join(root, "自定义 settings")
	layout, err = LayoutFor(custom)
	wantConfig := filepath.Join(physicalRoot, "自定义 settings")
	if err != nil || layout.ConfigDir != wantConfig || layout.InstallDir != install {
		t.Fatalf("custom layout: ConfigDir got %q want %q; InstallDir got %q want %q; error %v", layout.ConfigDir, wantConfig, layout.InstallDir, install, err)
	}
}
func TestWindowsCanonicalDirRejectsReparseAncestor(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "junction")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create disposable junction: %v %s", err, out)
	}
	defer os.Remove(link)
	if _, err := CanonicalDir(filepath.Join(link, "settings")); err == nil {
		t.Fatal("accepted reparse ancestor")
	}
	if _, err := CanonicalDir(target); err != nil {
		t.Fatalf("rejected real directory: %v", err)
	}
}
