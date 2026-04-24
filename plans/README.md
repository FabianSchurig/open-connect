# Plans

This directory holds the **execution-level milestone plan** for Open-Connect.

It is the operational counterpart to the strategic documentation in
[`../docs/`](../docs/):

| Layer | Where | Audience |
|-------|-------|----------|
| Use cases, FR/NFR, arc42, ADRs | [`../docs/`](../docs/) | Architects, regulators, auditors |
| Program Increments → Deliverables → Epics → Tasks (Brief mapping) | [`../docs/requirements/traceability-matrix.md`](../docs/requirements/traceability-matrix.md) | Cross-reference / audit |
| **Implementation milestones M0..M5, Epics A..S, with live status** | **this directory** | **Engineers picking up the next session** |

## Contents

- [`milestone-plan.md`](milestone-plan.md) — the detailed milestone plan. Lists
  every implementation epic (A..S), its scope, exit criteria, the requirements
  it satisfies, where the code lives today, and whether it is **Done**,
  **In progress**, or **Open**. The "Next session" section at the bottom names
  the epics to pick up first.

## Conventions

- Epic letters (A, B, C, …) match the table in the repository
  [`README.md`](../README.md) and the commit history. They are stable — never
  renumber.
- Milestones M0..M5 group epics by delivery phase and align with the
  Program Increments PI 1..PI 5 from the project brief (see the mapping table
  in `milestone-plan.md`).
- Status legend:
  - ✅ **Done** — code merged, tests green, exit criteria met.
  - 🟡 **In progress / partial** — scaffolded or in-memory; a follow-up PR is
    required for production readiness (e.g. real adapter, end-to-end wiring).
  - ⏭ **Open** — not started in this codebase yet. Pick these up next.
  - 🚫 **Deferred** — explicitly out of MVP per the descope rules; tracked so
    they don't silently re-appear.

## How to update this plan

When closing or opening work:

1. Edit the relevant epic row in [`milestone-plan.md`](milestone-plan.md):
   flip the status emoji and update the "Evidence / location" column with the
   path(s) to the new code.
2. If a new epic is needed, append it (do not renumber existing ones) and add
   a row to the Brief mapping table.
3. Keep [`../README.md`](../README.md) §"What's in this PR" in sync at the
   high level — that table is the elevator pitch; this plan is the detail.
4. Cross-link any new ADR or arc42 section that the work touches.
