// S8 -- does Wish serve a Bubble Tea app over SSH with correct resize?
//
// This is a measurement harness, not application code. It answers the
// questions that gate SS8 option C (a daemon owning HA state with an SSH
// front-end) and risk R7 (resize / detach / reattach):
//
//  1. Does Wish serve a Bubble Tea program over SSH at all, with no login
//     shell involved?
//  2. Does the app see the client's real terminal size on connect?
//  3. Does SIGWINCH propagate -- does the app observe a live resize?
//  4. Does reconnecting at a DIFFERENT grid work? This is the one that
//     matters. D5 puts font-per-dashboard in the hatty-connect launcher,
//     so a dashboard switch changes the console font and the app gets a
//     different grid on reattach than it last drew.
//  5. Do multiple simultaneous clients at different sizes each render
//     correctly? (Option C claims this falls out naturally.)
//
// The view deliberately includes a column ruler. Do not trust the reported
// numbers alone -- if the ruler does not end flush with the right edge of
// the terminal, the app's idea of the width is wrong, which is exactly the
// class of bug R3 is about.
//
// Run:  go mod tidy && go run .   then connect with:  ssh -p 2222 localhost
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host = "0.0.0.0"
	port = "2222"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	okStyle    = lipgloss.NewStyle().Bold(true)
	warnStyle  = lipgloss.NewStyle().Bold(true)
)

type resizeEvent struct {
	at   time.Time
	w, h int
}

type model struct {
	term      string
	connected time.Time
	width     int
	height    int
	resizes   []resizeEvent
	frames    int
	now       time.Time
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Record every size the app is told about, including the first.
		if m.width != msg.Width || m.height != msg.Height {
			m.resizes = append(m.resizes, resizeEvent{time.Now(), msg.Width, msg.Height})
		}
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		m.frames++
		return m, tick()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "c":
			m.resizes = nil
			return m, nil
		}
	}
	return m, nil
}

// ruler draws a width-calibration line: every 10th column is marked, and the
// line is exactly m.width cells wide. If it wraps or falls short, the app's
// width is not the terminal's width.
func (m model) ruler() string {
	if m.width <= 0 {
		return ""
	}
	var b strings.Builder
	for i := 1; i <= m.width; i++ {
		switch {
		case i%10 == 0:
			b.WriteString(fmt.Sprintf("%d", (i/10)%10))
		case i%5 == 0:
			b.WriteString("+")
		default:
			b.WriteString("·")
		}
	}
	return b.String()
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("hatty S8 — Wish + Bubble Tea over SSH"))
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "  grid          %d cols × %d rows\n", m.width, m.height)
	fmt.Fprintf(&b, "  TERM          %s\n", m.term)
	fmt.Fprintf(&b, "  connected     %s ago\n", time.Since(m.connected).Truncate(time.Second))
	fmt.Fprintf(&b, "  ticks         %d %s\n", m.frames,
		dimStyle.Render("(proves the render loop is live, not a one-shot paint)"))
	b.WriteString("\n")

	// The 100x30 target from decision D1.
	if m.width >= 100 && m.height >= 30 {
		b.WriteString("  " + okStyle.Render("✓ meets the 100×30 design target") + "\n")
	} else {
		b.WriteString("  " + warnStyle.Render(
			fmt.Sprintf("✗ below the 100×30 target (have %d×%d)", m.width, m.height)) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("  " + dimStyle.Render("size history — reconnect at a different font to test R7") + "\n")
	if len(m.resizes) == 0 {
		b.WriteString("  " + dimStyle.Render("(none yet)") + "\n")
	}
	start := 0
	if len(m.resizes) > 8 {
		start = len(m.resizes) - 8
	}
	for i, r := range m.resizes[start:] {
		label := "resize"
		if start+i == 0 {
			label = "initial"
		}
		fmt.Fprintf(&b, "    %s  %-7s %d×%d\n", r.at.Format("15:04:05"), label, r.w, r.h)
	}
	b.WriteString("\n")

	b.WriteString("  " + dimStyle.Render("column ruler — must end flush with the right edge") + "\n  ")
	b.WriteString(m.ruler())
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render("  q quit · c clear history · resize the window to generate events"))
	return b.String()
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, active := s.Pty()
	if !active {
		wish.Fatalln(s, "no pty requested — allocate one (ssh -t)")
		return nil, nil
	}
	m := model{
		term:      pty.Term,
		connected: time.Now(),
		width:     pty.Window.Width,
		height:    pty.Window.Height,
		now:       time.Now(),
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

func main() {
	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		// Generated on first run. Spike-only: do not reuse this key anywhere.
		wish.WithHostKeyPath(".ssh/hatty_s8_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(), // refuse non-interactive sessions
			logging.Middleware(),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create server: %v\n", err)
		os.Exit(1)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("S8 harness listening on %s:%s\n", host, port)
	fmt.Printf("connect with:  ssh -p %s localhost\n\n", port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			done <- syscall.SIGTERM
		}
	}()

	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	fmt.Println("stopped")
}
