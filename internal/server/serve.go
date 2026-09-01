package server

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

const shutdownGrace = 15 * time.Second

// Options configures the SSH front end.
type Options struct {
	Addr        string
	HostKeyPath string
	AuthKeys    *AuthorizedKeys
	Dash        *config.Dashboard
	Theme       *widget.Theme
	Store       *state.Store
	Log         *slog.Logger
}

// Serve runs the SSH server until ctx is cancelled.
//
// This is §8 option C: the daemon owns the Home Assistant connection and the
// state, and each SSH connection attaches a RENDERER to it. A disconnect drops
// a renderer, not the state, so reattaching re-renders from a warm cache with
// no Home Assistant round-trip -- the ten-second browser launch (D24)
// eliminated by architecture rather than by optimisation.
func Serve(ctx context.Context, o Options) error {
	srv, err := wish.NewServer(
		wish.WithAddress(o.Addr),
		wish.WithHostKeyPath(o.HostKeyPath),
		// C1: wish's default accepts ANY key. Without this handler the daemon
		// would serve home-relative aircraft bearings to anyone on the LAN.
		wish.WithPublicKeyAuth(func(sctx ssh.Context, key ssh.PublicKey) bool {
			ok := o.AuthKeys.Contains(key)
			if !ok {
				o.Log.Warn("ssh: rejected key", "from", sctx.RemoteAddr())
			}
			return ok
		}),
		wish.WithMiddleware(
			// MiddlewareWithProgramHandler hands us the *tea.Program for THIS
			// session, which is what the frame sink needs. Sharing one program
			// across sessions -- the obvious shortcut -- would deliver every
			// client's frames to whichever connected last.
			bm.MiddlewareWithProgramHandler(func(s ssh.Session) *tea.Program {
				return newProgram(s, o)
			}, termProfile),
			activeterm.Middleware(),
		),
	)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	o.Log.Info("ssh: listening", "addr", o.Addr, "dashboard", o.Dash.Name)
	if err := srv.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
		return err
	}
	return nil
}

func newProgram(s ssh.Session, o Options) *tea.Program {
	pty, _, _ := s.Pty()
	o.Log.Info("session: open", "from", s.RemoteAddr(), "term", pty.Term,
		"cols", pty.Window.Width, "rows", pty.Window.Height)

	m := Session{Dash: o.Dash, Theme: o.Theme, snap: o.Store.Snapshot()}
	p := tea.NewProgram(m,
		tea.WithInput(s), tea.WithOutput(s), tea.WithAltScreen(),
		tea.WithContext(s.Context()),
	)

	// Frames reach the program through a Sink, so a wedged client can never
	// block the store (A1/E1). Verified on hardware: a stalled client blocks
	// the write for seconds while Push stays at ~300 microseconds.
	go func() {
		sink := NewSink(s.Context(), func(ctx context.Context, f Frame) error {
			p.Send(f)
			return nil
		})
		defer sink.Close()

		unsub := o.Store.Subscribe(func(snap *state.Snapshot) {
			sink.Push(FrameMsg{Snap: snap})
		})
		defer unsub()

		<-s.Context().Done()
		sent, dropped := sink.Stats()
		o.Log.Info("session: closed", "from", s.RemoteAddr(),
			"frames", sent, "dropped", dropped)
	}()
	return p
}
