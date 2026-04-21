# ADR-0005 — Pull-based asynchronous Claim Registry with optimistic concurrency

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Platform Engineering
- **Related Requirements:** [FR-07](../requirements/functional.md#fr-07--claim-registry), [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance), [FR-09](../requirements/functional.md#fr-09--pipeline-polling-endpoint), [FR-15](../requirements/functional.md#fr-15--claim-ttl-and-lock-preparation-timeout-from-uc-03), [FR-16](../requirements/functional.md#fr-16--rbac-protected-claim-creation-from-uc-03), [FR-17](../requirements/functional.md#fr-17--per-device-claim-state-in-api-responses-from-uc-03), [NFR-08](../requirements/non-functional.md#nfr-08--bounded-force-release-latency-from-uc-03), [NFR-09](../requirements/non-functional.md#nfr-09--linearizable-claim-locks-from-uc-03)
- **Related Use Cases:** [UC-03](../use-cases/UC-03-cicd-hil-claiming.md)

## Context

CI pipelines need to reserve N physical test rigs matching tags + a desired software version. Pipelines are stateless and may live across CI runners; devices are pull-only (no inbound ports). The orchestration must:

- Allow pipelines to issue a single creation call and then poll.
- Allow idle devices to discover and lock matching claims without any inbound connectivity.
- Guarantee no over-allocation when many devices race for a few slots.
- Force-release slots if devices or pipelines disappear.
- Be RBAC-protected.

## Decision

The control plane exposes an **asynchronous Claim Registry**:

1. **Create:** `POST /v1/claims` (RBAC-protected, [FR-16](../requirements/functional.md#fr-16--rbac-protected-claim-creation-from-uc-03)) returns a `claim_id` and persists the claim with state `Open` plus a TTL.
2. **Offer fan-out:** Registry publishes `ClaimOffer` on `claim.offer.<tag>` for each tag.
3. **Lock (pull):** Idle agents matching the tag(s) issue NATS request-reply on `claim.lock.<claim_id>` with their serial and an `attempt_id`. The Registry uses **optimistic concurrency** in Postgres (e.g., `UPDATE … WHERE slots_remaining > 0 RETURNING …`) to grant exactly N locks; losers receive a deny reply.
4. **Prepare:** Each locked agent fetches the desired-state for the requested version and runs preparation. Each lock has a `preparation_timeout_seconds` lease.
5. **Poll:** Pipelines poll `GET /v1/claims/{id}` and observe per-device sub-states ([FR-17](../requirements/functional.md#fr-17--per-device-claim-state-in-api-responses-from-uc-03)).
6. **Release:** Pipeline `DELETE`s the claim, or claim TTL expires and the Registry force-releases.
7. **Sweeper:** A leader-elected goroutine (or `CronJob` every 10 s) reclaims expired claims and locks within ≤ 30 s of expiry ([NFR-08](../requirements/non-functional.md#nfr-08--bounded-force-release-latency-from-uc-03)).

## Consequences

### Positive

- **Pull-only at the device** — preserves [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance) and [TC-02](../arc42/02-architecture-constraints.md#22-technical-constraints).
- **Linearizable locks** — Postgres `UPDATE … RETURNING` under a single transaction provides linearizability at the row level ([NFR-09](../requirements/non-functional.md#nfr-09--linearizable-claim-locks-from-uc-03)).
- **Pipeline statelessness** — pipelines don't need persistent connections, just a `claim_id` and polling.
- **No coupling between pipeline runtimes and device runtimes** — they communicate exclusively through the Registry.
- **Force-release is bounded** — the sweeper guarantees [NFR-08](../requirements/non-functional.md#nfr-08--bounded-force-release-latency-from-uc-03).

### Negative

- **Polling overhead on the pipeline side** — mitigated by recommended poll cadence (1–5 s) and `Cache-Control` hints; could be replaced with WebSocket push later if pipeline ergonomics demand.
- **Claim fairness defaults to FIFO** at offer time; explicit priority classes are out of scope for v1.
- **TTL/preparation-timeout tuning** is operationally non-trivial; defaults documented but not magic.

### Neutral

- The same Registry could later support priority queues, affinity rules, or geographic constraints; the schema is forward-compatible (`required_tags` is a list, additional dimensions can be added).

## Alternatives Considered

### A. Push reservations (control plane assigns devices)
- **Pros:** Pipeline gets immediate device list.
- **Cons:** Requires the control plane to know which devices are *currently* idle in real time, which is a distributed-systems trap (heartbeat skew, race conditions). Devices that "look idle" might be busy with a local task; we'd over-allocate.
- **Verdict:** Rejected; pull (devices self-report idleness by accepting offers) is the source of truth.

### B. Centralized message queue (e.g., RabbitMQ/Redis Streams) for claim offers
- **Pros:** Could provide work-stealing semantics out of the box.
- **Cons:** Adds another infrastructure component beyond NATS + Postgres; we already have the primitives we need.
- **Verdict:** Rejected.

### C. Long-poll on `GET /v1/claims/{id}`
- **Pros:** Lower latency for the pipeline.
- **Cons:** Holds connections at the API gateway; complicates load balancing; deferred to v1.1 if real measurements justify it.
- **Verdict:** Deferred.

### D. Distributed lock via etcd / consul
- **Pros:** Strong primitives.
- **Cons:** Adds infra; Postgres advisory locks suffice for the scale we're targeting (thousands of devices, hundreds of concurrent claims).
- **Verdict:** Rejected.

## Compliance Notes

- RBAC enforcement on claim creation maps to a documentable control for ISO 13485 change-management traceability ("only authorized pipelines may stage software on devices").
- The Registry's audit trail is part of the cryptographic chain in [§8.7](../arc42/08-crosscutting-concepts.md#87-audit--compliance).
