package launch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
	"golang.org/x/sys/windows"
)

// This fixture executes only from a disposable Windows CI worker. It never calls
// main/Steam discovery, uses an explicit owned config, and opens no real browser.
func init() {
	root := os.Getenv("STEAM_SAGE_NATIVE_FIXTURE")
	if root == "" || len(os.Args) < 2 || os.Args[1] != "serve" {
		return
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_OS") != "Windows" {
		os.Exit(90)
	}
	if b, e := os.ReadFile(filepath.Join(root, "owned-fixture")); e != nil || string(b) != "task2-synthetic-only" {
		os.Exit(91)
	}
	args := os.Args[2:]
	dir := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--config-dir" {
			dir = args[i+1]
		}
	}
	if dir != filepath.Join(root, "设置 private") {
		os.Exit(92)
	}
	modeBytes, _ := os.ReadFile(filepath.Join(root, "mode"))
	mode := string(modeBytes)
	if mode == "exited" {
		os.Exit(12)
	}
	pid := []byte(strconv.Itoa(os.Getpid()))
	_ = os.WriteFile(filepath.Join(root, "child-pid"), pid, 0600)
	if mode == "unready" {
		time.Sleep(20 * time.Second)
		os.Exit(13)
	}
	console, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	if console != 0 {
		_ = os.WriteFile(filepath.Join(root, "visible-console"), []byte("visible"), 0600)
	}
	raw, _ := json.Marshal(args)
	_ = os.WriteFile(filepath.Join(root, "child-args"), raw, 0600)
	port, err := net.Listen("tcp4", bridge.Address)
	if err != nil {
		os.Exit(14)
	}
	defer port.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	l, err := app.ListenActivation(ctx, dir, func() error {
		f, e := os.OpenFile(filepath.Join(root, "settings-opened"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = f.WriteString("opened\n")
		return e
	}, cancel)
	if err != nil {
		os.Exit(15)
	}
	defer l.Close()
	<-ctx.Done()
	os.Exit(0)
}

func nativeFixture(t *testing.T) (string, Options) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_OS") != "Windows" {
		t.Skip("owned process lifecycle requires disposable native Windows CI")
	}
	root := t.TempDir()
	t.Setenv("STEAM_SAGE_NATIVE_FIXTURE", root)
	if err := os.WriteFile(filepath.Join(root, "owned-fixture"), []byte("task2-synthetic-only"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "用户 installed helper.exe")
	b, e := os.ReadFile(os.Args[0])
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(executable, b, 0700); e != nil {
		t.Fatal(e)
	}
	options := Options{Executable: executable, ConfigDir: filepath.Join(root, "设置 private"), ExtraOrigin: "http://localhost:5173"}
	t.Cleanup(func() {
		// The only PID eligible for cleanup came from this fixture's private file.
		_ = app.RequestStopAndWait(options.ConfigDir, executable)
		if raw, e := os.ReadFile(filepath.Join(root, "child-pid")); e == nil {
			if pid, e := strconv.Atoi(string(raw)); e == nil {
				h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
				if e == nil {
					defer windows.CloseHandle(h)
					buf := make([]uint16, 32768)
					n := uint32(len(buf))
					if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil && strings.EqualFold(windows.UTF16ToString(buf[:n]), executable) {
						_ = windows.TerminateProcess(h, 19)
						_, _ = windows.WaitForSingleObject(h, 1000)
					}
				}
			}
		}
	})
	return root, options
}
func waitOwned(t *testing.T, options Options) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if app.RequestReadyFor(options.ConfigDir, options.Executable) == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("owned daemon disappeared after launcher returned")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func TestWindowsLaunchDestinationsReuseOwnedDetachedDaemon(t *testing.T) {
	for _, destination := range []Destination{Sage, Settings, None} {
		t.Run(fmt.Sprint(destination), func(t *testing.T) {
			root, options := nativeFixture(t)
			options.Destination = destination
			var urls []string
			browser := func(url string) error { urls = append(urls, url); return nil }
			if err := run(options, browser); err != nil {
				t.Fatal(err)
			}
			waitOwned(t, options)
			if _, e := os.Stat(filepath.Join(root, "visible-console")); !os.IsNotExist(e) {
				t.Fatal("background child inherited or created console")
			}
			var args []string
			raw, e := os.ReadFile(filepath.Join(root, "child-args"))
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(raw, &args); e != nil {
				t.Fatal(e)
			}
			if len(args) != 5 || args[0] != "--background" || args[1] != "--config-dir" || args[2] != options.ConfigDir || args[3] != "--allow-origin" || args[4] != "http://localhost:5173" {
				t.Fatalf("Unicode/space arguments changed: %q", args)
			}
			if destination == Sage {
				if len(urls) != 1 || urls[0] != "https://steam-sage.effigy-analytics.com/companion?installed=1" {
					t.Fatalf("wrong public destination: %q", urls)
				}
			} else if len(urls) != 0 {
				t.Fatal("settings/startup opened Sage")
			}
			pidBefore, e := os.ReadFile(filepath.Join(root, "child-pid"))
			if e != nil {
				t.Fatal(e)
			}
			if err := run(options, browser); err != nil {
				t.Fatal(err)
			}
			pidAfter, _ := os.ReadFile(filepath.Join(root, "child-pid"))
			if string(pidBefore) != string(pidAfter) {
				t.Fatal("warm launch replaced daemon")
			}
			if destination == Settings {
				raw, e := os.ReadFile(filepath.Join(root, "settings-opened"))
				if e != nil || string(raw) != "opened\nopened\n" {
					t.Fatal("cold/warm Settings must each open once")
				}
			} else if _, e := os.Stat(filepath.Join(root, "settings-opened")); !os.IsNotExist(e) {
				t.Fatal("readiness or startup opened controls")
			}
			if err := app.RequestStopAndWait(options.ConfigDir, options.Executable); err != nil {
				t.Fatal("verified owned stop did not wait for exit:", err)
			}
		})
	}
}
func TestWindowsParallelLaunchUsesOneOwner(t *testing.T) {
	root, options := nativeFixture(t)
	options.Destination = None
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- run(options, func(string) error { return errors.New("unexpected browser") })
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Error(e)
		}
	}
	waitOwned(t, options)
	if _, e := os.Stat(filepath.Join(root, "settings-opened")); !os.IsNotExist(e) {
		t.Fatal("parallel startup opened controls")
	}
}
func TestWindowsLaunchBrowserFailureKeepsReadyOwner(t *testing.T) {
	_, options := nativeFixture(t)
	if err := run(options, func(string) error { return errors.New("synthetic browser failure") }); err == nil {
		t.Fatal("lost immediate browser failure")
	}
	waitOwned(t, options)
}
func TestWindowsLaunchFailuresBoundedAndUnknownPortUntouched(t *testing.T) {
	for _, mode := range []string{"missing", "exited", "unready", "occupied"} {
		t.Run(mode, func(t *testing.T) {
			root, options := nativeFixture(t)
			_ = os.WriteFile(filepath.Join(root, "mode"), []byte(mode), 0600)
			if mode == "missing" {
				options.Executable = filepath.Join(root, "missing.exe")
			}
			var listener net.Listener
			if mode == "occupied" {
				var err error
				listener, err = net.Listen("tcp4", bridge.Address)
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
			}
			started := time.Now()
			opened := false
			err := run(options, func(string) error { opened = true; return nil })
			if err == nil || opened {
				t.Fatal("unready child accepted or browser opened")
			}
			if time.Since(started) > 6*time.Second {
				t.Fatal("launch exceeded readiness bound")
			}
			if listener != nil {
				c, e := net.DialTimeout("tcp4", bridge.Address, time.Second)
				if e != nil {
					t.Fatal("unrelated port occupant was terminated")
				}
				c.Close()
			}
			if mode == "unready" {
				raw, _ := os.ReadFile(filepath.Join(root, "child-pid"))
				pid, _ := strconv.Atoi(string(raw))
				h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if e == nil {
					defer windows.CloseHandle(h)
					status, e := windows.WaitForSingleObject(h, 0)
					if e != nil || status != windows.WAIT_OBJECT_0 {
						t.Fatal("unready owned child remained alive")
					}
				}
			}
		})
	}
}

func TestWindowsLauncherProcess(t *testing.T) {
	if os.Getenv("STEAM_SAGE_LAUNCHER_SUBPROCESS") != "1" {
		return
	}
	var options Options
	if e := json.Unmarshal([]byte(os.Getenv("STEAM_SAGE_LAUNCHER_OPTIONS")), &options); e != nil {
		t.Fatal(e)
	}
	if e := run(options, func(string) error { return nil }); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsLauncherExitLeavesDaemonAlive(t *testing.T) {
	_, options := nativeFixture(t)
	options.Destination = None
	raw, _ := json.Marshal(options)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsLauncherProcess$")
	c.Env = append(os.Environ(), "STEAM_SAGE_LAUNCHER_SUBPROCESS=1", "STEAM_SAGE_LAUNCHER_OPTIONS="+string(raw))
	if out, e := c.CombinedOutput(); e != nil {
		t.Fatalf("launcher: %v %s", e, out)
	}
	waitOwned(t, options)
}
