package config

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertPrivateWindows(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || !windows.EqualSid(owner, user.User.Sid) {
		t.Fatalf("wrong owner: %v", err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL inherits broad permissions: %v", err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		t.Fatalf("expected exactly one owner ACE: %v", err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid(sid, user.User.Sid) || ace.Mask&0x001f01ff != 0x001f01ff {
		t.Fatal("settings grants access beyond the owner or denies full owner access")
	}
}

func TestWindowsSettingsPrivateAfterCreateUpdateAndReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "用户 settings")
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Startup {
		t.Fatal("new settings enabled startup")
	}
	assertPrivateWindows(t, dir)
	assertPrivateWindows(t, filepath.Join(dir, "settings.json"))
	c.SelectedAccount = "synthetic"
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	assertPrivateWindows(t, filepath.Join(dir, "settings.json"))
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(c *Config) error { c.Startup = true; return nil }); err != nil {
		t.Fatal(err)
	}
	assertPrivateWindows(t, dir)
	assertPrivateWindows(t, filepath.Join(dir, "settings.json"))
	got, err := Load(dir)
	if err != nil || got.SelectedAccount != "synthetic" || !got.Startup {
		t.Fatalf("lost settings during replacement: %+v %v", got, err)
	}
}

func TestWindowsSettingsRejectBroadExistingPermissions(t *testing.T) {
	for _, leaf := range []string{"directory", "file"} {
		t.Run(leaf, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "settings")
			if _, err := Load(dir); err != nil {
				t.Fatal(err)
			}
			path := dir
			if leaf == "file" {
				path = filepath.Join(dir, "settings.json")
			}
			sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
			if err != nil {
				t.Fatal(err)
			}
			acl, _, err := sd.DACL()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("read existing unowned/broad settings")
			}
			if err := Save(dir, Config{InstanceID: "replacement"}); err == nil {
				t.Fatal("rewrote existing unowned/broad settings")
			}
		})
	}
}

func TestWindowsFailedReplacementRetainsLastGoodSettings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "settings")
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Allow reads, but hold off replacement and deletion using a native sharing lock.
	handle, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.SelectedAccount = "must-not-commit"
	saveErr := Save(dir, c)
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	if saveErr == nil {
		t.Fatal("replacement ignored the sharing lock")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("last good settings changed: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(dir, ".settings-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary credentials leaked: %v %v", entries, err)
	}
}

func TestWindowsSettingsRejectReparseAncestors(t *testing.T) {
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
	dir := filepath.Join(link, "settings")
	if _, err := Load(dir); err == nil {
		t.Fatal("created settings through reparse ancestor")
	}
	if err := Save(dir, Config{InstanceID: "synthetic"}); err == nil {
		t.Fatal("saved settings through reparse ancestor")
	}
	if _, err := os.Stat(filepath.Join(target, "settings")); !os.IsNotExist(err) {
		t.Fatal("reparse target was modified")
	}
}
