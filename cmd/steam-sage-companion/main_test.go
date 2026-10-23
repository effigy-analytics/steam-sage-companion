package main

import (
	"context"
	"net"
	"testing"

	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
)

func TestServeRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp4", bridge.Address)
	if err != nil {
		t.Skip("test port already occupied")
	}
	defer listener.Close()
	if err := serve(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("did not report an occupied port")
	}
}

func TestDefaultCommandHasNoFlags(t *testing.T) {
	command, args := commandArgs(nil)
	if command != "open" || len(args) != 0 {
		t.Fatal("default launch must open controls without arguments")
	}
	command, args = commandArgs([]string{"serve", "--background"})
	if command != "serve" || len(args) != 1 || args[0] != "--background" {
		t.Fatal("explicit command lost its flags")
	}
}
