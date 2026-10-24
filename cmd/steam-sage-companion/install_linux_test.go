package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"context"
	"fmt"
	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
	"golang.org/x/sys/unix"
)

func TestLinuxInstallRestartRevokeUpgradeUninstall(t *testing.T) {
	requireIsolatedNetwork(t)
	probe, err := net.Listen("tcp4", bridge.Address)
	if err != nil {
		t.Skip("companion port is occupied; run installation test with the helper stopped")
	}
	probe.Close()
	home := filepath.Join(t.TempDir(), "person space ñ %")
	cfg := filepath.Join(home, ".config", "steam-sage-companion")
	data := filepath.Join(home, ".local", "share")
	os.MkdirAll(home, 0700)
	original := filepath.Join(t.TempDir(), "helper")
	build := exec.Command("go", "build", "-o", original, ".")
	if err := build.Run(); err != nil {
		t.Fatalf("build integration executable: %v", err)
	}
	bin := filepath.Join(home, ".local", "bin", "steam-sage-companion")
	desktop := filepath.Join(data, "applications", "steam-sage-companion.desktop")
	startup := filepath.Join(home, ".config", "autostart", "steam-sage-companion.desktop")
	openFile := filepath.Join(home, "control-url")
	mockBin := filepath.Join(home, "mock-tools")
	os.MkdirAll(mockBin, 0700)
	os.WriteFile(filepath.Join(mockBin, "xdg-open"), []byte("#!/bin/sh\numask 077\nprintf '%s' \"$1\" > \"$STEAM_SAGE_TEST_OPEN_FILE\"\n"), 0700)
	env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+data, "PATH="+mockBin+":"+os.Getenv("PATH"), "STEAM_SAGE_TEST_OPEN_FILE="+openFile, "DBUS_SESSION_BUS_ADDRESS=disabled:", "DISPLAY=", "WAYLAND_DISPLAY=", "XDG_RUNTIME_DIR="+filepath.Join(home, "runtime"))
	var output bytes.Buffer
	run := func(exe string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		output.Write(out)
		if err != nil {
			t.Fatalf("command %s failed: %v", args[0], err)
		}
		return out
	}
	run(original, "install", "--config-dir", cfg)
	if _, err := os.Stat(bin); err != nil {
		t.Fatal("installed executable missing")
	}
	if _, err := os.Stat(desktop); err != nil {
		t.Fatal("desktop launcher missing")
	}
	if _, err := os.Stat(startup); !os.IsNotExist(err) {
		t.Fatal("startup enabled without choosing it")
	}
	keep := filepath.Join(filepath.Dir(bin), "keep.txt")
	os.WriteFile(keep, []byte("unrelated"), 0600)
	steamDir := filepath.Join(data, "Steam", "userdata", "1", "config", "cloudstorage")
	os.MkdirAll(steamDir, 0700)
	steamFile := filepath.Join(steamDir, "cloud-storage-namespace-1.json")
	source := []byte(`[["user-collections.done",{"value":"{\"id\":\"done\",\"name\":\"Completed\",\"added\":[1],\"removed\":[]}"}]]`)
	os.WriteFile(steamFile, source, 0600)
	var daemon *exec.Cmd
	stop := func() {
		if daemon == nil {
			app.RequestStop(cfg)
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				if app.RequestReady(cfg) != nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Error("detached helper did not stop")
			return
		}
		if daemon.Process != nil {
			daemon.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() { done <- daemon.Wait() }()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				daemon.Process.Kill()
				<-done
				t.Error("helper did not stop gracefully")
			}
			daemon = nil
		}
	}
	defer stop()
	digest := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	mac := func(key string, parts []string) string {
		raw, _ := json.Marshal(parts)
		m := hmac.New(sha256.New, []byte(key))
		m.Write(raw)
		return hex.EncodeToString(m.Sum(nil))
	}
	var call func(method, path, origin, token string, body any, control bool) (int, map[string]any)
	call = func(method, path, origin, token string, body any, control bool) (int, map[string]any) {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		request, _ := http.NewRequest(method, bridge.LocalOrigin+path, bytes.NewReader(payload))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			if control {
				request.Header.Set("X-Companion-Control", token)
			} else {
				key := digest(token)
				clientNonce := strings.Repeat("a", 43)
				status, value := call("POST", "/v1/challenge", origin, "", map[string]string{"pairing_id": digest(key), "client_nonce": clientNonce, "method": method, "path": path, "body_sha256": digest(string(payload))}, false)
				if status != 201 {
					return status, value
				}
				parts := []string{origin, value["challenge_id"].(string), clientNonce, value["server_nonce"].(string), method, path, digest(string(payload))}
				if value["proof"] != mac(key, append([]string{"steam-sage-companion:v1:server"}, parts...)) {
					t.Fatal("unverified helper")
				}
				request.Header.Set("Authorization", "SageProof "+value["challenge_id"].(string)+"."+mac(key, append([]string{"steam-sage-companion:v1:client"}, parts...)))
			}
		}
		client := http.Client{Timeout: time.Second}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("local HTTP request failed")
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
		var result map[string]any
		json.Unmarshal(raw, &result)
		return response.StatusCode, result
	}
	start := func() {
		t.Helper()
		daemon = exec.Command(bin, "serve", "--background", "--config-dir", cfg)
		daemon.Env = env
		if err := daemon.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			conn, err := net.DialTimeout("tcp4", bridge.Address, 50*time.Millisecond)
			if err == nil {
				conn.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("installed helper did not listen")
	}
	packageDir := filepath.Join(t.TempDir(), "extracted download ñ")
	os.MkdirAll(packageDir, 0700)
	downloaded := filepath.Join(packageDir, "steam-sage-companion")
	dataBinary, _ := os.ReadFile(original)
	os.WriteFile(downloaded, dataBinary, 0700)
	script, _ := os.ReadFile("../../packaging/linux/install.sh")
	installer := filepath.Join(packageDir, "install.sh")
	os.WriteFile(installer, script, 0700)
	run("sh", installer, "--config-dir", cfg)
	// The normal launcher opens Sage. The explicit settings alias preserves manual controls.
	run(bin, "settings", "--config-dir", cfg)
	var controlURL *url.URL
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(openFile); err == nil {
			parsed, _ := url.Parse(string(raw))
			if parsed != nil && parsed.Scheme+"://"+parsed.Host == bridge.LocalOrigin && strings.HasPrefix(parsed.Fragment, "launch=") {
				controlURL = parsed
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if controlURL == nil {
		t.Fatal("shortcut did not open local controls")
	}
	cap := strings.TrimPrefix(controlURL.Fragment, "launch=")
	code, value := call("POST", "/control/session", bridge.LocalOrigin, "", map[string]string{"capability": cap}, false)
	if code != 200 {
		t.Fatal("control launch failed")
	}
	controlToken, _ := value["control_token"].(string)
	code, _ = call("POST", "/control/startup", bridge.LocalOrigin, controlToken, map[string]bool{"enabled": true}, true)
	if code != 200 {
		t.Fatal("startup selection failed")
	}
	os.RemoveAll(packageDir)
	entry, _ := os.ReadFile(startup)
	if !strings.Contains(string(entry), "%%") || !strings.Contains(string(entry), "serve") || strings.Contains(string(entry), packageDir) {
		t.Fatal("startup path was not escaped")
	}
	verifier := "synthetic-verifier"
	sum := sha256.Sum256([]byte(verifier))
	code, value = call("POST", "/v1/pair/request", bridge.ProductionOrigin, "", map[string]string{"challenge_sha256": hex.EncodeToString(sum[:])}, false)
	if code != 201 {
		t.Fatal("pair request failed")
	}
	requestID := value["request_id"].(string)
	code, _ = call("POST", "/control/approve", bridge.LocalOrigin, controlToken, map[string]string{"request_id": requestID}, true)
	if code != 200 {
		t.Fatal("approval failed")
	}
	code, value = call("POST", "/v1/pair/claim", bridge.ProductionOrigin, "", map[string]string{"request_id": requestID, "verifier": verifier}, false)
	if code != 200 {
		t.Fatal("claim failed")
	}
	token := value["token"].(string)
	stop()
	start()
	code, value = call("GET", "/v1/snapshot", bridge.ProductionOrigin, token, nil, false)
	if code != 200 || value["state"] != "ready" {
		t.Fatal("pairing/snapshot did not survive restart")
	}
	code, _ = call("DELETE", "/v1/pairing", bridge.ProductionOrigin, token, nil, false)
	if code != 200 {
		t.Fatal("revoke failed")
	}
	code, _ = call("GET", "/v1/snapshot", bridge.ProductionOrigin, token, nil, false)
	if code != 401 {
		t.Fatal("revoked token still works")
	}
	run(original, "install", "--config-dir", cfg)
	stop()
	if _, err := os.Stat(startup); err != nil {
		t.Fatal("upgrade lost opted-in startup")
	}
	run(original, "uninstall", "--config-dir", cfg)
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatal("uninstall left executable")
	}
	if _, err := os.Stat(startup); !os.IsNotExist(err) {
		t.Fatal("uninstall left startup entry")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("uninstall removed unrelated file")
	}
	if after, _ := os.ReadFile(steamFile); !bytes.Equal(after, source) {
		t.Fatal("installation changed Steam data")
	}
	if _, err := os.Stat(filepath.Join(cfg, "settings.json")); err != nil {
		t.Fatal("uninstall unexpectedly purged settings")
	}
	run(original, "uninstall", "--purge", "--config-dir", cfg)
	if _, err := os.Stat(filepath.Join(cfg, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("explicit purge did not remove owned settings")
	}
	for _, secret := range []string{cap, controlToken, token} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("a private capability was printed")
		}
	}
}

