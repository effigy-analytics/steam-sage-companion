package platform

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"reflect"
	"testing"
)

func TestWindowsSteamRegistryViewsAndUnicodePaths(t *testing.T) {
	oldRead, oldEnv := readSteamRegistry, steamEnvironment
	t.Cleanup(func() { readSteamRegistry = oldRead; steamEnvironment = oldEnv })
	steamEnvironment = func(string) string { return "" }
	readSteamRegistry = func(root registry.Key, path, name string, access uint32) (string, error) {
		if path != `Software\Valve\Steam` && path != `SOFTWARE\Valve\Steam` {
			return "", errors.New("unexpected path")
		}
		if root == registry.CURRENT_USER && name == "SteamPath" {
			return `D:\用户 games\Steam`, nil
		}
		if root == registry.LOCAL_MACHINE && name == "InstallPath" {
			if access&registry.WOW64_32KEY != 0 {
				return `E:\Steam 32`, nil
			}
			if access&registry.WOW64_64KEY != 0 {
				return `F:\Steam 64`, nil
			}
		}
		return "", errors.New("unrequested registry view")
	}
	want := []string{`D:\用户 games\Steam`, `E:\Steam 32`, `F:\Steam 64`}
	if got := SteamRoots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("roots=%q; want %q", got, want)
	}
}
func TestWindowsSteamDeniedOrEmptyRegistryUsesOnlyKnownFallbacks(t *testing.T) {
	oldRead, oldEnv := readSteamRegistry, steamEnvironment
	t.Cleanup(func() { readSteamRegistry = oldRead; steamEnvironment = oldEnv })
	readSteamRegistry = func(root registry.Key, path, name string, access uint32) (string, error) {
		if root == registry.CURRENT_USER {
			return "", nil
		}
		return "", registry.ErrNotExist
	}
	steamEnvironment = func(name string) string {
		switch name {
		case "ProgramFiles(x86)":
			return `Q:\游戏 x86`
		case "ProgramFiles":
			return `R:\Apps`
		default:
			t.Fatalf("unbounded fallback lookup: %q", name)
			return ""
		}
	}
	want := []string{`Q:\游戏 x86\Steam`, `R:\Apps\Steam`}
	if got := SteamRoots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("roots=%q; want %q", got, want)
	}
}
