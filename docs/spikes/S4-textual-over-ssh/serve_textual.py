#!/usr/bin/env python3
"""
S4 -- can a Textual app be served over a programmatically owned SSH
connection, in-process, without a login shell?

This is the go/no-go that decides whether Python + Textual can implement
SS8 option C (a daemon owning HA state with an SSH front-end). Go and
Bubble Tea have a documented path via Wish (see spike S8); Textual has no
documented equivalent, which is the entire reason this spike exists.

A NEGATIVE RESULT IS A USEFUL RESULT. If Textual cannot be served this
way it is effectively limited to SS8 options A and B, and the Phase 4
framework decision becomes much simpler. Do not sink days into this --
timebox it (suggested: 4 hours) and record what failed.

Two tests, deliberately ordered:

  TEST A -- single client, pty pair plus dup2.
      Binds one Textual app to a pty whose master end is pumped to the
      SSH channel. Because it redirects the process's own fds 0/1 it
      serves ONE client at a time. If this fails, Textual over SSH is
      dead and Test B is moot.

  TEST B -- multiple clients, custom Textual Driver.
      The real option C: several renderers in one process, each bound to
      its own channel, sharing state. Sketched at the bottom of this
      file, not implemented -- implementing it is the substance of the
      spike.

Run:
    pip install asyncssh textual
    ssh-keygen -t ed25519 -f ./spike_host_key -N ''
    ./serve_textual.py                       # then: ssh -p 2223 localhost
"""

import argparse
import asyncio
import fcntl
import os
import signal
import struct
import sys
import termios

try:
    import asyncssh
except ImportError:
    sys.exit("missing dependency: pip install asyncssh")

try:
    from textual.app import App, ComposeResult
    from textual.widgets import Footer, Header, Static
except ImportError:
    sys.exit("missing dependency: pip install textual")


HOST_KEY = "./spike_host_key"
PORT = 2223


