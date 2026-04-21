# Requirements

This catalogue captures the contractual functional and non-functional requirements that the platform must satisfy. Every requirement is back-referenced from at least one [use case](../use-cases/) and forward-referenced into [arc42 §05](../arc42/05-building-block-view.md), [§08](../arc42/08-crosscutting-concepts.md), and [§10](../arc42/10-quality-requirements.md).

## Index

- [Functional Requirements](functional.md) — `FR-01` through `FR-22` (10 baseline + 12 derived).
- [Non-Functional Requirements](non-functional.md) — `NFR-01` through `NFR-14` (4 baseline + 10 derived).
- [Traceability Matrix](traceability-matrix.md) — UC ↔ FR/NFR ↔ arc42 ↔ Epic/Task ↔ ADR.

## Authoring Rules

- **IDs are stable.** Do not renumber requirements when reordering or deleting. Mark deprecated ones with status `Deprecated` rather than removing them.
- **One sentence statements.** Use "shall" / "must" language. Avoid "should" except for genuine SHOULD-priority items.
- **Verifiable.** Every requirement must specify a verification method that a test or audit can execute.
- **Sourced.** Every requirement cites the originating UC or brief reference.
