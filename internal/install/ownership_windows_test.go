package install

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func fixtureLayout(t *testing.T) winuser.Layout {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "安装 雪", "Steam Sage Companion")
	menu := filepath.Join(root, "开始 菜单")
	l := winuser.Layout{InstallDir: dir, Executable: filepath.Join(dir, "steam-sage-companion.exe"), Uninstaller: filepath.Join(dir, "Uninstall.exe"), Ledger: filepath.Join(dir, "package.ini"), StartMenuDir: menu, LaunchShortcut: filepath.Join(menu, "Steam Sage Companion.lnk"), SettingsShortcut: filepath.Join(menu, "Companion Settings.lnk"), ConfigDir: filepath.Join(root, "private settings")}
	oldLayout, oldUninstall, oldRun, oldSource := packageLayout, uninstallRegistry, runRegistry, PackageSource
	packageLayout = func(string) (winuser.Layout, error) { return l, nil }
	key := `Software\SteamSageCompanionInspectorTests\` + filepath.Base(root)
	uninstallRegistry = key + `\Uninstall`
	runRegistry = key + `\Run`
	PackageSource = strings.Repeat("a", 40)
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, uninstallRegistry)
		registry.DeleteKey(registry.CURRENT_USER, runRegistry)
		registry.DeleteKey(registry.CURRENT_USER, key)
		packageLayout, uninstallRegistry, runRegistry, PackageSource = oldLayout, oldUninstall, oldRun, oldSource
	})
	return l
}
func TestWindowsInspectorMissingPackageWritesOnlyPrivateUnicodeReport(t *testing.T) {
	l := fixtureLayout(t)
	stage := filepath.Join(t.TempDir(), "报告 雪")
	report := filepath.Join(stage, "report.ini")
	if err := InspectPackage("preflight-install", stage, report); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 16384 || !bytes.HasPrefix(raw, []byte{255, 254}) {
		t.Fatal("report encoding/bounds")
	}
	if err := winuser.ValidatePrivateDir(stage); err != nil {
		t.Fatal(err)
	}
	if err := winuser.ValidatePrivateFile(report); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{l.InstallDir, l.StartMenuDir, l.ConfigDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("inspector created destination", path, err)
		}
	}
	outcome, err := decodeINI(raw, reportLimit)
	if err != nil || outcome["exe"]["status"] != "missing" {
		t.Fatal("wrong missing report", err)
	}
}
func TestWindowsInspectorRefusesUnownedIdenticalAndOversizeFile(t *testing.T) {
	l := fixtureLayout(t)
	os.MkdirAll(l.InstallDir, 0700)
	os.WriteFile(l.Executable, []byte("payload"), 0600)
	stage := filepath.Join(t.TempDir(), "report")
	if err := InspectPackage("preflight-install", stage, filepath.Join(stage, "report.ini")); err == nil {
		t.Fatal("accepted existing unowned bytes")
	}
	f, err := os.OpenFile(l.Executable, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(fileLimit + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := managedHash(l.Executable); err == nil {
		t.Fatal("hashed a prefix of an oversized file")
	}
}
func TestWindowsInspectorSnapshotRoundTripsFixedRootsHashesAndOpaqueRegistry(t *testing.T) {
	l := fixtureLayout(t)
	os.MkdirAll(l.InstallDir, 0700)
	os.MkdirAll(l.StartMenuDir, 0700)
	for _, f := range packageFiles(l) {
		if err := os.WriteFile(f.path, []byte("fixture "+f.id), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range packageValues(l) {
		if v.id == "run" {
			continue
		}
		key, _, err := registry.CreateKey(registry.CURRENT_USER, v.key, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		err = key.SetStringValue(v.name, v.wanted)
		key.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	stage := filepath.Join(t.TempDir(), "snapshot")
	if err := InspectPackage("snapshot", stage, filepath.Join(stage, "report.ini")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(stage, "candidate.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(l.Ledger, raw, 0600); err != nil {
		t.Fatal(err)
	}
	record, err := loadLedger(l)
	if err != nil {
		t.Fatal(err)
	}
	if record["package"]["install"] != l.InstallDir {
		t.Fatal("Unicode root changed")
	}
	// Hand-computed SHA-256 of the fixture bytes, independent of managedHash.
	if record["exe"]["sha256"] != "9efd11ad0cc0a37107c357cc748e2b402cbbeabc5651c5460133d7fe7a0832ab" {
		t.Fatal("complete fixture SHA differs:", record["exe"])
	}
	if got := StartupExecutable("portable.exe", Paths{ConfigDir: l.ConfigDir}); got != l.Executable {
		t.Fatal("did not select verified installed image", got)
	}
	os.WriteFile(l.Executable, []byte("changed"), 0600)
	if got := StartupExecutable("portable.exe", Paths{ConfigDir: l.ConfigDir}); got != "portable.exe" {
		t.Fatal("selected altered installed image", got)
	}
}
func TestWindowsLedgerCodecRejectsAmbiguousOrUnboundedInput(t *testing.T) {
	encode := func(s string) []byte {
		words := utf16.Encode([]rune(s))
		raw := make([]byte, 2+2*len(words))
		raw[0] = 255
		raw[1] = 254
		for i, v := range words {
			binary.LittleEndian.PutUint16(raw[2+i*2:], v)
		}
		return raw
	}
	for _, input := range []string{"[package]\na=1\na=2\n", "[package]\na=1\n[package]\nb=2\n", "[package]\na=\"quoted\"\n", "[package]\na=x\x00y\n", "[package]\na=x\x1fy\n"} {
		if _, err := decodeINI(encode(input), ledgerLimit); err == nil {
			t.Fatalf("accepted ambiguous record %q", input)
		}
	}
	for _, raw := range [][]byte{[]byte("[package]\na=1\n"), {255, 254, 0, 216}, make([]byte, ledgerLimit+2)} {
		if _, err := decodeINI(raw, ledgerLimit); err == nil {
			t.Fatal("accepted invalid Unicode/bounds")
		}
	}
}
func TestWindowsInspectorRefusesProductStageAndExternalReport(t *testing.T) {
	l := fixtureLayout(t)
	if err := InspectPackage("preflight-install", l.InstallDir, filepath.Join(l.InstallDir, "report.ini")); err == nil {
		t.Fatal("wrote into product destination")
	}
	stage := filepath.Join(t.TempDir(), "stage")
	external := filepath.Join(t.TempDir(), "report.ini")
	if err := InspectPackage("preflight-install", stage, external); err == nil {
		t.Fatal("accepted report outside stage")
	}
	if _, err := os.Stat(external); !os.IsNotExist(err) {
		t.Fatal("external report created")
	}
}

// REG_SZ is raw bytes. GetStringValue silently truncates embedded NULs and
// replaces malformed UTF-16, so it cannot establish exact package ownership.
func TestWindowsRegistryInspectionRejectsHiddenSuffixAndMalformedUTF16(t *testing.T) {
	fixtureLayout(t)
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRegistry, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	name, _ := windows.UTF16PtrFromString("DisplayName")
	set := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")
	cases := map[string][]byte{
		"embedded NUL suffix": {'o', 0, 'w', 0, 'n', 0, 'e', 0, 'd', 0, 0, 0, 'x', 0, 0, 0},
		"unpaired surrogate":  {0, 216, 0, 0},
		"odd byte count":      {'o', 0, 0},
		"missing terminator":  {'o', 0},
	}
	oversize := make([]byte, 20000)
	copy(oversize, []byte{'o', 0, 'w', 0, 'n', 0, 'e', 0, 'd', 0, 0, 0})
	cases["oversize hidden suffix"] = oversize
	for label, raw := range cases {
		t.Run(label, func(t *testing.T) {
			code, _, _ := set.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(registry.SZ), uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)))
			if code != 0 {
				t.Fatalf("write synthetic raw REG_SZ: %d", code)
			}
			storedSize, storedKind, sizeErr := key.GetValue("DisplayName", nil)
			probe := bytes.Repeat([]byte{0xcc}, len(raw)+4)
			returnedSize, returnedKind, probeErr := key.GetValue("DisplayName", probe)
			tail := probe
			if len(tail) > 16 {
				tail = tail[len(tail)-16:]
			}
			t.Logf("raw registry diagnostic: supplied=%d queried=%d kind=%d sizeErr=%v returned=%d kind=%d readErr=%v tail=%x", len(raw), storedSize, storedKind, sizeErr, returnedSize, returnedKind, probeErr, tail)
			if probeErr != nil || returnedSize < 0 || returnedSize > len(probe) {
				t.Fatalf("cannot inspect actual fixture bytes: %v", probeErr)
			}
			if label == "missing terminator" && bytes.Equal(probe[:returnedSize], append(append([]byte{}, raw...), 0, 0)) {
				// This Windows version stores a canonical NUL-terminated REG_SZ.
				// Ownership follows stored bytes, not the malformed writer intent.
				value, present, err := readValue(managedValue{key: uninstallRegistry, name: "DisplayName", wanted: "owned"})
				if err != nil || !present || value != "o" {
					t.Fatalf("OS-normalized value did not decode exactly: %q %v", value, err)
				}
				t.Log("Win32 normalized the writer input; canonical stored string decoded")
				return
			}
			_, present, err := readValue(managedValue{key: uninstallRegistry, name: "DisplayName", wanted: "owned"})
			if err == nil || !present {
				t.Fatalf("malformed or changed raw value became an owned string: present=%v err=%v", present, err)
			}
		})
	}
}

func TestWindowsInspectorEnumerationIsFiniteAndConservative(t *testing.T) {
	for _, scenario := range []string{"empty", "install-extra", "menu-extra", "values-extra", "subkey", "directory-error", "values-error", "subkey-error"} {
		t.Run(scenario, func(t *testing.T) {
			l := fixtureLayout(t)
			for _, dir := range []string{l.InstallDir, l.StartMenuDir} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRegistry, registry.ALL_ACCESS)
			if err != nil {
				t.Fatal(err)
			}
			defer key.Close()
			switch scenario {
			case "install-extra", "menu-extra":
				dir := l.InstallDir
				if scenario == "menu-extra" {
					dir = l.StartMenuDir
				}
				for i := 0; i < 10; i++ {
					if err := os.WriteFile(filepath.Join(dir, string(rune('a'+i))), []byte("keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "values-extra":
				for i := 0; i < 6; i++ {
					if err := key.SetStringValue(string(rune('a'+i)), "keep"); err != nil {
						t.Fatal(err)
					}
				}
			case "subkey":
				child, _, err := registry.CreateKey(key, "keep", registry.ALL_ACCESS)
				if err != nil {
					t.Fatal(err)
				}
				child.Close()
				defer registry.DeleteKey(key, "keep")
			}
			oldDir, oldValues, oldChildren := readPackageDirectory, readPackageValueNames, readPackageSubkeyNames
			defer func() {
				readPackageDirectory, readPackageValueNames, readPackageSubkeyNames = oldDir, oldValues, oldChildren
			}()
			dirs, values, children := 0, 0, 0
			readPackageDirectory = func(path string, count int) ([]os.DirEntry, error) {
				dirs++
				limit := 7 // Five fixed install files plus ledger, then one overflow entry.
				if path == l.StartMenuDir {
					limit = 3
				}
				if count <= 0 || count > limit {
					t.Errorf("directory enumeration count=%d, require 1..%d", count, limit)
				}
				if scenario == "directory-error" {
					return nil, os.ErrPermission
				}
				return oldDir(path, count)
			}
			readPackageValueNames = func(k registry.Key, count int) ([]string, error) {
				values++
				if count <= 0 || count > 6 {
					t.Errorf("value enumeration count=%d, require 1..6", count)
				}
				if scenario == "values-error" {
					return nil, os.ErrPermission
				}
				return oldValues(k, count)
			}
			readPackageSubkeyNames = func(k registry.Key, count int) ([]string, error) {
				children++
				if count != 1 {
					t.Errorf("subkey enumeration count=%d, require exactly 1", count)
				}
				if scenario == "subkey-error" {
					return nil, os.ErrPermission
				}
				return oldChildren(k, count)
			}
			stage := filepath.Join(t.TempDir(), "inspection")
			report := filepath.Join(stage, "report.ini")
			if err := InspectPackage("preflight-install", stage, report); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			ini, err := decodeINI(raw, reportLimit)
			if err != nil {
				t.Fatal(err)
			}
			want := "yes"
			if scenario == "empty" {
				want = "no"
			}
			if got := ini["package"]["leftovers"]; got != want {
				t.Errorf("leftovers=%q want %q", got, want)
			}
			if dirs == 0 {
				t.Fatal("directory reader was not exercised")
			}
			if scenario == "empty" && (values == 0 || children == 0) {
				t.Fatal("empty fixture did not exercise every enumeration boundary")
			}
		})
	}
}
