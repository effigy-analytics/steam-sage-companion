package main

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
)

func TestServeRejectsOccupiedPort(t *testing.T) {
	requireIsolatedNetwork(t)
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
	if command != "launch" || len(args) != 0 {
		t.Fatal("default launch must start Sage without arguments")
	}
	command, args = commandArgs([]string{"serve", "--background"})
	if command != "serve" || len(args) != 1 || args[0] != "--background" {
		t.Fatal("explicit command lost its flags")
	}
}

// Fixed-port subprocesses must never contact the owner's installed helper.
func requireIsolatedNetwork(t *testing.T) {
	t.Helper()
	current, err := os.Readlink("/proc/self/ns/net")
	host := os.Getenv("STEAM_SAGE_TEST_HOST_NETWORK")
	if err != nil || os.Getenv("STEAM_SAGE_ISOLATED_NETWORK") != "1" || host == "" || current == host {
		t.Skip("run fixed-port integration tests with scripts/with-isolated-network.sh")
	}
}
