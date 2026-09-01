// Command hatty is the dashboard daemon.
//
// It owns the Home Assistant connection and the state, and serves a terminal
// dashboard over SSH. Clients attach a renderer; the state stays warm, so
// reattaching is instant (§8 option C, D7).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/ha"
	"github.com/ifnull/hatty/internal/server"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

const tokenEnv = "HATTY_HA_TOKEN"

func main() {
	var (
		dashPath = flag.String("dashboard", "", "dashboard TOML (required)")
		haURL    = flag.String("ha", "ws://ha.home.arpa:8123/api/websocket", "Home Assistant WebSocket URL")
		addr     = flag.String("addr", "0.0.0.0:2222", "SSH listen address")
		authKeys = flag.String("authorized-keys", "authorized_keys", "SSH allow-list")
		hostKey  = flag.String("host-key", "hatty_host_key", "SSH host key")
		tickMS   = flag.Int("tick", 250, "render coalescing interval, ms")
		debug    = flag.Bool("debug", false, "verbose logging")
	)
	flag.Parse()

	// Logs go to STDERR, never to stdout: stdout is a rendered terminal for
	// every session, and one stray line corrupts the frame (r3 §11).
	lvl := slog.LevelInfo
	if *debug {
		lvl = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))

	if *dashPath == "" {
		fatal(log, "--dashboard is required")
	}
	dash, err := config.Load(*dashPath)
	if err != nil {
		// Configuration errors are fatal at startup. A dashboard that silently
		// drops a misconfigured panel is worse than one that refuses to start,
		// because the missing panel looks like an absent condition.
		fatal(log, fmt.Sprintf("dashboard %s:\n%v", *dashPath, err))
	}

	// D15: the token comes from the environment, never a CLI argument, which
	// would place it in ps output and shell history.
	token := ha.Secret(os.Getenv(tokenEnv))
	if token.Empty() {
		fatal(log, tokenEnv+" is not set")
	}

	keys, err := server.LoadAuthorizedKeys(*authKeys)
	if err != nil {
		fatal(log, err.Error()) // C1: fail closed
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// E9: SIGHUP reloads the allow-list, so a key can be added or revoked
	// without dropping every live session.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			if err := keys.Reload(); err != nil {
				log.Warn("authorized_keys reload failed; keeping the previous set", "err", err)
				continue
			}
			log.Info("authorized_keys reloaded", "keys", keys.Len())
		}
	}()

	store := state.NewStore()
	go store.Run(ctx, time.Duration(*tickMS)*time.Millisecond)

	subs := dash.Subscriptions() // includes guard referents (finding E6)
	log.Info("hatty starting",
		"dashboard", dash.Name, "entities", len(subs),
		"target", fmt.Sprintf("%dx%d", dash.Display.Cols, dash.Display.Rows))

	go ha.Run(ctx, *haURL, token, subs, store, func() {
		log.Info("ha: subscribed", "entities", len(subs))
	}, log)

	th := widget.Default()
	if dash.Display.Glyphs != "" {
		th.Glyphs = dash.Display.Glyphs
	}

	err = server.Serve(ctx, server.Options{
		Addr: *addr, HostKeyPath: *hostKey, AuthKeys: keys,
		Dash: dash, Theme: th, Store: store, Log: log,
	})
	if err != nil {
		fatal(log, err.Error())
	}
	log.Info("hatty stopped")
}

func fatal(log *slog.Logger, msg string) {
	log.Error(msg)
	os.Exit(1)
}
