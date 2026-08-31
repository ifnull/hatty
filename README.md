# hatty

*Home Assistant + TTY.*

A full-screen terminal dashboard for Home Assistant, designed to be viewed over SSH on
hardware too weak to run a browser.

The application runs on a capable server, holds a live event-driven mirror of Home Assistant
state, and renders into any terminal that connects to it. The client — a Raspberry Pi 3B with
a 7" panel — does nothing but transport keystrokes and paint cells.

> **Status: planning. No implementation.**
> **Phase 5 complete; Phase 6 at revision 2; Phase 7 at round 2.** Round 1 found 13 problems
> and all were fixed; round 2 found 10 more, **four of them defects introduced by round 1's own
> repairs**. A revision 3 is required before implementation. No source tree yet; see D12.

## Start here

| Document | What it is |
|---|---|
| [`BRIEF.md`](BRIEF.md) | The original handoff. Never edited. The authority on intent. |
| [`docs/planning/phase-1-plan.md`](docs/planning/phase-1-plan.md) | The Phase 1 planning package — product definition, MVP, architecture, framework comparison, risks, spikes. |
| [`docs/planning/phase-6-engineering-r2.md`](docs/planning/phase-6-engineering-r2.md) | **Current spec.** Closes all 13 Phase 7 findings; adds the decision-oriented `home` screen. |
| [`docs/planning/weatherflow-data-audit.md`](docs/planning/weatherflow-data-audit.md) | HA vs. the vendor app. Lightning is genuinely broken; two mockup bindings were wrong. |
| [`docs/planning/phase-7-adversarial-r2.md`](docs/planning/phase-7-adversarial-r2.md) | Round 2. Ten findings — four are defects introduced by round 1's own fixes. |
| [`docs/planning/phase-7-adversarial.md`](docs/planning/phase-7-adversarial.md) | 13 findings against the Phase 6 spec. Four would ship broken; three are invariants that fail at runtime; two are security holes. |
| [`docs/planning/phase-6-engineering.md`](docs/planning/phase-6-engineering.md) | Implementation-ready spec: modules, concurrency, state, config schema, widget contracts, tests. |
| [`docs/design/`](docs/design/) | Phase 5. Five rounds; `README-v3` (tables), `README-v4` (colour), `README-v5` (responsive) are current. |
| [`docs/planning/prior-art-survey.md`](docs/planning/prior-art-survey.md) | Does an adequate HA TUI already exist? (No.) Phase 2 entry condition. |
| [`docs/planning/dashboard-source-analysis.md`](docs/planning/dashboard-source-analysis.md) | The primary dashboard's Lovelace YAML read as a requirements document. |
| [`docs/planning/airspace-entity-model.md`](docs/planning/airspace-entity-model.md) | The ha-airspace entity/attribute contract the Radar panel binds against. |
| [`docs/spikes/`](docs/spikes/) | Measurement harnesses for the open questions. S1, S4, S8 ready to run. |
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

1. **Phase 6 revision 3** against round-2 findings E1–E10. E3 (statistics corrections
   discarded) and E8 (token on the protocol-trace path) first.
2. **Phase 8** — QA and test strategy.
3. Implementation, once a round of adversarial review finds nothing new.
3. **Phase 2 — product review.** The plan names what it should attack hardest.

## Not yet done

This is not a git repository. `git init` would give the multi-reviewer process a history to
read; left to the user rather than done unasked.
