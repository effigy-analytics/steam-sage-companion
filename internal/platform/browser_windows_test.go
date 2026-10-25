package platform

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsBrowserImmediateAndLateFailure(t *testing.T) {
	old := browserCommand
	defer func() { browserCommand = old }()
	browserCommand = func(string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing.exe")) }
	if err := OpenBrowserObserved("http://127.0.0.1:43821/private", nil); err == nil {
		t.Fatal("lost immediate browser dispatch error")
	}
	root := t.TempDir()
	result := filepath.Join(root, "browser-result")
	browserCommand = func(url string) *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=^TestWindowsBrowserProcess$", "--", url)
		c.Env = append(os.Environ(), "STEAM_SAGE_FAKE_BROWSER="+result)
		return c
	}
	exited := make(chan error, 1)
	const url = "http://127.0.0.1:43821/控制?name=用户%20space"
	if err := OpenBrowserObserved(url, func(err error) { exited <- err }); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err == nil {
			t.Fatal("late nonzero browser exit was lost")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("browser callback did not finish")
	}
	raw, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		URL     string
		Console bool
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != url {
		t.Fatal("browser Unicode arguments changed")
	}
	if got.Console {
		t.Fatal("browser dispatcher inherited or created a visible console")
	}
}
func TestWindowsBrowserProcess(t *testing.T) {
	path := os.Getenv("STEAM_SAGE_FAKE_BROWSER")
	if path == "" {
		return
	}
	console, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	raw, _ := json.Marshal(struct {
		URL     string
		Console bool
	}{os.Args[len(os.Args)-1], console != 0})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		os.Exit(16)
	}
	os.Exit(7)
}
