package app

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestActivationOnlyOpensForValidSameUserRequest(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan bool, 2)
	listener, err := ListenActivation(ctx, dir, func() error { opened <- true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := RequestOpen(dir); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("activation did not open controls")
	}
	conn, err := net.Dial("unix", activationName(dir))
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte{'x'})
	conn.Close()
	select {
	case <-opened:
		t.Fatal("invalid activation opened controls")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestReadinessDoesNotOpenControls(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan bool, 1)
	listener, err := ListenActivation(ctx, dir, func() error { opened <- true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := requestActivation(dir, 'p'); err != nil {
		t.Fatal("owned readiness probe failed:", err)
	}
	select {
	case <-opened:
		t.Fatal("readiness minted or opened a control capability")
	default:
	}
}
