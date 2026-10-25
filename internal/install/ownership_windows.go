package install

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var PackageVersion = "0.1.0-alpha.3"
var PackageSource = "development"

const ledgerLimit = 64 * 1024
const reportLimit = 16 * 1024
const fileLimit = 64 * 1024 * 1024

type managedFile struct{ id, path string }
type managedValue struct{ id, key, name, wanted string }

func packageFiles(l winuser.Layout) []managedFile {
	return []managedFile{
		{"exe", l.Executable}, {"uninstaller", l.Uninstaller}, {"license", filepath.Join(l.InstallDir, "LICENSE")}, {"notices", filepath.Join(l.InstallDir, "THIRD_PARTY_NOTICES.txt")}, {"release", filepath.Join(l.InstallDir, "release.json")}, {"launch", l.LaunchShortcut}, {"settings", l.SettingsShortcut},
	}
}
func packageValues(l winuser.Layout) []managedValue {
	return []managedValue{
		{"display-name", uninstallRegistry, "DisplayName", "Steam Sage Companion"},
		{"display-version", uninstallRegistry, "DisplayVersion", PackageVersion},
		{"publisher", uninstallRegistry, "Publisher", "Effigy Analytics"},
		{"uninstall-command", uninstallRegistry, "UninstallString", `"` + l.Uninstaller + `"`},
		{"install-location", uninstallRegistry, "InstallLocation", l.InstallDir},
		{"run", runRegistry, "SteamSageCompanion", syscall.EscapeArg(l.Executable) + " serve --background --config-dir " + syscall.EscapeArg(l.ConfigDir)},
	}
}

