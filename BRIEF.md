# Terminal Home Assistant Dashboard — Project Handoff

## Purpose

This repository is the starting point for planning and eventually implementing a lightweight, SSH-friendly terminal user interface (TUI) for Home Assistant.

The immediate goal is **planning, not implementation**. The project should move through several explicit review stages before substantial development begins.

## Problem Statement

I have an older Raspberry Pi with a dedicated Pi display and enclosure. The display/enclosure are tied closely to that older Pi generation, and replacing the hardware would undermine much of the reason to reuse it.

The Pi is capable of running a terminal comfortably, but running a modern browser and the full Home Assistant web dashboard is cumbersome and slow. Even launching the browser has significant overhead.

The desired architecture is therefore to treat this Pi primarily as a **thin terminal client** rather than as the machine responsible for running the dashboard application.

The Pi should be able to SSH into a substantially more capable server and display a live Home Assistant dashboard entirely inside the terminal. The dashboard application itself can run on the server. SSH transports the terminal output and input while the old Pi primarily renders it.

## Core Concept

The experience should resemble applications such as `btop`, `k9s`, `lazygit`, and other modern TUIs rather than a conventional command-line program that continually appends lines of output.

A full-screen terminal application can use ANSI terminal capabilities to reposition the cursor, update individual regions, use color, respond to keyboard input, and continuously redraw changing information without scrolling the terminal.

Conceptually:

```text
Home Assistant
      │
      │ WebSocket API / REST where appropriate
      ▼
Dashboard application
(runs on capable server)
      │
      │ terminal rendering
      ▼
SSH / persistent terminal session
      │
      ▼
Old Raspberry Pi + display
(thin client)
```

The Raspberry Pi should require very little local processing beyond SSH and terminal rendering.

## Home Assistant Integration

The preferred integration model is event-driven rather than polling-heavy.

The application should investigate using the Home Assistant WebSocket API to:

- obtain initial entity state;
- subscribe to relevant state changes/events;
- maintain a local in-memory representation of dashboard state; and
- redraw only the affected UI elements when practical.

REST may still be appropriate for particular operations, but the design should avoid repeatedly polling the entire Home Assistant state just to refresh the display.

Authentication and credential storage must be considered explicitly during engineering/security review. Do not casually embed a long-lived Home Assistant token in source code or the repository.

## SSH and Session Model

The intended use is approximately:

```bash
ssh dashboard-server
ha-dashboard
```

However, the final UX may be even more appliance-like. Possibilities worth evaluating include:

- automatically launching the dashboard after SSH login;
- a dedicated restricted SSH user;
- attaching automatically to a persistent `tmux` session;
- running the application as a service with clients attaching to it;
- a normal executable that establishes its own Home Assistant connection per invocation; or
- some combination of these approaches.

`tmux` is particularly interesting because the dashboard could survive an SSH disconnect and the Pi could simply reattach after reconnecting.

The architecture review should determine whether tmux is merely operational convenience or an actual dependency.

## Visual Direction

The desired aesthetic is **retro terminal / old computer**, but readability and information density take priority over nostalgia.

This should *not* become a novelty green-on-black terminal that sacrifices usability merely to look old.

Desired characteristics include:

- strong monospace typography;
- box-drawing characters and clearly delineated regions;
- restrained but meaningful use of color;
- excellent readability at a glance;
- compact presentation suitable for a small Pi display;
- bar graphs and progress-style visualizations;
- sparklines where useful;
- obvious warning/error states;
- keyboard navigation where interaction is useful;
- minimal animation or visual noise; and
- graceful behavior at different terminal dimensions.

Color should communicate state rather than merely decorate the screen. For example, normal information can remain relatively neutral while accent colors indicate selection, activity, warnings, failures, unusual sensor readings, or unavailable entities.

Pie charts are not currently a requirement. Terminal-friendly bars, gauges, sparklines, trends, numbers, and status indicators are likely more useful.

## Design References

Five TUI projects were identified as useful references. The design phase should inspect them directly rather than blindly copying their appearance.

### 1. btop

Primary reference for dashboard visualization.

Areas to study:

- live graphs;
- bar visualization;
- compact presentation of many metrics;
- use of color;
- hierarchy within a single screen;
- continuous refresh without making the interface visually unstable.

### 2. k9s

Primary reference for interaction with a large changing state model.

Areas to study:

- panes;
- keyboard navigation;
- status indication;
- drill-down workflows;
- handling large collections of resources;
- making an operational interface understandable without a mouse.

### 3. lazygit

Primary reference for panel organization and restrained interaction design.

Areas to study:

- focus management;
- panel boundaries;
- contextual actions;
- keyboard-driven workflows;
- restrained use of color;
- keeping a complex application visually understandable.

### 4. Glow

Reference for typography and attractive terminal presentation.

Areas to study:

- whitespace;
- typography hierarchy;
- subtle styling;
- readable use of terminal colors.

### 5. gh-dash

Reference for dense dashboard/list presentation.

Areas to study:

- tables and lists;
- filtering;
- compact status presentation;
- dashboard-like organization of changing data.

