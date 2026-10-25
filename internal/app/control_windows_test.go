package app

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"golang.org/x/sys/windows"
)

func activationName(dir string) string {
	sid, _ := winuser.CurrentSID()
	name, _ := activationNameFor(sid, dir)
	return name
}

func TestWindowsActivationPipeHasProtectedOwnerOnlyACL(t *testing.T) {
	dir, _, _ := windowsActivation(t, func() error { return nil })
	c, err := winio.DialPipe(activationName(dir), durationPtr(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		t.Fatal("pipe handle unavailable")
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := winuser.CurrentSID()
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner.String() != sid {
		t.Fatal("pipe owner differs from process user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("pipe inherited a broad DACL")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		t.Fatal("pipe ACL is not a single explicit user grant")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(acl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERITED_ACE != 0 || (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String() != sid {
		t.Fatal("pipe grants access to an inherited or different identity")
	}
}

func windowsActivation(t *testing.T, open func() error, stop ...func()) (string, context.CancelFunc, io.Closer) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "设置")
	ctx, cancel := context.WithCancel(context.Background())
	l, err := ListenActivation(ctx, dir, open, stop...)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if l == nil {
		cancel()
		t.Fatal("activation listener is missing")
	}
	t.Cleanup(func() { cancel(); l.Close() })
	return dir, cancel, l
}

// Missing authentication, singleton ownership, and readiness/open separation all
// fail through public operations, without touching HTTP or an installed helper.
func TestWindowsActivationReadinessSingletonAndParallelOpen(t *testing.T) {
	var opened atomic.Int32
	dir, _, _ := windowsActivation(t, func() error { opened.Add(1); return nil })
	if err := RequestReady(dir); err != nil {
		t.Fatal(err)
	}
	if opened.Load() != 0 {
		t.Fatal("readiness opened private controls")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if other, err := ListenActivation(ctx, dir, func() error { return nil }); err == nil {
		if other != nil {
			other.Close()
		}
		t.Fatal("second listener took same-config identity")
	}
	if err := RequestReady(filepath.Join(t.TempDir(), "separate")); err == nil {
		t.Fatal("different config reached listener")
	}
	if err := RequestReady(strings.ToLower(dir)); err != nil {
		t.Fatal("case alias lost config identity:", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- RequestOpen(dir) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if opened.Load() != 6 {
		t.Fatalf("opened %d controls, want 6", opened.Load())
	}
}

func TestWindowsActivationNamesSeparateUsersAndCanonicalizeConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Private Settings")
	a, err := activationNameFor("S-1-5-21-1-2-3-1000", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := activationNameFor("S-1-5-21-1-2-3-1001", dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := activationNameFor("S-1-5-21-1-2-3-1000", strings.ToLower(dir))
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b || a != c {
		t.Fatal("pipe identity did not isolate SID and canonical config")
	}
}

func TestWindowsActivationOpenCallbackCanReenterReadiness(t *testing.T) {
	var dir string
	dir, _, _ = windowsActivation(t, func() error { return RequestReady(dir) })
	if err := RequestOpen(dir); err != nil {
		t.Fatal("open callback could not reenter activation readiness:", err)
	}
}

func TestWindowsActivationOpenDeadlineAndMalformedRequests(t *testing.T) {
	var opened atomic.Int32
	release := make(chan struct{})
	defer close(release)
	dir, _, listener := windowsActivation(t, func() error { opened.Add(1); <-release; return nil })
	c, err := winio.DialPipe(activationName(dir), durationPtr(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(time.Second))
	c.Write([]byte{'x'})
	var response [1]byte
	if _, e := io.ReadFull(c, response[:]); e == nil {
		t.Fatal("malformed command was acknowledged")
	}
	c.Close()
	if opened.Load() != 0 {
		t.Fatal("malformed command opened controls")
	}
	started := time.Now()
	if RequestOpen(dir) == nil {
		t.Fatal("unfinished open was acknowledged")
	}
	if elapsed := time.Since(started); elapsed < time.Second || elapsed > 2500*time.Millisecond {
		t.Fatalf("open operation deadline: %s", elapsed)
	}
	if opened.Load() != 1 {
		t.Fatal("open callback did not start exactly once")
	}
	if err = RequestReady(dir); err != nil {
		t.Fatal("blocked open callback held activation coordination mutex:", err)
	}
	started = time.Now()
	listener.Close()
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("shutdown waited for external callback")
	}
}

func TestWindowsActivationRejectsWrongPeerSIDBothDirections(t *testing.T) {
	var opened atomic.Int32
	dir, _, _ := windowsActivation(t, func() error { opened.Add(1); return nil })
	old := peerSID
	defer func() { peerSID = old }()
	peerSID = func(windows.Handle) (string, error) { return "S-1-5-18", nil }
	if err := RequestOpen(dir); err == nil {
		t.Fatal("client accepted wrong server SID")
	}
	c, err := winio.DialPipe(activationName(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	c.Write([]byte{'o'})
	var response [1]byte
	if _, err = io.ReadFull(c, response[:]); err == nil && response[0] == 'k' {
		t.Fatal("server accepted wrong client SID")
	}
	if opened.Load() != 0 {
		t.Fatal("wrong SID opened controls")
	}
}

func TestWindowsActivationRejectsFakePipeAndWrongMaintenanceImage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fake")
	name := activationName(dir)
	if name == "" {
		t.Fatal("missing canonical activation name")
	}
	// A different Windows image owns this pipe; it must not be accepted even
	// though it can produce the one-byte acknowledgement and runs as our user.
	root := t.TempDir()
	fixture := filepath.Join(root, "squatter.exe")
	bytes, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(fixture, bytes, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, fixture, "-test.run=^TestWindowsSquatterProcess$")
	c.Env = append(os.Environ(), "STEAM_SAGE_SQUAT_PIPE="+name)
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait() })
	deadline := time.Now().Add(3 * time.Second)
	for {
		p, e := winio.DialPipe(name, durationPtr(20*time.Millisecond))
		if e == nil {
			p.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not listen")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = RequestOpen(dir); err == nil {
		t.Fatal("accepted squatted pipe from another executable")
	}
	owned, _, _ := windowsActivation(t, func() error { return nil })
	if err = RequestStopAndWait(owned, fixture); err == nil {
		t.Fatal("maintenance accepted mismatched peer image")
	}
	if err = RequestReady(owned); err != nil {
		t.Fatal("mismatched maintenance stopped owner:", err)
	}
}
func durationPtr(d time.Duration) *time.Duration { return &d }
func TestWindowsSquatterProcess(t *testing.T) {
	name := os.Getenv("STEAM_SAGE_SQUAT_PIPE")
	if name == "" {
		return
	}
	l, err := winio.ListenPipe(name, nil)
	if err != nil {
		os.Exit(20)
	}
	defer l.Close()
	for {
		c, e := l.Accept()
		if e != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			c.SetDeadline(time.Now().Add(time.Second))
			var b [1]byte
			io.ReadFull(c, b[:])
			c.Write([]byte{'k'})
		}(c)
	}
}

func TestWindowsActivationDeadlineAndShutdown(t *testing.T) {
	dir, cancel, l := windowsActivation(t, func() error { return nil })
	// Eight silent connections cannot indefinitely starve probes or shutdown.
	var clients []net.Conn
	for range 8 {
		c, err := winio.DialPipe(activationName(dir), durationPtr(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()
	start := time.Now()
	_ = RequestReady(dir)
	if time.Since(start) > 250*time.Millisecond {
		t.Fatal("readiness exceeded short deadline")
	}
	cancel()
	l.Close()
	for _, c := range clients {
		c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var b [1]byte
		if _, e := c.Read(b[:]); e == nil {
			t.Fatal("shutdown left a live handler")
		} else if timeout, ok := e.(net.Error); ok && timeout.Timeout() {
			t.Fatal("shutdown did not close the active connection")
		}
	}
	start = time.Now()
	if RequestReady(dir) == nil {
		t.Fatal("closed listener remained ready")
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Fatal("closed probe did not finish promptly")
	}
}

func TestWindowsStopAndWaitHonorsOneOverallDeadline(t *testing.T) {
	var stopped atomic.Int32
	dir, _, _ := windowsActivation(t, func() error { return nil }, func() { stopped.Add(1) })
	start := time.Now()
	err := RequestStopAndWait(dir, os.Args[0])
	if err == nil {
		t.Fatal("reported current test process exited")
	}
	if stopped.Load() != 1 {
		t.Fatal("verified owner did not receive stop")
	}
	if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed > 5500*time.Millisecond {
		t.Fatalf("overall stop deadline: %s", elapsed)
	}
}
