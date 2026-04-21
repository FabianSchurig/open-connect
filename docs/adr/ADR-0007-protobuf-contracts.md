# ADR-0007 — Use Protobuf as the canonical wire contract language

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group
- **Related Requirements:** all FRs that involve cross-process communication; specifically [FR-01](../requirements/functional.md#fr-01--modular-manifest-execution), [FR-11](../requirements/functional.md#fr-11--cryptographic-deployment-acknowledgment-from-uc-01), [FR-18](../requirements/functional.md#fr-18--telemetry-stream-contract-from-uc-01-uc-02), [OC-02](../arc42/02-architecture-constraints.md#23-organizational-constraints)
- **Related Use Cases:** all

## Context

Two languages (Go control plane, Rust edge agent) must agree on the shape of every NATS message — desired states, telemetry, step results, claim messages, deployment acks. The contract must:

- Be schema-driven (we cannot afford "shape drift" between server and edge in a regulated environment).
- Provide compact wire encoding (telemetry is published frequently, often over cellular).
- Have first-class support in both Go and Rust.
- Support evolution without breaking deployed older agents.

## Decision

Use **Protocol Buffers (proto3)** as the canonical schema and wire format for all NATS messages and all internal Go service interfaces where typing is valuable. Generate Go and Rust bindings from the same `.proto` source of truth, kept in a shared schema package.

REST API request/response bodies remain JSON (operator ergonomics, curl-debuggability), but the JSON shapes are derived from the same Protobuf messages where applicable.

JWS manifest envelopes carry **Protobuf-encoded payloads** (`DesiredState`); the JSON content type is reserved for human-facing API surfaces.

## Consequences

### Positive

- **Compact, fast, typed** — Protobuf wins on every dimension that matters at the device-server boundary.
- **Forward/backward compatibility** by construction (proto3 default-presence semantics; `reserved` for retired fields).
- **One source of truth** for message shapes; the schema lives in version control and reviewed PRs.
- **Tooling is mature** in both Go (`google.golang.org/protobuf`) and Rust (`prost`, `tonic`, `quick-protobuf`).

### Negative

- **Schema discipline required**: never reuse field numbers, never change types in place; reviewers must enforce. Mitigated by `buf lint` + `buf breaking` in CI.
- **Less self-describing on the wire** — Protobuf bytes need a schema to decode; this is fine for our boundaries but makes ad-hoc debugging harder. Mitigated by a small `otactl` CLI that decodes recorded messages against the live schema.
- **JWS payloads are not human-readable**; auditors get JSON-rendered views via the audit-export bundle.

### Neutral

- We may adopt **gRPC** for internal control-plane service-to-service calls in the implementation phase, since we already have Protobuf; not committed in this ADR.

## Alternatives Considered

### A. JSON everywhere
- **Pros:** Self-describing; trivial debug.
- **Cons:** No schema enforcement out of the box; verbose on the wire; no native typed bindings; we'd reinvent JSON Schema validation; ambiguous numeric types.
- **Verdict:** Rejected for the device-server boundary; JSON remains for human-facing REST.

### B. CBOR
- **Pros:** Binary, compact, widely supported.
- **Cons:** Schema story is weaker than Protobuf; less mature codegen in Go/Rust ecosystems; no `buf`-equivalent for breaking-change detection.
- **Verdict:** Rejected.

### C. FlatBuffers / Cap'n Proto
- **Pros:** Zero-copy reads; very fast.
- **Cons:** Smaller ecosystem; pay-for-what-you-don't-need at our message sizes; weaker schema-evolution guarantees; team unfamiliarity.
- **Verdict:** Rejected.

### D. Apache Avro
- **Pros:** Strong schema evolution; popular in data engineering.
- **Cons:** Schema fingerprint negotiation is its own complexity; less natural fit for low-volume per-message edge traffic.
- **Verdict:** Rejected.

## Compliance Notes

- Schema versioning policy is part of the configuration-management plan for the regulated product. The `DesiredState.schema_version` field provides an explicit break-glass for documented major-version migrations.
- All schema changes require code review by both the control-plane and the edge-agent maintainers (CODEOWNERS).
