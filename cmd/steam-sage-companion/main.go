package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/app"
	"github.com/effigy-analytics/steam-sage-companion/internal/bridge"
	"github.com/effigy-analytics/steam-sage-companion/internal/config"
	"github.com/effigy-analytics/steam-sage-companion/internal/install"
	"github.com/effigy-analytics/steam-sage-companion/internal/platform"
	"github.com/effigy-analytics/steam-sage-companion/internal/steam"
)

var version = "0.1.0-alpha.1"

func commandArgs(args []string) (string, []string) {
	if len(args) == 0 {
		return "open", nil
	}
	return args[0], args[1:]
}

func main() {
	command, rest := commandArgs(os.Args[1:])
	if command == "install" || command == "uninstall" {
		flags := flag.NewFlagSet(command, flag.ExitOnError)
		base, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Cannot locate per-user settings")
			os.Exit(1)
		}
		dir := flags.String("config-dir", filepath.Join(base, "steam-sage-companion"), "per-user settings directory")
		purge := flags.Bool("purge", false, "remove owned companion settings when uninstalling")
		flags.Parse(rest)
		paths, err := install.UserPaths(*dir)
		if err == nil {
			if command == "install" {
				var source string
				source, err = os.Executable()
				if err == nil {
					err = install.Install(source, paths)
				}
				if err == nil {
					fmt.Println("Installed Steam Sage Companion. Open it from your application launcher to connect.")
				}
			} else {
				var kept bool
				kept, err = install.Uninstall(paths, *purge)
				if err == nil {
					if kept {
						fmt.Println("Companion stopped. Changed or unowned files were kept.")
					} else {
						fmt.Println("Uninstalled Steam Sage Companion. Steam files and browser choices were kept.")
					}
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if command == "serve" || command == "open" {
		flags := flag.NewFlagSet("serve", flag.ExitOnError)
		base, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Cannot locate per-user settings")
			os.Exit(1)
		}
		dir := flags.String("config-dir", filepath.Join(base, "steam-sage-companion"), "per-user settings directory")
		flags.Bool("background", false, "run without opening controls")
		allowOrigin := flags.String("allow-origin", "", "explicit additional Sage origin (requires separate pairing)")
		flags.Parse(rest)
		var origins []string
		if *allowOrigin != "" {
			u, e := url.Parse(*allowOrigin)
			if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
				fmt.Fprintln(os.Stderr, "Provide an exact HTTPS origin or explicit localhost development origin")
				os.Exit(2)
			}
			origins = []string{*allowOrigin}
		}
		if command == "open" && app.RequestOpen(*dir) == nil {
			return
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := serveWithOpen(ctx, *dir, origins, command == "open"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) != 2 || os.Args[1] != "inspect" {
		fmt.Fprintln(os.Stderr, "Usage: steam-sage-companion open | serve | inspect | install | uninstall [--purge] | version")
		os.Exit(2)
	}
	accounts, err := steam.Discover(platform.SteamRoots())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not discover Steam")
		os.Exit(1)
	}
	type summary struct {
		Collections int    `json:"collections"`
		Supported   int    `json:"supported"`
		Games       int    `json:"games"`
		State       string `json:"state"`
	}
	result := []summary{}
	for _, account := range accounts {
		snapshot, err := steam.Read(account)
		s := summary{State: "ready"}
		if err != nil {
			s.State = err.Error()
		} else {
			s.Collections = len(snapshot.Collections)
			ids := map[uint32]bool{}
			for _, c := range snapshot.Collections {
				if c.Status == "supported" {
					s.Supported++
				}
				for _, id := range c.AppIDs {
					ids[id] = true
				}
			}
			s.Games = len(ids)
		}
		result = append(result, s)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}

func serve(ctx context.Context, dir string, extraOrigins []string) error {
	return serveWithOpen(ctx, dir, extraOrigins, false)
}
func serveWithOpen(parent context.Context, dir string, extraOrigins []string, open bool) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	listener, err := net.Listen("tcp4", bridge.Address)
	if err != nil {
		return errors.New("companion port is already occupied; close the other instance or application")
	}
	defer listener.Close()
	store, err := config.Open(dir)
	if err != nil {
		return err
	}
	application, err := app.New(store, platform.SteamRoots())
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.New("cannot locate companion executable")
	}
	if paths, err := install.UserPaths(dir); err == nil {
		executable = install.StartupExecutable(executable, paths)
	}
	bridgeServer := bridge.New(bridge.Config{Store: store, Version: version, ExtraOrigins: extraOrigins, Accounts: application.Accounts, OnSelect: application.SelectAccount, OnSetRoot: application.AddRoot, OnStop: cancel, OnStartup: func(enabled bool) error {
		if err := platform.SetStartup(enabled, executable, dir); err != nil {
			return err
		}
		return store.Update(func(c *config.Config) error { c.Startup = enabled; return nil })
	}, OnChooseRoot: func() error {
		go func() {
			pickerCtx, stop := context.WithTimeout(ctx, 2*time.Minute)
			defer stop()
			if root, err := platform.ChooseSteamRoot(pickerCtx); err == nil {
				application.AddRoot(root)
			}
		}()
		return nil
	}}, application)
	opener := func() error { return platform.OpenBrowser(bridgeServer.NewControlURL()) }
	activation, err := app.ListenActivation(ctx, dir, opener, cancel)
	if err != nil {
		return err
	}
	if activation != nil {
		defer activation.Close()
	}
	go application.Run(ctx)
	server := http.Server{Handler: bridgeServer, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	if open {
		if err := opener(); err != nil {
			return errors.New("cannot open browser; use the companion shortcut after setting a default browser")
		}
	}
	go func() {
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(stop)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return errors.New("companion local server stopped unexpectedly")
}