### Likely Primary References

Unless design review produces a better combination, the strongest initial three appear to be:

1. **btop** — visualization and live metrics;
2. **k9s** — state navigation and operational interaction;
3. **lazygit** — layout, panels, and interaction discipline.

The project should develop its own visual language rather than creating a Home Assistant-themed clone of any of these.

## What the Dashboard Should Show

The exact first dashboard has intentionally **not** been specified yet. That should emerge during product/design planning.

Likely categories include:

- temperatures;
- humidity / environmental sensors;
- doors and windows;
- occupancy or presence;
- lights and switches;
- alarm/security state;
- network or infrastructure state exposed through Home Assistant;
- alerts;
- unavailable entities;
- cameras or NVR status represented textually;
- battery levels;
- energy/power metrics;
- historical trends represented as sparklines or bars.

The project should resist the temptation to reproduce every Lovelace dashboard card. This is a different medium with different strengths.

A good terminal dashboard should prioritize information that benefits from being visible at a glance.

## Interaction

Although the first use case is passive display, the architecture should not unnecessarily prevent future control functionality.

Possible interactions include:

- navigating between dashboard pages;
- opening entity details;
- toggling lights/switches;
- changing scenes;
- acknowledging alerts;
- searching entities;
- filtering by domain/area;
- viewing recent state history;
- manually forcing a reconnect/refresh.

Any state-changing operations should be visually distinguishable from passive navigation and should be designed to avoid accidental activation.

## Responsive Layout

Terminal dimensions may vary substantially between the Pi display and a normal SSH session from a laptop or desktop.

The application should investigate responsive layouts rather than assuming a single fixed character grid.

For example:

```text
Large terminal
┌──────────────┬──────────────┬──────────────┐
│ Environment  │ Security     │ Infrastructure│
│              │              │              │
├──────────────┴──────────────┼──────────────┤
│ Trends                      │ Alerts       │
└─────────────────────────────┴──────────────┘
```

A smaller display might collapse the same information into fewer columns, switch pages, or omit secondary details.

The actual target Pi display resolution and practical terminal dimensions should be captured during requirements gathering.

## Technology Candidates

No framework has been selected yet.

Candidates worth evaluating include at least:

- **Python + Textual**;
- **Go + Bubble Tea**;
- lower-level terminal libraries only if they provide a compelling advantage.

Textual is attractive because of its relatively high-level widget/layout/event model and Python's suitability for Home Assistant integration.

Bubble Tea is attractive for producing a compact standalone binary and for its explicit update/view architecture.

The engineering review should make the selection based on measurable project needs rather than preference alone.

Important criteria include:

- SSH compatibility;
- CPU and memory usage on the server;
- terminal bandwidth during updates;
- redraw efficiency;
- Home Assistant WebSocket support;
- reconnect behavior;
- responsive layout capabilities;
- testing support;
- packaging/deployment simplicity;
- maintainability;
- quality of graph/table/widget primitives;
- Unicode and terminal compatibility.

## Performance Philosophy

The project exists largely because the browser-based approach is too heavy for the target display hardware.

Performance is therefore a product requirement, not an afterthought.

Important considerations include:

- event-driven state updates;
- avoiding unnecessary full-screen redraws;
- avoiding excessive terminal output over SSH;
- maintaining a bounded in-memory state model;
- reconnecting gracefully after network/Home Assistant interruptions;
- avoiding browser/Electron dependencies;
- ensuring the client Pi remains essentially a terminal appliance.

## Reliability

This dashboard may eventually be left running continuously.

It should eventually handle at least:

- SSH interruption;
- Pi reboot;
- server reboot;
- Home Assistant restart;
- WebSocket disconnect;
- temporarily unavailable entities;
- renamed/deleted entities;
- malformed or unexpected state values;
- terminal resize;
- terminal detach/reattach;
- loss and restoration of network connectivity.

Failure should preferably degrade into a useful status message rather than a crashed application or frozen stale dashboard.

## Configuration

The dashboard should eventually be configurable without requiring source-code modifications.

The planning phase should investigate a declarative configuration describing things such as:

- Home Assistant connection;
- dashboard pages;
- panels/widgets;
- entity IDs;
- areas/domains;
- labels;
- thresholds;
- units;
- graph ranges;
- colors/themes;
- keyboard bindings;
- responsive behavior.

Do not prematurely design a configuration schema. First determine the product and UI model that the schema needs to represent.

## Explicit Non-Goals for Initial Planning

Do not assume the first version needs to:

- replace the Home Assistant web UI;
- support every Home Assistant entity or card type;
- render images/video in the terminal;
- recreate Lovelace;
- provide remote administration of Home Assistant;
- implement a plugin ecosystem;
- support every terminal emulator;
- optimize for running the application itself on the old Pi.

These can be revisited if later requirements justify them.

# Planning and Review Process

Development should proceed through deliberate review gates. The purpose is to expose bad assumptions before they become implementation decisions.

## Phase 1 — Initial Planning

Produce an initial project plan from this handoff.

The plan should identify:

