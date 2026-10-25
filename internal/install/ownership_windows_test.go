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
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRegistry, registry.SET_VALUE)
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
	for label, raw := range cases {
		t.Run(label, func(t *testing.T) {
			code, _, _ := set.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(registry.SZ), uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)))
			if code != 0 {
				t.Fatalf("write synthetic raw REG_SZ: %d", code)
			}
			_, present, err := readValue(managedValue{key: uninstallRegistry, name: "DisplayName", wanted: "owned"})
			if err == nil || !present {
				t.Fatalf("malformed or changed raw value became an owned string: present=%v err=%v", present, err)
			}
		})
	}
}
