# ha-terminality

A full-screen terminal dashboard for Home Assistant, designed to be viewed over SSH on
hardware too weak to run a browser.

The application runs on a capable server, holds a live event-driven mirror of Home Assistant
state, and renders into any terminal that connects to it. The client — a Raspberry Pi 3B with
a 7" panel — does nothing but transport keystrokes and paint cells.

> **Status: planning. No implementation.**
> Currently at **Phase 1 complete, awaiting Phase 2 (product review)**.
> There is deliberately no source tree; see decision D12.

## Start here

| Document | What it is |
|---|---|
| [`BRIEF.md`](BRIEF.md) | The original handoff. Never edited. The authority on intent. |
| [`docs/planning/phase-1-plan.md`](docs/planning/phase-1-plan.md) | The Phase 1 planning package — product definition, MVP, architecture, framework comparison, risks, spikes. |
| [`docs/environment.md`](docs/environment.md) | Measured facts: hardware, display, terminal, HA instance. Method and date recorded. |
| [`docs/decisions/decision-log.md`](docs/decisions/decision-log.md) | Every consequential choice, why, and what would overturn it. |
| [`docs/reference/`](docs/reference/) | HA screenshots. The 23-51-49 capture is the source of truth for the primary dashboard. |

## Working conventions

**Phases append, they do not overwrite.** Each review produces its own document. Earlier
reasoning stays on disk to be argued with.

**Prior output is not authoritative.** The brief is explicit: this repository is worked on by
multiple AI models and a human reviewer. Distinguish requirements from assumptions, challenge
architecture choices, and verify technical claims against primary documentation. Claims marked
**[VERIFY]** in the plan have *not* been checked against primary sources and should not be
built on until they are.

**Home Assistant protocol claims are verified.** Plan §7 was checked against the developer
documentation and the `home-assistant/core` source on 2026-08-27 and carries inline citations.
The remaining **[VERIFY]** markers are in §6 and §8 and concern framework/library behaviour —
they resolve through spikes S4 and S8, not through reading documentation.

**Spikes record raw results, not just conclusions.** A spike whose numbers are gone cannot be
re-examined when a later phase disputes what was concluded from them.

**Surface uncertainty rather than resolving it by guesswork.** Phase 1 §12 ends with an
explicit list of what it is least confident about. Later phases should maintain the equivalent.

## Immediate next steps

1. **Prior-art survey** — do adequate Home Assistant TUIs already exist? A Phase 2 entry
   condition, and the most likely way this project turns out to be unnecessary (open question O5).
2. **Spikes S1–S3** — ADS-B event rate, console glyph inventory, and a `subscribe_entities`
   vs. `state_changed` bandwidth comparison. Cheap, measurement-only, and they make the
   product review factual.
3. **Phase 2 — product review.** The plan names what it should attack hardest.

## Not yet done

This is not a git repository. `git init` would give the multi-reviewer process a history to
read; left to the user rather than done unasked.