- user problem;
- primary use cases;
- assumptions;
- open questions;
- candidate architecture;
- candidate technology stack;
- major risks;
- MVP boundaries;
- potential milestones;
- decisions that require experiments/prototypes.

Do not begin substantial implementation during this phase.

## Phase 2 — CEO / Product Review

Review the plan from a product/executive perspective.

Challenge questions such as:

- What problem are we actually solving?
- Is a custom application justified?
- What is the smallest version that provides meaningful value?
- Which features are distractions?
- What makes this better than simply optimizing a browser dashboard?
- Who would use this besides the original use case?
- Should this be designed as a reusable/open-source tool or a narrowly personal utility?
- What does success look like?

Revise scope accordingly.

## Phase 3 — Technical Project Manager Review

Review execution feasibility.

Produce/refine:

- work breakdown;
- milestones;
- dependencies;
- sequencing;
- acceptance criteria;
- risk register;
- decision log;
- prototype/spike requirements;
- definition of MVP.

Identify requirements that are still ambiguous before engineering begins.

## Phase 4 — Initial Engineering / Architecture Review

Perform the first deep technical review.

Topics should include:

- Home Assistant API model;
- WebSocket subscriptions;
- state cache design;
- application architecture;
- TUI framework evaluation;
- SSH behavior;
- tmux/session persistence;
- configuration model;
- authentication/secrets;
- reconnect strategy;
- terminal compatibility;
- rendering/update strategy;
- logging and diagnostics;
- packaging/deployment;
- testability.

Where architectural decisions depend on uncertain behavior, specify small prototypes rather than guessing.

## Phase 5 — Design Review

After the initial engineering constraints are understood, begin visual design/mockups.

This should occur **before major UI implementation**.

Study the reference TUIs and create several competing dashboard concepts.

Design review should address:

- visual hierarchy;
- retro aesthetic versus readability;
- color semantics;
- panel layout;
- typography;
- graphs and bars;
- entity/status representation;
- alert treatment;
- navigation;
- keyboard interaction;
- responsive behavior;
- target Pi screen dimensions;
- behavior on larger terminals.

ASCII/Unicode mockups are appropriate at this stage and may be preferable to graphical mockups because they expose actual terminal constraints.

Do not settle immediately on the first design.

## Phase 6 — Detailed Engineering Review

Reconcile the approved design with the architecture.

Produce implementation-ready specifications for:

- modules/components;
- data flow;
- state model;
- rendering model;
- widget contracts;
- configuration;
- error handling;
- concurrency/event processing;
- Home Assistant communication;
- tests;
- observability;
- packaging.

## Phase 7 — Adversarial Engineering Review

Have a separate review intentionally try to break the design.

Assume previous reviewers are wrong until demonstrated otherwise.

Look specifically for:

- race conditions;
- stale state;
- reconnect bugs;
- event storms;
- excessive redraw traffic over SSH;
- terminal incompatibilities;
- Unicode width problems;
- resize bugs;
- secrets exposure;
- unsafe service calls;
- assumptions about Home Assistant state types;
- hard-coded entity behavior;
- configuration failure modes;
- memory growth;
- CPU churn;
- tmux/session edge cases;
- brittle framework dependencies.

The goal is not to defend the architecture. The goal is to find reasons it will fail.

## Phase 8 — QA / Test Review

Develop the validation strategy before considering the implementation complete.

Include:

- unit tests;
- integration tests against Home Assistant;
- simulated state changes;
- disconnect/reconnect tests;
- terminal resize tests;
- different terminal dimensions;
- slow/high-latency SSH links;
- unsupported terminal capabilities;
- missing entities;
- invalid configuration;
- Home Assistant restarts;
- server/Pi reboot recovery;
- long-running stability testing;
- interaction safety tests for commands that modify Home Assistant state.

Define measurable acceptance criteria for the MVP.

# Guidance for the AI Planning Process

This repository will be worked on using multiple AI models/providers and human review.

Do not treat previous AI output as authoritative merely because it already exists in the repository.

When reviewing prior work:

1. distinguish requirements from assumptions;
2. identify unsupported claims;
3. challenge architecture choices;
4. verify important technical behavior against primary documentation where practical;
5. preserve a decision log explaining *why* consequential choices were made;
6. surface uncertainty instead of quietly resolving it through guesswork.

The goal of the multi-review process is not consensus between models. It is to expose weaknesses from different perspectives and converge on decisions that survive scrutiny.

# Initial Deliverable

Starting from this document, the next planning agent should **not begin building the application yet**.

Its first deliverable should be a planning package containing:

1. a concise product definition;
2. explicit MVP proposal;
3. requirements and non-requirements;
4. assumptions and open questions;
5. candidate system architecture;
6. TUI framework comparison;
7. Home Assistant integration approach;
8. SSH/session architecture options;
9. major technical risks;
10. proposed prototypes/spikes needed to resolve uncertainty;
11. proposed project/repository structure for the planning phase;
12. recommended sequence for the review stages described above.

Where information is missing, flag it for review rather than inventing a requirement.

The first objective is to make the project **well-defined enough to criticize intelligently**. Implementation comes after that.
