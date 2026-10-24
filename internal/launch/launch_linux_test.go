package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLaunchFailuresAreBoundedAndOnlyTerminateSpawnedChild(t *testing.T) {
	current, _ := os.Readlink("/proc/self/ns/net")
	if os.Getenv("STEAM_SAGE_ISOLATED_NETWORK") != "1" || current == os.Getenv("STEAM_SAGE_TEST_HOST_NETWORK") {
		t.Skip("use scripts/with-isolated-network.sh")
	}
	for _, tc := range []struct {
		name, script, want string
		limit              time.Duration
	}{
		{"missing", "", "cannot start the installed companion", time.Second},
		{"exited", "#!/bin/sh\nexit 7\n", "companion could not start", time.Second},
		{"unready", "#!/bin/sh\nprintf '%s' \"$$\" > \"$STEAM_SAGE_TEST_CHILD_PID\"\nexec /bin/sleep 60\n", "companion did not become ready", 6 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			tools := filepath.Join(home, "tools")
			os.MkdirAll(tools, 0700)
			executable := filepath.Join(home, "synthetic-child")
			if tc.script != "" {
				if err := os.WriteFile(executable, []byte(tc.script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			pidFile := filepath.Join(home, "child-pid")
			opened := filepath.Join(home, "opened")
			os.WriteFile(filepath.Join(tools, "xdg-open"), []byte("#!/bin/sh\nprintf opened > \"$STEAM_SAGE_TEST_OPENED\"\n"), 0700)
			command := exec.Command(os.Args[0], "-test.run=^TestLaunchFailureProcess$")
			command.Env = append(os.Environ(), "STEAM_SAGE_LAUNCH_FAILURE_PROCESS=1", "GORACE=atexit_sleep_ms=0", "STEAM_SAGE_TEST_EXECUTABLE="+executable, "STEAM_SAGE_TEST_CONFIG="+filepath.Join(home, "config"), "STEAM_SAGE_TEST_CHILD_PID="+pidFile, "STEAM_SAGE_TEST_OPENED="+opened, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_RUNTIME_DIR="+filepath.Join(home, "runtime"), "PATH="+tools, "DBUS_SESSION_BUS_ADDRESS=disabled:", "DISPLAY=", "WAYLAND_DISPLAY=")
			started := time.Now()
			out, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("launch failure: %v %s", err, out)
			}
			if time.Since(started) > tc.limit {
				t.Fatalf("launch did not honor readiness/exit bound: %s", time.Since(started))
			}
			if _, err := os.Stat(opened); !os.IsNotExist(err) {
				t.Fatal("opened Sage without owned readiness")
			}
			if raw, err := os.ReadFile(pidFile); err == nil {
				pid, err := strconv.Atoi(string(raw))
				if err != nil {
					t.Fatal(err)
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					syscall.Kill(pid, syscall.SIGKILL)
					t.Fatal("unready spawned child was not reaped")
				}
			}
		})
	}
}
func TestLaunchFailureProcess(t *testing.T) {
	if os.Getenv("STEAM_SAGE_LAUNCH_FAILURE_PROCESS") != "1" {
		return
	}
	err := Run(Options{Executable: os.Getenv("STEAM_SAGE_TEST_EXECUTABLE"), ConfigDir: os.Getenv("STEAM_SAGE_TEST_CONFIG")})
	if err == nil {
		t.Fatal("unready child was accepted")
	}
	fmt.Println(err)
}
