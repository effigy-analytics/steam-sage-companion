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
	"strings"
	"testing"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
)

func TestLinuxInstallRestartRevokeUpgradeUninstall(t *testing.T) {
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
	env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+data, "PATH="+mockBin+":"+os.Getenv("PATH"), "STEAM_SAGE_TEST_OPEN_FILE="+openFile)
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
		if daemon != nil && daemon.Process != nil {
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
	daemon = exec.Command("sh", installer, "--config-dir", cfg)
	daemon.Env = env
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	var controlURL *url.URL
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(openFile); err == nil {
			controlURL, _ = url.Parse(string(raw))
			break
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