// Handles reject reparse leaves and deny writers while reading the entire file.
func readManaged(path string, limit int64) ([]byte, error) {
	if _, err := winuser.CanonicalDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(h), path)
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errors.New("managed path is a reparse point or wrong type")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size > uint64(limit) {
		return nil, errors.New("managed file exceeds size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("managed file exceeds size limit")
	}
	return raw, nil
}
func managedHash(path string) (string, error) {
	raw, err := readManaged(path, fileLimit)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func readValue(v managedValue) (string, bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, v.key, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer key.Close()
	value, kind, err := key.GetStringValue(v.name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	if kind != registry.SZ {
		return "", true, errors.New("registry entry has unexpected type")
	}
	if len(value) > 8192 {
		return "", true, errors.New("registry value exceeds limit")
	}
	return value, true, nil
}
func hexString(s string) string { return hex.EncodeToString([]byte(s)) }
func ledgerMetadata(l winuser.Layout) (map[string]string, error) {
	sid, err := winuser.CurrentSID()
	return map[string]string{"schema": "1", "kind": "steam-sage-companion", "owner": sid, "install": l.InstallDir, "menu": l.StartMenuDir, "config": l.ConfigDir, "version": PackageVersion, "source": PackageSource}, err
}
func loadLedger(l winuser.Layout) (packageINI, error) {
	raw, err := readManaged(l.Ledger, ledgerLimit)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record, err := decodeINI(raw, ledgerLimit)
	if err != nil {
		return nil, err
	}
	wanted, err := ledgerMetadata(l)
	if err != nil {
		return nil, err
	}
	meta := record["package"]
	if len(meta) != len(wanted) {
		return nil, errors.New("invalid package identity fields")
	}
	for _, key := range []string{"schema", "kind", "owner", "install", "menu", "config"} {
		if meta[key] != wanted[key] {
			return nil, errors.New("package ownership or fixed roots differ")
		}
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-alpha\.[0-9]+)?$`).MatchString(meta["version"]) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(meta["source"]) {
		return nil, errors.New("invalid package source identity")
	}
	ids := map[string]bool{"package": true}
	for _, f := range packageFiles(l) {
		ids[f.id] = true
		entry := record[f.id]
		if len(entry) != 1 || len(entry["sha256"]) != 64 {
			return nil, errors.New("invalid managed file hash")
		}
		if _, err := hex.DecodeString(entry["sha256"]); err != nil {
			return nil, err
		}
	}
	for _, v := range packageValues(l) {
		ids[v.id] = true
		entry := record[v.id]
		if v.id == "run" && entry == nil {
			continue
		}
		if len(entry) != 1 {
			return nil, errors.New("invalid registry ownership entry")
		}
		raw, err := hex.DecodeString(entry["hex"])
		if err != nil || len(raw) > 8192 {
			return nil, errors.New("invalid registry ownership value")
		}
	}
	for id := range record {
		if !ids[id] {
			return nil, errors.New("unknown package managed ID")
		}
	}
	return record, nil
}
func inside(path, root string) bool {
	return strings.EqualFold(path, root) || strings.HasPrefix(strings.ToLower(path), strings.ToLower(root)+string(filepath.Separator))
}
func stagePaths(stage, report string, l winuser.Layout) (string, error) {
	if !filepath.IsAbs(stage) || !filepath.IsAbs(report) {
		return "", errors.New("package staging paths must be absolute")
	}
	canonical, err := winuser.CanonicalDir(stage)
	if err != nil {
		return "", err
	}
	reportParent, err := winuser.CanonicalDir(filepath.Dir(report))
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(reportParent, canonical) || !strings.EqualFold(filepath.Base(report), "report.ini") {
		return "", errors.New("report must be report.ini inside the private stage")
	}
	for _, root := range []string{l.InstallDir, l.StartMenuDir, l.ConfigDir} {
		if inside(canonical, root) || inside(root, canonical) {
			return "", errors.New("report staging must not overlap product destinations")
		}
	}
	if err := winuser.EnsurePrivateDir(canonical); err != nil {
		return "", err
	}
	return canonical, nil
}
func writePrivate(stage, name string, raw []byte) error {
	dst := filepath.Join(stage, name)
	if err := winuser.ValidatePrivateFile(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(stage, ".report-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = winuser.ProtectNewFile(f); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	from, _ := windows.UTF16PtrFromString(tmp)
	to, _ := windows.UTF16PtrFromString(dst)
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func unknownItems(l winuser.Layout) bool {
	allowed := map[string]bool{strings.ToLower(l.Ledger): true}
	for _, f := range packageFiles(l) {
		allowed[strings.ToLower(f.path)] = true
	}
	for _, dir := range []string{l.InstallDir, l.StartMenuDir} {
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return true
		}
		for _, entry := range entries {
			if !allowed[strings.ToLower(filepath.Join(dir, entry.Name()))] {
				return true
			}
		}
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallRegistry, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	defer key.Close()
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return true
	}
	known := map[string]bool{}
	for _, v := range packageValues(l) {
		if v.key == uninstallRegistry {
			known[strings.ToLower(v.name)] = true
		}
	}
	for _, name := range names {
		if !known[strings.ToLower(name)] {
			return true
		}
	}
	children, err := key.ReadSubKeyNames(-1)
	return err != nil || len(children) > 0
}

// InspectPackage reads fixed product destinations and writes only private staging
// reports/candidate ledger. NSIS alone commits or removes product files/registry.
func InspectPackage(operation, stage, report string) error {
	if operation != "preflight-install" && operation != "preflight-uninstall" && operation != "snapshot" {
		return errors.New("unknown package inspection operation")
	}
	l, err := packageLayout("")
	if err != nil {
		return err
	}
	for _, dir := range []string{l.InstallDir, l.StartMenuDir, l.ConfigDir} {
		if _, err := winuser.CanonicalDir(dir); err != nil {
			return err
		}
	}
	stage, err = stagePaths(stage, report, l)
	if err != nil {
		return err
	}
	owner, err := winuser.CurrentSID()
	if err != nil {
		return err
	}
	// Independent roots are report cross-checks only, never mutation authority.
	outcome := packageINI{"package": {"schema": "1", "result": "ok", "leftovers": "no", "install": l.InstallDir, "menu": l.StartMenuDir, "config": l.ConfigDir, "owner": owner}}
	record, inspectErr := loadLedger(l)
	candidate := packageINI{}
	if operation == "snapshot" {
		metadata, e := ledgerMetadata(l)
		if e != nil {
			return e
		}
		candidate["package"] = metadata
	}
	leftovers := unknownItems(l)
	for _, f := range packageFiles(l) {
		hash, e := managedHash(f.path)
		status := "missing"
		if e == nil {
			status = "unowned"
			if record != nil {
				status = "changed"
				if record[f.id]["sha256"] == hash {
					status = "owned"
				}
			}
		} else if !os.IsNotExist(e) {
			status = "unowned"
			inspectErr = errors.Join(inspectErr, fmt.Errorf("cannot inspect %s: %w", f.id, e))
		}
		if operation == "snapshot" {
			if e != nil {
				inspectErr = errors.Join(inspectErr, fmt.Errorf("missing snapshot file %s", f.id))
			} else {
				candidate[f.id] = map[string]string{"sha256": hash}
				status = "owned"
			}
		}
		outcome[f.id] = map[string]string{"status": status}
		if status == "changed" || status == "unowned" {
			leftovers = true
			if operation == "preflight-install" {
				inspectErr = errors.Join(inspectErr, fmt.Errorf("preserving %s: %s", f.id, status))
			}
		}
	}
	for _, v := range packageValues(l) {
		value, present, e := readValue(v)
		status := "missing"
		if e != nil {
			status = "unowned"
			inspectErr = errors.Join(inspectErr, e)
		} else if present {
			status = "unowned"
			if record != nil {
				status = "changed"
				expected := record[v.id]["hex"]
				if v.id == "run" {
					expected = hexString(v.wanted)
				}
				if expected == hexString(value) {
					status = "owned"
				}
			}
		}
		if operation == "snapshot" {
			if !present && v.id == "run" {
			} else if e != nil || !present || value != v.wanted {
				inspectErr = errors.Join(inspectErr, fmt.Errorf("unexpected snapshot registry %s", v.id))
			} else {
				candidate[v.id] = map[string]string{"hex": hexString(value)}
				status = "owned"
			}
		}
		outcome[v.id] = map[string]string{"status": status}
		if status == "changed" || status == "unowned" {
			leftovers = true
			if operation == "preflight-install" {
				inspectErr = errors.Join(inspectErr, fmt.Errorf("preserving registry %s", v.id))
			}
		}
	}
	if leftovers {
		outcome["package"]["leftovers"] = "yes"
	}
	if inspectErr != nil {
		outcome["package"]["result"] = "preserve"
	}
	raw, err := encodeINI(outcome, reportLimit)
	if err != nil {
		return err
	}
	if err = writePrivate(stage, "report.ini", raw); err != nil {
		return err
	}
	if inspectErr != nil {
		return inspectErr
	}
	if operation == "snapshot" {
		raw, err := encodeINI(candidate, ledgerLimit)
		if err != nil {
			return err
		}
		return writePrivate(stage, "candidate.ini", raw)
	}
	return nil
}