class ProbeApp(App):
    """Mirrors the S8 Bubble Tea harness so the two can be compared like for like."""

    CSS = "Screen { background: $surface; } #body { padding: 1 2; }"
    BINDINGS = [("q", "quit", "quit"), ("c", "clear", "clear history")]

    def __init__(self):
        super().__init__()
        self.resizes = []

    def compose(self) -> ComposeResult:
        yield Header()
        yield Static(id="body")
        yield Footer()

    def on_mount(self) -> None:
        self.set_interval(1.0, self.refresh_body)
        self.refresh_body()

    def action_clear(self) -> None:
        self.resizes.clear()
        self.refresh_body()

    def on_resize(self, event) -> None:
        self.resizes.append((event.size.width, event.size.height))
        self.refresh_body()

    def refresh_body(self) -> None:
        w, h = self.size.width, self.size.height
        ruler = "".join(
            str((i // 10) % 10) if i % 10 == 0 else ("+" if i % 5 == 0 else "·")
            for i in range(1, max(w, 1) + 1)
        )
        target = ("meets the 100x30 design target" if w >= 100 and h >= 30
                  else f"BELOW the 100x30 target (have {w}x{h})")
        history = "\n".join(f"    {a}x{b}" for a, b in self.resizes[-8:]) or "    (none yet)"
        try:
            self.query_one("#body", Static).update(
                f"hatty S4 -- Textual over SSH\n\n"
                f"  grid          {w} cols x {h} rows\n"
                f"  {target}\n\n"
                f"  size history\n{history}\n\n"
                f"  column ruler -- must end flush with the right edge\n  {ruler}\n"
            )
        except Exception:
            pass


def set_winsize(fd: int, cols: int, rows: int) -> None:
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


async def handle_session(process) -> None:
    term = process.get_terminal_type()
    size = process.get_terminal_size()
    cols, rows = (size[0], size[1]) if size else (80, 24)
    print(f"[S4] session: TERM={term} size={cols}x{rows}", file=sys.stderr)

    if not term:
        process.stderr.write("no pty requested -- connect with: ssh -t\r\n")
        process.exit(1)
        return

    master_fd, slave_fd = os.openpty()
    set_winsize(slave_fd, cols, rows)

    saved_in, saved_out = os.dup(0), os.dup(1)
    saved_term = os.environ.get("TERM")
    os.environ["TERM"] = term

    loop = asyncio.get_running_loop()
    master = os.fdopen(master_fd, "r+b", buffering=0)
    stop = asyncio.Event()

    try:
        # Redirect this process's stdio at the pty slave. THIS IS WHY TEST A
        # IS SINGLE-CLIENT ONLY -- fds 0 and 1 are process-global.
        os.dup2(slave_fd, 0)
        os.dup2(slave_fd, 1)

        def on_master_readable():
            try:
                data = master.read(65536)
            except OSError:
                stop.set()
                return
            if not data:
                stop.set()
                return
            try:
                process.stdout.write(data.decode("utf-8", "replace"))
            except Exception:
                stop.set()

        loop.add_reader(master_fd, on_master_readable)

        async def feed_input():
            try:
                async for chunk in process.stdin:
                    master.write(chunk.encode() if isinstance(chunk, str) else chunk)
            except Exception:
                pass
            stop.set()

        async def watch_resize():
            last = (cols, rows)
            while not stop.is_set():
                await asyncio.sleep(0.25)
                cur = process.get_terminal_size()
                if cur and (cur[0], cur[1]) != last:
                    last = (cur[0], cur[1])
                    set_winsize(slave_fd, last[0], last[1])
                    try:
                        os.kill(os.getpid(), signal.SIGWINCH)
                    except OSError:
                        pass

        app = ProbeApp()
        tasks = [
            asyncio.create_task(feed_input()),
            asyncio.create_task(watch_resize()),
            asyncio.create_task(app.run_async()),
        ]
        _, pending = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for t in pending:
            t.cancel()

    finally:
        try:
            loop.remove_reader(master_fd)
        except Exception:
            pass
        os.dup2(saved_in, 0)
        os.dup2(saved_out, 1)
        os.close(saved_in)
        os.close(saved_out)
        if saved_term is None:
            os.environ.pop("TERM", None)
        else:
            os.environ["TERM"] = saved_term
        for closer in (master.close, lambda: os.close(slave_fd)):
            try:
                closer()
            except Exception:
                pass
        process.exit(0)


class SpikeServer(asyncssh.SSHServer):
    """Spike only: no authentication. Never expose this port."""

    def begin_auth(self, username: str) -> bool:
        return False  # False == no auth required

    def connection_made(self, conn):
        print(f"[S4] connection from {conn.get_extra_info('peername')}", file=sys.stderr)


async def main_async(port: int) -> None:
    if not os.path.exists(HOST_KEY):
        sys.exit(f"missing host key. Run:\n  ssh-keygen -t ed25519 -f {HOST_KEY} -N ''")

    await asyncssh.create_server(
        SpikeServer, "", port,
        server_host_keys=[HOST_KEY],
        process_factory=handle_session,
    )
    print(f"[S4] listening on port {port}")
    print(f"[S4] connect with:  ssh -p {port} localhost")
    print("[S4] TEST A is single-client by construction -- connect one at a time.")
    await asyncio.Future()


def main() -> None:
    ap = argparse.ArgumentParser(description="S4 -- Textual over SSH go/no-go")
    ap.add_argument("--port", type=int, default=PORT)
    args = ap.parse_args()
    try:
        asyncio.run(main_async(args.port))
    except (KeyboardInterrupt, SystemExit):
        pass


if __name__ == "__main__":
    main()


# ---------------------------------------------------------------------------
# TEST B -- the real option C. NOT IMPLEMENTED; implementing it is the work.
#
# Test A proves Textual can render over SSH, but its dup2 makes it
# single-client, which is not option C. Option C needs N renderers in one
# process sharing one HA state cache.
#
# The extension point is Textual's Driver. Subclass it so that instead of
# touching the process's stdout it writes into a specific asyncssh channel:
#
#     from textual.driver import Driver
#
#     class SSHDriver(Driver):
#         def __init__(self, app, *, debug=False, size=None, channel=None):
#             super().__init__(app, debug=debug, size=size)
#             self._channel = channel
#
#         def write(self, data: str) -> None: ...        # -> channel
#         def flush(self) -> None: ...
#         def start_application_mode(self) -> None: ...  # enter alt screen
#         def stop_application_mode(self) -> None: ...
#         def disable_input(self) -> None: ...
#
#     App(driver_class=SSHDriver)   # then feed channel input into the app
#
# VERIFY the Driver signature against the installed Textual version before
# assuming the sketch is current -- it is written from working knowledge,
# not from the docs, and Textual's API moves quickly (risk R10).
#
# GO/NO-GO, to be recorded in RESULTS.md:
#   Test A renders, resizes, exits cleanly       -> Textual viable for A/B
#   Test B serves 2+ clients at different sizes  -> Textual viable for C
#   Test B not working inside the timebox        -> Textual limited to A/B;
#                                                   Phase 4 should prefer
#                                                   Go + Wish
# ---------------------------------------------------------------------------
