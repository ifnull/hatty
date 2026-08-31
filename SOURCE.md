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
| `internal/config` | next | B2, D4, E5, E6, E10 |
| `internal/layout` | | B1, D36, D37 |
| `internal/widget` | | design rounds 3–5 |
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

## A bug the tests caught immediately

`TestRemovalsActuallyRemove` failed on first run. The diff struct used
`json:"-"` for the removals field — and `encoding/json` reads that tag as
*"never encode this field"*, so removals silently decoded to `nil` and every
attribute deletion was dropped.

No error, no panic; just a dashboard showing values Home Assistant had deleted.
Exactly the class of silent wrongness D18 was written about, and it would have
been invisible without a test that asserts the removal actually happened. The
literal key requires `json:"-,"` with a trailing comma; the field now carries a
comment saying so, because it looks like a typo and someone will try to fix it.

## Running

```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=arm64 go build ./...   # the deployment target
```