type installedFixture struct {
	binary, cfg, home, opened string
	env                       []string
}

func newInstalledFixture(t *testing.T) installedFixture {
	t.Helper()
	requireIsolatedNetwork(t)
	home := filepath.Join(t.TempDir(), "person space ñ %")
	cfg := filepath.Join(home, "custom settings ñ")
	tools := filepath.Join(home, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	opened := filepath.Join(home, "opened-urls")
	if err := os.WriteFile(filepath.Join(tools, "xdg-open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$STEAM_SAGE_TEST_OPEN_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_RUNTIME_DIR="+filepath.Join(home, "runtime"), "PATH="+tools+":"+os.Getenv("PATH"), "DBUS_SESSION_BUS_ADDRESS=disabled:", "DISPLAY=", "WAYLAND_DISPLAY=", "STEAM_SAGE_TEST_OPEN_FILE="+opened)
	original := filepath.Join(t.TempDir(), "downloaded-helper")
	build := exec.Command("go", "build", "-o", original, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	f := installedFixture{binary: filepath.Join(home, ".local", "bin", "steam-sage-companion"), cfg: cfg, home: home, opened: opened, env: env}
	f.run(t, original, "install", "--config-dir", cfg)
	t.Cleanup(func() {
		app.RequestStop(cfg)
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if app.RequestReady(cfg) != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("synthetic detached helper did not stop")
	})
	return f
}
func (f installedFixture) run(t *testing.T, exe string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, exe, args...)
	command.Env = f.env
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v %s", args, err, out)
	}
}
func (f installedFixture) sageOnly(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(f.opened); err == nil {
			if string(raw) != "https://steam-sage.effigy-analytics.com/companion?installed=1\n" {
				t.Fatalf("normal launch opened private or unexpected URL: %s", raw)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Sage was not opened")
}
func TestLaunchReturnsAndHelperSurvivesParentExit(t *testing.T) {
	f := newInstalledFixture(t)
	command := exec.Command(f.binary, "launch", "--config-dir", f.cfg, "--allow-origin", "http://localhost:5173")
	command.Env = f.env
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	if err := app.RequestReady(f.cfg); err != nil {
		t.Fatal("helper depended on launcher lifetime:", err)
	}
	f.sageOnly(t)
	pid, _ := strconv.Atoi(installedHelperPID(t, f.cfg))
	if sid, err := unix.Getsid(pid); err != nil || sid != pid {
		t.Fatalf("helper was not detached from launcher session: %d %v", sid, err)
	}
	request, _ := http.NewRequest("GET", bridge.LocalOrigin+"/v1/status", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("custom origin was lost in detached argv")
	}
	var status struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(status.Capabilities, ","), "pairing-ui-v1") {
		t.Fatal("real observed browser dispatcher was not wired into bridge")
	}
}
func TestLaunchReusesOwnedHelperWithoutOpeningControls(t *testing.T) {
	f := newInstalledFixture(t)
	daemon := exec.Command(f.binary, "serve", "--background", "--config-dir", f.cfg)
	daemon.Env = f.env
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		done := make(chan error, 1)
		go func() { done <- daemon.Wait() }()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			app.RequestStop(f.cfg)
			select {
			case <-done:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		daemon.Process.Kill()
		<-done
		t.Error("synthetic helper did not stop")
	})
	deadline := time.Now().Add(3 * time.Second)
	for app.RequestReady(f.cfg) != nil {
		if time.Now().After(deadline) {
			t.Fatal("owned helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := installedHelperPID(t, f.cfg)
	f.run(t, f.binary, "launch", "--config-dir", f.cfg)
	f.sageOnly(t)
	if after := installedHelperPID(t, f.cfg); after != before {
		t.Fatalf("launch replaced owned helper %s with %s", before, after)
	}
}
func TestUnknownPortOccupantIsNotTrustedOrStopped(t *testing.T) {
	f := newInstalledFixture(t)
	listener, err := net.Listen("tcp4", bridge.Address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, f.binary, "launch", "--config-dir", f.cfg)
	command.Env = f.env
	if out, err := command.CombinedOutput(); err == nil {
		t.Fatalf("trusted unrelated port occupant: %s", out)
	} else if !strings.Contains(string(out), "local port") {
		t.Fatalf("did not diagnose readiness failure: %s", out)
	}
	if conn, err := net.DialTimeout("tcp4", bridge.Address, time.Second); err != nil {
		t.Fatal("unrelated listener was stopped:", err)
	} else {
		conn.Close()
	}
	if _, err := os.Stat(f.opened); !os.IsNotExist(err) {
		t.Fatal("opened Sage despite readiness failure")
	}
}
func TestInstallerStartsInstalledCopyAndPreservesStartup(t *testing.T) {
	f := newInstalledFixture(t)
	// Enable the existing optional startup choice without using any owner's settings.
	if err := os.MkdirAll(f.cfg, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.cfg, "settings.json"), []byte(`{"kind":"steam-sage-companion","instance_id":"synthetic-instance","startup":true,"selected_account":"kept","selected_steam_id":"99","sources":{"synthetic-source":"kept-source"},"pairings":[{"hash":"kept-hash","origin":"https://steam-sage.effigy-analytics.com","steam_id":"99"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(t.TempDir(), "download ñ")
	os.MkdirAll(packageDir, 0700)
	binary, _ := os.ReadFile(f.binary)
	os.WriteFile(filepath.Join(packageDir, "steam-sage-companion"), binary, 0700)
	script, _ := os.ReadFile("../../packaging/linux/install.sh")
	installer := filepath.Join(packageDir, "install.sh")
	os.WriteFile(installer, script, 0700)
	f.run(t, "sh", installer, "--config-dir", f.cfg)
	os.RemoveAll(packageDir)
	f.sageOnly(t)
	if err := app.RequestReady(f.cfg); err != nil {
		t.Fatal("installed helper did not survive installer:", err)
	}
	childExe, err := os.Readlink("/proc/" + installedHelperPID(t, f.cfg) + "/exe")
	if err != nil || childExe != f.binary {
		t.Fatalf("installer did not start installed copy: %q %v", childExe, err)
	}
	raw, _ := os.ReadFile(filepath.Join(f.home, ".config", "autostart", "steam-sage-companion.desktop"))
	if !strings.Contains(string(raw), "serve") || !strings.Contains(string(raw), "--background") || strings.Contains(string(raw), packageDir) {
		t.Fatalf("startup did not retain installed background entry: %s", raw)
	}
	raw, _ = os.ReadFile(filepath.Join(f.cfg, "settings.json"))
	for _, choice := range []string{`"startup":true`, `"selected_account":"kept"`, `"selected_steam_id":"99"`, `"synthetic-source":"kept-source"`, `"hash":"kept-hash"`} {
		if !bytes.Contains(raw, []byte(choice)) {
			t.Fatalf("upgrade lost choice %s", choice)
		}
	}
}

func installedHelperPID(t *testing.T, dir string) string {
	t.Helper()
	absolute, _ := filepath.Abs(dir)
	sum := sha256.Sum256([]byte(absolute))
	address := fmt.Sprintf("\x00steam-sage-%d-%s", os.Getuid(), hex.EncodeToString(sum[:12]))
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var peer *unix.Ucred
	var peerErr error
	if err := raw.Control(func(fd uintptr) { peer, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil || peerErr != nil || peer == nil {
		t.Fatal("cannot identify synthetic installed child")
	}
	return fmt.Sprint(peer.Pid)
}

func TestInstalledNativeOpeningReportsDispatchFailures(t *testing.T) {
	for _, mode := range []string{"immediate", "late"} {
		t.Run(mode, func(t *testing.T) {
			f := newInstalledFixture(t)
			f.run(t, f.binary, "launch", "--config-dir", f.cfg)
			f.sageOnly(t)
			dispatcher := filepath.Join(f.home, "tools", "xdg-open")
			if mode == "immediate" {
				os.WriteFile(dispatcher, []byte("#!/nonexistent-steam-sage-test-interpreter\n"), 0700)
			} else {
				os.WriteFile(dispatcher, []byte("#!/bin/sh\n/bin/sleep 0.1\nexit 7\n"), 0700)
			}
			client := http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
			call := func(path string, body any) (int, map[string]any) {
				t.Helper()
				payload, _ := json.Marshal(body)
				request, _ := http.NewRequest("POST", bridge.LocalOrigin+path, bytes.NewReader(payload))
				request.Header.Set("Origin", bridge.ProductionOrigin)
				request.Header.Set("Content-Type", "application/json")
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				var value map[string]any
				if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
					t.Fatal(err)
				}
				return response.StatusCode, value
			}
			sum := sha256.Sum256([]byte("original-verifier"))
			code, pair := call("/v1/pair/request", map[string]string{"challenge_sha256": hex.EncodeToString(sum[:])})
			if code != 201 {
				t.Fatalf("pair request: %d %v", code, pair)
			}
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				code, claim := call("/v1/pair/claim", map[string]any{"request_id": pair["request_id"], "verifier": "original-verifier"})
				if code != 202 {
					t.Fatalf("dispatch failure approved or lost the pending request: %d %v", code, claim)
				}
				if claim["approval_ui"] == "failed" {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("real dispatcher failure was not reported to the original pending request")
		})
	}
}
