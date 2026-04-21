# UC-03 — CI/CD Pipeline Hardware-in-the-Loop (HIL) Device Claiming

| Field | Value |
|-------|-------|
| **ID** | UC-03 |
| **Title** | CI/CD Pipeline HIL Device Claiming |
| **Primary Actor** | CI/CD Pipeline (automated client) |
| **Secondary Actors** | Free Edge Nodes (idle HIL rigs), Go Control Plane (Claim Registry), QA Engineer (consumes test results) |
| **Scope** | Asynchronous reservation of N physical test rigs matching tags + software version |
| **Level** | System-goal |
| **Status** | Baseline (from project brief) |

---

## 1. Brief

A CI pipeline needs `X` physical test rigs running software version `Y` to execute integration tests. The reservation is **asynchronous** and **pull-based**: idle rigs poll the registry, lock matching claims, prepare themselves, and signal readiness. The pipeline polls for `Ready` status, runs its tests, then releases the claim.

## 2. Stakeholders & Interests

| Stakeholder | Interest |
|-------------|----------|
| CI/CD Pipeline | Deterministic access to hardware in finite time; no inbound network coupling. |
| QA Engineer | Tests run on a known software/hardware combination. |
| Operator | Free pool is shared fairly across pipelines without manual scheduling. |
| Security Officer | Only authorized pipelines may create claims (RBAC). |

## 3. Preconditions

- Pipeline holds a service-account credential authorized to call the Claim API. *(Task 4.3 RBAC)*
- A pool of HIL rigs is registered with hardware tags (e.g., `ros2-hil`, `x86`).
- Each rig's agent is in the `Idle` state and authenticated to NATS via mTLS.
- Software version `Y` artifact and signed manifest are available.

## 4. Trigger

CI runner sends `POST /v1/claims` with body:

```json
{
  "count": 5,
  "tags": ["ros2-hil", "x86"],
  "desired_version": "v2.4",
  "ttl_seconds": 3600,
  "preparation_timeout_seconds": 900
}
```

## 5. Main Success Flow

1. Control Plane validates the request (RBAC, tag existence, quota), assigns `claim_id`, and stores the claim in state `Open`. *(FR-07)*
2. Control Plane fans out an offer on `claim.offer.<tag>` for each tag in the claim.
3. Each idle, matching agent receives the offer via its NATS subscription. *(FR-08)*
4. Agents race to lock the claim via `claim.lock.<claim_id>` request-reply; the registry uses optimistic concurrency to grant locks atomically (first-N-win).
5. The first `count` successful lockers transition their local state to `Preparing`; losers stay `Idle`.
6. Each locked agent fetches the desired-state for version `Y`, verifies the JWS signature, and executes the modular flow to install `Y`. *(FR-10, FR-01)*
7. After successful preparation and self-test, agent transitions to `Ready` and publishes status.
8. Control Plane updates the claim's per-device state map.
9. Pipeline polls `GET /v1/claims/{claim_id}` periodically. *(FR-09)*
10. When `count` devices are `Ready`, the response payload includes their connection info; pipeline executes its tests.
11. Pipeline calls `DELETE /v1/claims/{claim_id}` to release; agents transition `In-Use → Released → Idle`.

**Post-Condition (success):** Devices return to `Idle`. Audit log links pipeline identity → claim_id → device serials → installed version → test outcome.

## 6. Alternative & Error Flows

### Alt-1 — Insufficient devices available
- 1a. Control Plane accepts the claim; it remains `Open` partially fulfilled.
- 5a. As more rigs become idle, they pick up remaining offers until `count` is reached or `ttl_seconds` expires.

### Alt-2 — Agent loses NATS connectivity after locking but before `Ready`
- 6a. Claim Registry tracks heartbeat freshness per locked device.
- 6b. After `preparation_timeout_seconds` without progress, registry releases the lock and re-offers the slot. *(FR-15 new)*
- 6c. The disconnected rig, on reconnect, sees its lock revoked, aborts its preparation, and returns to `Idle`.

### Err-1 — Pipeline lacks RBAC permission
- 1b. Control Plane returns `403 Forbidden`; no claim created. *(Task 4.3)*

### Err-2 — Preparation step fails on a locked device
- 6d. Agent reports `Failed`; registry releases the slot and re-offers.
- 6e. Pipeline observes `Ready` count not increasing; either waits (Alt-1) or accepts the claim's `ttl` to expire.

### Err-3 — Pipeline never releases the claim
- 11a. After `ttl_seconds` (or pipeline-side disconnect heartbeat loss), registry force-releases the claim and returns devices to the pool. *(NFR-08 new)*

### Err-4 — Two pipelines race for the last available rig
- 4a. Optimistic-concurrency lock guarantees exactly one winner; the loser sees the slot disappear and continues waiting under its own claim.

## 7. Derived Requirements

- **Functional:** FR-07, FR-08, FR-09, FR-10
- **Non-Functional:** NFR-01, NFR-04
- **New requirements raised by walkthrough:**
  - FR-15 *(new)* — Claim Registry must implement per-claim TTL and per-lock preparation timeout, automatically reclaiming expired slots.
  - FR-16 *(new)* — Claim creation API must be RBAC-protected; only principals with role `pipeline:create-claim` may invoke it.
  - FR-17 *(new)* — Claim API responses must include per-device state (`Pending|Preparing|Ready|Failed`) and last-known timestamp.
  - NFR-08 *(new)* — Force-release on TTL expiry must occur within ≤ 30 s of expiry; no orphaned reservations.
  - NFR-09 *(new)* — Lock acquisition shall be linearizable: under N concurrent lockers, exactly `count` succeed.

## 8. Related Architecture

- Sequence diagram: [arc42 §06 — UC-03 sequence](../arc42/06-runtime-view.md#uc-03--cicd-hil-device-claiming)
- Component: Claim Registry inside the Go Control Plane — see [arc42 §05](../arc42/05-building-block-view.md#go-control-plane-l2)
- ADRs: [ADR-0001 NATS](../adr/ADR-0001-nats-over-http-rest.md), [ADR-0005 Pull-based claims](../adr/ADR-0005-pull-based-claim-model.md)

## 9. Open Issues

- Fairness across competing pipelines (FIFO vs. priority class) — deferred; v1 uses FIFO at offer time.
- Multi-region claim affinity — out of scope for v1.
