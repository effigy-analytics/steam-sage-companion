package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func activationName(dir string) string {
	absolute, _ := filepath.Abs(dir)
	sum := sha256.Sum256([]byte(absolute))
	return fmt.Sprintf("\x00steam-sage-%d-%s", os.Getuid(), hex.EncodeToString(sum[:12]))
}
func sameUser(conn *net.UnixConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Ucred
	var credErr error
	if raw.Control(func(fd uintptr) { cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }) != nil {
		return false
	}
	return credErr == nil && cred != nil && cred.Uid == uint32(os.Getuid())
}
func RequestReady(configDir string) error {
	return requestActivationTimeout(configDir, 'p', 40*time.Millisecond)
}
func RequestOpen(configDir string) error { return requestActivation(configDir, 'o') }
func RequestStop(configDir string) error { return requestActivation(configDir, 's') }
func requestActivation(configDir string, command byte) error {
	return requestActivationTimeout(configDir, command, 2*time.Second)
}
func requestActivationTimeout(configDir string, command byte, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", activationName(configDir), timeout)
	if err != nil {
		return errors.New("no running companion for these settings")
	}
	defer conn.Close()
	u, ok := conn.(*net.UnixConn)
	if !ok || !sameUser(u) {
		return errors.New("could not verify running companion")
	}
	conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.Write([]byte{command}); err != nil {
		return err
	}
	var response [1]byte
	if _, err = io.ReadFull(conn, response[:]); err != nil || response[0] != 'k' {
		return errors.New("companion activation was not acknowledged")
	}
	return nil
}
func ListenActivation(ctx context.Context, dir string, open func() error, stop ...func()) (io.Closer, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: activationName(dir), Net: "unix"})
	if err != nil {
		return nil, errors.New("companion activation is already in use")
	}
	go func() { <-ctx.Done(); listener.Close() }()
	go func() {
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			if sameUser(conn) {
				var value [1]byte
				if _, err := io.ReadFull(conn, value[:]); err == nil {
					if value[0] == 'p' {
						conn.Write([]byte{'k'})
					}
					if value[0] == 'o' && open() == nil {
						conn.Write([]byte{'k'})
					}
					if value[0] == 's' && len(stop) > 0 {
						conn.Write([]byte{'k'})
						stop[0]()
					}
				}
			}
			conn.Close()
		}
	}()
	return listener, nil
}
