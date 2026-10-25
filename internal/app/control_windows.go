package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/effigy-analytics/steam-sage-companion/internal/winuser"
	"golang.org/x/sys/windows"
)

// ErrNotRunning means only that the independently named activation pipe does not exist.
// Other connection and peer-verification failures must never permit package mutation.
var ErrNotRunning = errors.New("companion activation pipe is absent")

func activationNameFor(sid, dir string) (string, error) {
	if _, err := windows.StringToSid(sid); err != nil {
		return "", err
	}
	canonical, err := winuser.CanonicalDir(dir)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(sid + "\x00" + strings.ToLower(canonical)))
	return `\\.\pipe\steam-sage-` + hex.EncodeToString(sum[:]), nil
}

// This adapter is the token-query boundary, kept separate from pipe/process
// ownership so native tests can exercise fail-closed peer identity failures.
var peerSID = func(process windows.Handle) (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// canonicalExecutable resolves filesystem spelling, rejecting reparse leaves
// and ancestors. A path supplied by an untrusted pipe is never used here.
func canonicalExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("expected executable must be absolute")
	}
	if _, err := winuser.CanonicalDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	if base := filepath.Base(path); strings.HasSuffix(base, ".") || strings.HasSuffix(base, " ") {
		return "", errors.New("ambiguous executable name")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return "", err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return "", errors.New("unsafe executable path")
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buf)) {
		return "", errors.New("executable path too long")
	}
	result := windows.UTF16ToString(buf[:n])
	if strings.HasPrefix(result, `\\?\UNC\`) {
		result = `\\` + strings.TrimPrefix(result, `\\?\UNC\`)
	} else {
		result = strings.TrimPrefix(result, `\\?\`)
	}
	return result, nil
}

func verifiedPipePeer(conn net.Conn, server bool, expectedExecutable string) (windows.Handle, error) {
	file, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("pipe handle unavailable")
	}
	var pid uint32
	var err error
	if server {
		err = windows.GetNamedPipeClientProcessId(windows.Handle(file.Fd()), &pid)
	} else {
		err = windows.GetNamedPipeServerProcessId(windows.Handle(file.Fd()), &pid)
	}
	if err != nil || pid == 0 {
		return 0, errors.New("cannot identify companion peer")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, errors.New("cannot inspect companion peer")
	}
	valid := false
	defer func() {
		if !valid {
			windows.CloseHandle(process)
		}
	}()
	sid, err := winuser.CurrentSID()
	if err != nil {
		return 0, err
	}
	actual, err := peerSID(process)
	if err != nil || actual != sid {
		return 0, errors.New("companion peer belongs to another user")
	}
	if !server {
		expected, err := canonicalExecutable(expectedExecutable)
		if err != nil {
			return 0, errors.New("cannot verify expected companion executable")
		}
		buf := make([]uint16, 32768)
		n := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(process, 0, &buf[0], &n); err != nil {
			return 0, errors.New("cannot inspect companion image")
		}
		image, err := canonicalExecutable(windows.UTF16ToString(buf[:n]))
		if err != nil || !strings.EqualFold(expected, image) {
			return 0, errors.New("running companion executable does not match the expected copy")
		}
	}
	valid = true
	return process, nil
}

func RequestReady(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return RequestReadyFor(dir, exe)
}
func RequestOpen(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return RequestOpenFor(dir, exe)
}
func RequestStop(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return requestActivationFor(dir, exe, 's', 2*time.Second)
}
func RequestReadyFor(dir, exe string) error {
	return requestActivationFor(dir, exe, 'p', 40*time.Millisecond)
}
func RequestOpenFor(dir, exe string) error { return requestActivationFor(dir, exe, 'o', 2*time.Second) }
func requestActivationFor(dir, exe string, command byte, timeout time.Duration) error {
	h, err := requestVerifiedActivation(dir, exe, command, time.Now().Add(timeout))
	if h != 0 {
		windows.CloseHandle(h)
	}
	return err
}
func requestVerifiedActivation(dir, exe string, command byte, deadline time.Time) (windows.Handle, error) {
	sid, err := winuser.CurrentSID()
	if err != nil {
		return 0, err
	}
	name, err := activationNameFor(sid, dir)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return 0, errors.Join(ErrNotRunning, err)
		}
		return 0, errors.Join(errors.New("cannot verify running companion for these settings"), err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(deadline); err != nil {
		return 0, err
	}
	peer, err := verifiedPipePeer(conn, false, exe)
	if err != nil {
		return 0, err
	}
	acknowledged := false
	defer func() {
		if !acknowledged {
			windows.CloseHandle(peer)
		}
	}()
	if _, err = conn.Write([]byte{command}); err != nil {
		return 0, errors.New("companion activation could not be sent")
	}
	var response [1]byte
	if _, err = io.ReadFull(conn, response[:]); err != nil || response[0] != 'k' {
		return 0, errors.New("companion activation was not acknowledged")
	}
	acknowledged = true
	return peer, nil
}

// RequestStopAndWait holds the process handle obtained from the authenticated
// pipe. An acknowledgement alone is never treated as permission to replace it.
// All connection, acknowledgement and exit waits share one five-second budget.
func RequestStopAndWait(dir, expectedExecutable string) error {
	deadline := time.Now().Add(5 * time.Second)
	peer, err := requestVerifiedActivation(dir, expectedExecutable, 's', time.Now().Add(2*time.Second))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(peer)
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return errors.New("companion did not exit before the stop deadline")
	}
	status, err := windows.WaitForSingleObject(peer, uint32(remaining.Milliseconds()))
	if err != nil || status != windows.WAIT_OBJECT_0 {
		return errors.New("companion did not exit before the stop deadline")
	}
	return nil
}

type activationListener struct {
	listener net.Listener
	mu       sync.Mutex
	active   map[net.Conn]bool
	closed   chan struct{}
	once     sync.Once
	slots    chan struct{}
}

func (a *activationListener) Close() error {
	var err error
	a.once.Do(func() {
		close(a.closed)
		err = a.listener.Close()
		a.mu.Lock()
		connections := make([]net.Conn, 0, len(a.active))
		for conn := range a.active {
			connections = append(connections, conn)
		}
		a.mu.Unlock()
		for _, conn := range connections {
			conn.Close()
		}
	})
	return err
}
func ListenActivation(ctx context.Context, dir string, open func() error, stop ...func()) (io.Closer, error) {
	sid, err := winuser.CurrentSID()
	if err != nil {
		return nil, err
	}
	name, err := activationNameFor(sid, dir)
	if err != nil {
		return nil, err
	}
	// go-winio v0.6.4 uses an exclusive first instance and rejects remote clients.
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: "O:" + sid + "D:P(A;;GA;;;" + sid + ")", InputBufferSize: 64, OutputBufferSize: 64})
	if err != nil {
		return nil, errors.New("companion activation is already in use or unavailable")
	}
	a := &activationListener{listener: l, active: make(map[net.Conn]bool), closed: make(chan struct{}), slots: make(chan struct{}, 8)}
	go func() {
		select {
		case <-ctx.Done():
			a.Close()
		case <-a.closed:
		}
	}()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			select {
			case a.slots <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			a.mu.Lock()
			select {
			case <-a.closed:
				a.mu.Unlock()
				conn.Close()
				<-a.slots
				return
			default:
			}
			a.active[conn] = true
			a.mu.Unlock()
			go a.handle(conn, open, stop)
		}
	}()
	return a, nil
}
func (a *activationListener) handle(conn net.Conn, open func() error, stop []func()) {
	defer func() { conn.Close(); a.mu.Lock(); delete(a.active, conn); a.mu.Unlock(); <-a.slots }()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return
	}
	peer, err := verifiedPipePeer(conn, true, "")
	if err != nil {
		return
	}
	windows.CloseHandle(peer)
	var command [1]byte
	if _, err := io.ReadFull(conn, command[:]); err != nil {
		return
	}
	select {
	case <-a.closed:
		return
	default:
	}
	switch command[0] {
	case 'p':
		conn.Write([]byte{'k'})
	case 'o':
		if open != nil && open() == nil {
			conn.Write([]byte{'k'})
		}
	case 's':
		if len(stop) > 0 && stop[0] != nil {
			if _, err := conn.Write([]byte{'k'}); err == nil {
				stop[0]()
			}
		}
	}
}
