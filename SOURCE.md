# Source tree

Implementation began 2026-08-31, after Phase 8. Decision D12 held a source tree
back until the framework was selected (D29) and the spec had stopped moving;
Phase 6 reached revision 3 and Phase 7 ran twice before this directory existed.

Built against [`docs/planning/phase-6-engineering-r3.md`](docs/planning/phase-6-engineering-r3.md).

## Order of construction

Bottom-up along r3 §1's strictly-downward dependency rule, prioritising the
layers where the adversarial findings live and which are testable without Home
Assistant.

| Package | Status | Closes |
|---|---|---|
| `internal/render` | **done** | width safety as a structural invariant (D39, C1) |
| `internal/state` | **done** — `Value`, `Kind`, `Ring` | A3, A4, E3, E4, D3, D42 |
| `internal/ha` | **done** — protocol, merge, secret | D6, D16, D17, D18, E8 |
| `internal/config` | **done** — grammar, schema, validation | B1, B2, D4, E5, E6, E7, E10 |
| `internal/layout` | **done** — solver, column drop | B1, D36, D37 |
| `internal/widget` | next | design rounds 3–5 |
| `internal/server` | | D7, C1, E1, E9 |
| `cmd/hatty` | | |

## What is already enforced by a test

- Every glyph spike S2 verified measures **exactly one cell** — box drawing,
  blocks, half blocks, arrows, status glyphs, `µ ² ³`, Braille. If
  `go-runewidth`'s ambiguous-width setting ever drifts, this fails first.
- A `Row` is **always** its declared width: over-long content, callers that
  overflow, wide runes, and styling all leave it exact. Styles wrap content
  *after* measurement, so an escape sequence cannot affect width — which closes
  "width safety under composition" structurally rather than by care.
- In `Strict` mode (tests) a short row **panics**, so a layout bug fails in CI
  instead of corrupting a frame on the panel.
- Ring writes **commute**: 200 shuffled orderings of the same live+statistics
  writes converge to one buffer.
- Statistics beat live; a newer statistics revision beats an older one; a full
  reconnect refetch into a full buffer **lands** rather than being discarded.
- Capacity is bounded: 1,000 writes into an 8-bucket ring retain exactly 8.
- A binding with no declared cadence **never** goes stale, even after 30 days.
- Only `Valid` and `Stale` are renderable as content.
- A state-only change **does not drop** an entity's other attributes, and a
  `-` removal **actually removes** — the two silent data-loss modes D18 exists
  to prevent.
- The **real 120-frame capture replays** without loss: 119 change diffs applied,
  no entity emptied, list-valued attributes intact.
- Message ids are strictly increasing and reset per connection (D17), and the
  generator is concurrency-safe.
- The auth frame carries the **real** token — so authentication works — and
  `RedactFrame` scrubs it before logging.
- `Secret.reveal()` has **exactly one call site**, asserted by parsing the
  package's own AST.

- One binding grammar covers **every real shape**: entities, nested
  watchpoint dicts, indexed forecast entries, collections, and projections
  across a collection — and resolves them against the live capture.
- A projection **skips** elements missing a field rather than substituting a
  zero, which is the same failure as rendering `0.0 mi` for absent lightning.
- The width budget is checked **at every breakpoint**, so an overflow at 80
  columns fails even when 44 and 56 are fine.
- An undeclared `{template}` variable, a malformed guard, a wrong ramp arity,
  a missing elastic panel, and an elastic panel that would not survive the
  minimum grid are all load errors — reported **together**, not one at a time.
- Guard referents are **auto-subscribed**, so a guard cannot silently disable
  the value it guards.
- `safety` defaults to true: silence must be opted into.
- Sort hysteresis defaults to **0** — exact ordering — because the concern
  driving it is still unmeasured.
- **Every row of the grid is assigned** at all seven verified breakpoints, with
  no gap and no overlap.
- Reserved panels (alert strip, status bar) survive at **every** size from 12
  rows to 32.
- Panels drop by rank, lowest first; the table outlives the trend and the
  detail pane.
- When the top-ranked elastic panel is dropped, the next one **inherits** the
  leftover rows — the B1 failure, tested directly.
- Columns drop by declared minimum width, and the surviving set **fits** at
  every breakpoint from 44 to 100.

## Three bugs the tests caught immediately

`TestRemovalsActuallyRemove` failed on first run. The diff struct used
`json:"-"` for the removals field — and `encoding/json` reads that tag as
*"never encode this field"*, so removals silently decoded to `nil` and every
attribute deletion was dropped.

No error, no panic; just a dashboard showing values Home Assistant had deleted.
Exactly the class of silent wrongness D18 was written about, and it would have
been invisible without a test that asserts the removal actually happened. The
literal key requires `json:"-,"` with a trailing comma; the field now carries a
comment saying so, because it looks like a typo and someone will try to fix it.

**Second: the E6 guard rule was implemented backwards.** Validation rejected a
guard whose referent nothing else subscribed to — and then fired on the
project's own example dashboard, because a guard referent is *precisely* the
thing nothing else binds. r3 specifies auto-adding referents to the
subscription set; rejecting them makes the feature unusable. Implemented as
`Dashboard.Subscriptions()`, which folds guard referents in, so a guard cannot
disable what it guards.

**Third: the layout solver squeezed the wrong panels.** Panels declare a
`DropRank` ordering removal, but the shrink step chose victims by *largest
slack* — which is rank-blind. At a 12-row grid it produced `table:3 detail:3
trend:4`: the panel that would be dropped first held the most rows, while the
table — the content — sat at its minimum.

Not a crash, and not obviously wrong from the code. It surfaced only because a
test printed the actual row assignment at a tight grid. Shrinking now walks the
same ascending `DropRank` as dropping, so importance decides both.

## Running

```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=arm64 go build ./...   # the deployment target
```
