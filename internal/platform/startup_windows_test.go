package platform

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func fixtureStartup(t *testing.T) registry.Key {
	t.Helper()
	old := startupRegistryPath
	startupRegistryPath = fmt.Sprintf(`Software\SteamSageTests\%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRegistryPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	path := startupRegistryPath
	t.Cleanup(func() { key.Close(); registry.DeleteKey(registry.CURRENT_USER, path); startupRegistryPath = old })
	return key
}

const fixtureEXE = `C:\Users\用户 name\Steam Sage\steam-sage-companion.exe`
const fixtureConfig = `C:\Users\用户 name\AppData\Roaming\steam-sage-companion`
const fixtureCommand = `"C:\Users\用户 name\Steam Sage\steam-sage-companion.exe" serve --background --config-dir "C:\Users\用户 name\AppData\Roaming\steam-sage-companion"`

func startupValue(t *testing.T, key registry.Key) string {
	t.Helper()
	s, _, err := key.GetStringValue("SteamSageCompanion")
	if err == registry.ErrNotExist {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestWindowsStartupOwnEnableAndRemove(t *testing.T) {
	key := fixtureStartup(t)
	if err := SetStartup(false, fixtureEXE, fixtureConfig); err != nil {
		t.Fatal(err)
	}
	if got := startupValue(t, key); got != "" {
		t.Fatal("default enabled startup")
	}
	if err := SetStartup(true, fixtureEXE, fixtureConfig); err != nil {
		t.Fatal(err)
	}
	got := startupValue(t, key)
	if got != fixtureCommand {
		t.Fatalf("command=%q", got)
	}
	argv, err := windows.DecomposeCommandLine(got)
	if err != nil || len(argv) != 5 || argv[0] != fixtureEXE || argv[4] != fixtureConfig {
		t.Fatalf("invalid native argument quoting: %q %v", argv, err)
	}
	if err := SetStartup(false, fixtureEXE, fixtureConfig); err != nil {
		t.Fatal(err)
	}
	if got := startupValue(t, key); got != "" {
		t.Fatal("owned startup retained")
	}
}
func TestWindowsStartupPreservesForeignValues(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			key := fixtureStartup(t)
			foreign := `C:\Other\foreign.exe --flag`
			if err := key.SetStringValue("SteamSageCompanion", foreign); err != nil {
				t.Fatal(err)
			}
			if err := SetStartup(enabled, fixtureEXE, fixtureConfig); err == nil {
				t.Fatal("accepted foreign startup registration")
			}
			if got := startupValue(t, key); got != foreign {
				t.Fatalf("foreign registration changed: %q", got)
			}
		})
	}
}
func TestWindowsStartupTransactionRollback(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			key := fixtureStartup(t)
			before := ""
			if !enabled {
				before = fixtureCommand
				if err := key.SetStringValue("SteamSageCompanion", before); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := errors.New("settings persistence failed")
			called := false
			err := SetStartupTransaction(enabled, fixtureEXE, fixtureConfig, func() error {
				called = true
				want := ""
				if enabled {
					want = fixtureCommand
				}
				if got := startupValue(t, key); got != want {
					t.Fatalf("persist sees wrong startup state: %q", got)
				}
				return sentinel
			})
			if !called || !errors.Is(err, sentinel) {
				t.Fatalf("persist error lost: called=%v err=%v", called, err)
			}
			if got := startupValue(t, key); got != before {
				t.Fatalf("rollback lost prior registration: %q", got)
			}
		})
	}
}
func TestWindowsStartupRollbackPreservesConcurrentChange(t *testing.T) {
	key := fixtureStartup(t)
	foreign := `C:\Changed\other.exe`
	sentinel := errors.New("save failed")
	err := SetStartupTransaction(true, fixtureEXE, fixtureConfig, func() error {
		if err := key.SetStringValue("SteamSageCompanion", foreign); err != nil {
			t.Fatal(err)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("persist error lost: %v", err)
	}
	if got := startupValue(t, key); got != foreign {
		t.Fatalf("rollback erased concurrent change: %q", got)
	}
}

func TestWindowsStartupPreservesLossyOrOversizeRawRegistryValues(t *testing.T) {
	words := utf16.Encode([]rune(fixtureCommand))
	rawWords := func(words []uint16) []byte {
		raw := make([]byte, len(words)*2)
		for i, u := range words {
			binary.LittleEndian.PutUint16(raw[2*i:], u)
		}
		return raw
	}
	suffix := append(append([]uint16{}, words...), 0, 'x', 0)
	malformed := append(append([]uint16{}, words...), 0, 0xd800, 0)
	oversized := append(append([]uint16{}, words...), make([]uint16, 9000)...)
	cases := map[string][]byte{"embedded-NUL suffix": rawWords(suffix), "invalid UTF16": rawWords(malformed), "missing terminator": rawWords(words), "oversize": rawWords(oversized)}
	for label, raw := range cases {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", label, enabled), func(t *testing.T) {
				key := fixtureStartup(t)
				name, _ := windows.UTF16PtrFromString("SteamSageCompanion")
				set := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")
				code, _, _ := set.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(registry.SZ), uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)))
				if code != 0 {
					t.Fatalf("write synthetic REG_SZ: %d", code)
				}
				storedSize, storedKind, sizeErr := key.GetValue("SteamSageCompanion", nil)
				probe := bytes.Repeat([]byte{0xcc}, len(raw)+4)
				returnedSize, returnedKind, probeErr := key.GetValue("SteamSageCompanion", probe)
				tail := probe
				if len(tail) > 16 {
					tail = tail[len(tail)-16:]
				}
				t.Logf("raw registry diagnostic: supplied=%d queried=%d kind=%d sizeErr=%v returned=%d kind=%d readErr=%v tail=%x", len(raw), storedSize, storedKind, sizeErr, returnedSize, returnedKind, probeErr, tail)
				if probeErr != nil || returnedSize < 0 || returnedSize > len(probe) {
					t.Fatalf("cannot inspect actual fixture bytes: %v", probeErr)
				}
				if label == "missing terminator" && bytes.Equal(probe[:returnedSize], append(append([]byte{}, raw...), 0, 0)) {
					// The OS stored a valid command; don't mistake writer intent
					// for an actually changed or malformed registration.
					registration, err := readStartup(key)
					if err != nil || !registration.present || registration.command != fixtureCommand {
						t.Fatalf("OS-normalized command did not decode exactly: %#v %v", registration, err)
					}
					t.Log("Win32 normalized the writer input; canonical stored command decoded")
					return
				}
				called := false
				err := SetStartupTransaction(enabled, fixtureEXE, fixtureConfig, func() error { called = true; return nil })
				if err == nil || called {
					t.Fatalf("malformed registration accepted: err=%v persistCalled=%v", err, called)
				}
				after := make([]byte, len(raw))
				n, kind, err := key.GetValue("SteamSageCompanion", after)
				if err != nil || kind != registry.SZ || n != len(raw) || !bytes.Equal(raw, after) {
					t.Fatalf("changed registration was not preserved byte-for-byte: %v", err)
				}
			})
		}
	}
}
