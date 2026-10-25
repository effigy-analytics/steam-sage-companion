package winuser

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
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
	install := filepath.Join(local, "Programs", "Steam Sage Companion")
	if layout.InstallDir != install || layout.Executable != filepath.Join(install, "steam-sage-companion.exe") || layout.Uninstaller != filepath.Join(install, "uninstall.exe") || layout.Ledger != filepath.Join(install, "package.ini") || layout.StartMenuDir != filepath.Join(programs, "Steam Sage Companion") || layout.LaunchShortcut != filepath.Join(programs, "Steam Sage Companion", "Launch.lnk") || layout.SettingsShortcut != filepath.Join(programs, "Steam Sage Companion", "Companion Settings.lnk") || layout.ConfigDir != filepath.Join(roaming, "steam-sage-companion") {
		t.Fatalf("incorrect independently derived layout: %+v", layout)
	}
	custom := filepath.Join(root, "自定义 settings")
	layout, err = LayoutFor(custom)
	if err != nil || layout.ConfigDir != custom || layout.InstallDir != install {
		t.Fatalf("custom config changed product roots: %+v %v", layout, err)
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
