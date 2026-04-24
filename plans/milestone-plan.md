# Open-Connect — Detailed Milestone Plan

> **Snapshot date:** 2026-04-24
> **Source of truth for status:** the code in this repository, cross-checked
> against the [`README.md`](../README.md) epic table and the
> [traceability matrix](../docs/requirements/traceability-matrix.md).

This plan decomposes the *Milestone Plan & Epics — Open-Connect MVP* into
**delivery milestones M0..M5** and **implementation epics A..S**, and records
for each one:

- **Scope** — what the epic covers.
- **Requirements** — the FR/NFR satisfied (per the traceability matrix).
- **Status** — ✅ Done · 🟡 In progress / partial · ⏭ Open · 🚫 Deferred.
- **Evidence / location** — where the code or doc lives today (or `—` if open).
- **Exit criteria** — what "done" means; for partial/open epics this is the
  to-do list for the next session.

Status legend, deferral list, and update workflow are described in
[`README.md`](README.md).

---

## 0. Big-picture status

| Milestone | Theme | Epics | Status |
|-----------|-------|-------|--------|
| **M0** | Repo skeleton, contracts, CI | A, B, C | ✅ Done |
| **M1** | Control-plane MVP (in-memory) + agent skeleton | D, E, F, G, I | ✅ Done (in-memory); 🟡 Postgres + real NATS adapter pending |
| **M2** | Cryptography & rollback safety | J, K | ✅ Done |
| **M3** | Execution engine + reference device profile | L, M, N, O | ✅ Done (server-side `ArtifactURLSigner` for M still pending) |
| **M4** | Claim accept on agent + audit trail | H, P | 🟡 Scaffolding only — **first targets for next session** |
| **M5** | End-to-end, telemetry, release engineering | Q, R, S | ⏭ Open — post-MVP |

Roughly: M0..M3 are done modulo the production adapters; M4 needs to be
finished to call the MVP "feature complete"; M5 is post-MVP hardening and
release.

### Mapping to the project brief

The brief uses *Program Increments (PI 1..PI 5)* and *Epic 1..Epic 4*. The
implementation epics A..S used here are a finer-grained breakdown. The
correspondence is:

| Brief PI / Deliverable | Brief Epic / Tasks | Implementation Epic(s) here |
|------------------------|--------------------|------------------------------|
| PI 1 / D 1.1, D 1.2, D 1.3 | (cross-cutting) | A, B, C, D, E |
| PI 2 / D 2.1 (Rust agent skeleton) | — | G |
| PI 2 / D 2.2 (JWS verification) | Epic 4 / T 4.1 | J |
| PI 2 / D 2.3 (telemetry) | — | (folded into N + R) |
| PI 3 / D 3.1 (engine), D 3.2 (A/B), D 3.3 (boot fallback) | Epic 1, Epic 2 | L, N, O |
| PI 4 / D 4.1 (Claim Registry), D 4.2 (edge claiming), D 4.3 (ROS2 leaf) | Epic 3 | F, H, I, (R) |
| PI 5 / D 5.1 (SELinux), D 5.2 (traceability), D 5.3 (self-update) | Epic 4 / T 4.2, T 4.3 | K, P, plus deferred items |

---

## 1. Milestone M0 — Repo skeleton & contracts (Done)

### Epic A — Repo skeleton & devcontainer · ✅ Done

- **Scope.** Workspace layout, devcontainer, root `Makefile` exposing
  `bootstrap / proto / test / lint / build / fr30-lint`.
- **Requirements.** Enables every later epic; no direct FR.
- **Evidence.** [`Makefile`](../Makefile), [`.devcontainer/`](../.devcontainer/),
  [`Cargo.toml`](../Cargo.toml), [`go.mod`](../go.mod),
  [`README.md`](../README.md).
- **Exit criteria.** ✅ `make bootstrap` passes locally and in CI.

### Epic B — Proto contracts & codegen · ✅ Done (Go) · 🟡 Rust prost

- **Scope.** Single source of truth in `proto/`, buf-generated bindings.
  Implements ADR-0007.
- **Requirements.** FR-01, FR-02, FR-13, FR-14, FR-18, FR-21, FR-23.
- **Evidence.** [`proto/`](../proto/), [`gen/go/`](../gen/go/),
  [`buf.yaml`](../buf.yaml), [`buf.gen.yaml`](../buf.gen.yaml).
- **Exit criteria.**
  - ✅ `buf generate` produces `gen/go` deterministically.
  - ⏭ Rust `prost`/`tonic-build` wiring so the generated types are consumed by
    the agent crates instead of hand-rolled structs. **Open — pick up in next
    session as part of finishing Epic H.**

### Epic C — CI & quality gates · ✅ Done (basic)

- **Scope.** GitHub Actions running `go vet` + `go test -race -cover`,
  `cargo fmt --check` + `cargo clippy -D warnings` + `cargo test`,
  the FR-30 device-profile lint, and an FR-24 grep.
- **Requirements.** NFR-01 (traceability), NFR-02 (Rust safety), FR-24, FR-30.
- **Evidence.** [`.github/workflows/ci.yml`](../.github/workflows/ci.yml),
  [`scripts/lint-device-profile-scripts.sh`](../scripts/lint-device-profile-scripts.sh).
- **Exit criteria.**
  - ✅ All gates run on PRs and main.
  - ⏭ Coverage reporting, SBOM (cargo-deny / govulncheck / cyclonedx), and
    container image build are deferred to **Epic S**.

---

## 2. Milestone M1 — Control plane skeleton & agent (Done in-memory)

### Epic D — Go control-plane skeleton · 🟡 In-memory · Postgres + mTLS pending

- **Scope.** HTTP API gateway, RBAC subject middleware, NATS bus seam,
  in-memory device + claim stores, signing service, error mapper.
- **Requirements.** FR-07, FR-08, FR-09, FR-15, FR-16, FR-17, FR-22, NFR-09,
  NFR-11.
- **Evidence.** [`services/control-plane/`](../services/control-plane/) —
  `cmd/control-plane`, `internal/{api,claims,clock,devices,httperr,nats,rbac,signing}`.
- **Exit criteria.**
  - ✅ End-to-end happy path against the in-memory store covered by Go tests.
  - 🟡 **Open — next session:** swap the in-memory device/claim stores for the
    Postgres adapter, wire mTLS + JWT subject extraction in the middleware
    chain (currently behind `OC_SUBJECT` env), and run the API against a real
    `nats-server` in an integration test.

### Epic E — Device & tagging · 🟡 In-memory

- **Scope.** Device registration, tag CRUD, AND-tag filter for the matcher.
- **Requirements.** FR-22, NFR-11 (registration prerequisite).
- **Evidence.** [`services/control-plane/internal/devices/`](../services/control-plane/internal/devices/),
  schema in [`db/migrations/0001_initial.up.sql`](../db/migrations/0001_initial.up.sql)
  (`devices.tags TEXT[]` + GIN index).
- **Exit criteria.**
  - ✅ In-memory store with tag filter + tests.
  - 🟡 Postgres adapter using the migrated schema; concurrency test that the
    GIN index actually serves AND-tag matching at the expected scale.

### Epic F — Claim Registry server side · 🟡 In-memory

- **Scope.** Claim lifecycle (`Open → PartiallyLocked → Locked → Preparing →
  Ready → InUse → Released/Expired`), TTL expiry, force-release.
- **Requirements.** FR-07, FR-09, FR-15, FR-17, NFR-08, NFR-09.
- **Evidence.** [`services/control-plane/internal/claims/`](../services/control-plane/internal/claims/),
  schema in [`db/migrations/0001_initial.up.sql`](../db/migrations/0001_initial.up.sql)
  (`claims`, `claim_locks`).
- **Exit criteria.**
  - ✅ State-machine + TTL + force-release covered by Go tests.
  - 🟡 Postgres adapter using `SELECT ... FOR UPDATE SKIP LOCKED` for
    NFR-09 linearizable lock acquisition. Schema is in place; the adapter is
    not yet implemented.

### Epic G — Rust agent skeleton · ✅ Done

- **Scope.** State machine (`agent-core`), TOML loader (`agent-config`), NATS
  abstraction with in-memory bus (`agent-nats`), CLI shell (`agent-cli`).
- **Requirements.** NFR-02, NFR-03, NFR-10, NFR-15, FR-18.
- **Evidence.** [`crates/agent-core/`](../crates/agent-core/),
  [`crates/agent-config/`](../crates/agent-config/),
  [`crates/agent-nats/`](../crates/agent-nats/),
  [`crates/agent-cli/`](../crates/agent-cli/).
- **Exit criteria.**
  - ✅ State-machine table tests, config loader test, in-memory bus pub/sub +
    request tests all green.
  - 🚫 No further work in MVP; the real `async-nats` adapter is **Epic H**.

### Epic I — `oc` CLI · ✅ Done

- **Scope.** Operator CLI: `claim create / wait / release / get`, plus
  `oc-sign` for manifest signing during dev.
- **Requirements.** FR-07, FR-09, FR-15, FR-17.
- **Evidence.** [`services/control-plane/cmd/oc/`](../services/control-plane/cmd/oc/),
  [`services/control-plane/cmd/oc-sign/`](../services/control-plane/cmd/oc-sign/).
- **Exit criteria.**
  - ✅ All four subcommands wired, with subject from `OC_SUBJECT`.
  - ⏭ JWT-based auth replacing the env var ships with **Epic D** Postgres/auth
    work.

---

## 3. Milestone M2 — Cryptography & rollback safety (Done)

### Epic J — JWS / Ed25519 / JCS verifier · ✅ Done

- **Scope.** Manifest verification per ADR-0003: JWS compact form, EdDSA, JCS
  canonicalisation, key-id pinning, artifact URL/SHA pinning.
- **Requirements.** FR-10, FR-11 (partial), NFR-01, NFR-11.
- **Evidence.** [`crates/manifest-verifier/`](../crates/manifest-verifier/),
  fixtures in [`testdata/`](../testdata/).
- **Exit criteria.**
  - ✅ 7 negative-test cases (alg confusion, kid mismatch, tampered payload,
    canonicalisation drift, artifact-pin mismatch, …) green.
  - ⏭ Hardware-backed signer (HSM/PKCS#11) deferred — out of MVP.

### Epic K — Anti-rollback gate · ✅ Done

- **Scope.** Per-device monotonic `max_seen_version`, `authorised_downgrade`
  exception, replay protection. Implements ADR-0010 / FR-25 / FR-26 / NFR-16.
- **Requirements.** FR-25, FR-26, NFR-16.
- **Evidence.** [`crates/agent-rollback/`](../crates/agent-rollback/).
- **Exit criteria.**
  - ✅ 5 tests cover replay attack, lower-version reject, authorised
    downgrade, monotonic update, and persistence boundary.
  - 🚫 TPM-backed `RollbackStore` is deferred — file-backed implementation
    sits behind a trait, swap is one-line in MVP+.

---

## 4. Milestone M3 — Execution engine & reference device profile (Done)

### Epic L — Engine + FILE_TRANSFER primitive · ✅ Done

- **Scope.** Config-driven primitive engine per ADR-0008; `FILE_TRANSFER`
  primitive with halt-and-rollback semantics.
- **Requirements.** FR-01, FR-02, FR-21.
- **Evidence.** [`crates/agent-engine/`](../crates/agent-engine/),
  [`crates/agent-primitives/`](../crates/agent-primitives/) (`file_transfer.rs`).
- **Exit criteria.**
  - ✅ Engine tests: happy path, halt-and-rollback on primitive error,
    unknown-primitive rejection.

### Epic M — Artifactory auth in agent · 🟡 Client done, server signer pending

- **Scope.** Agent fetches signed artifact URLs and verifies SHA-256.
- **Requirements.** FR-10 (artifact pinning side), NFR-07 (resumable).
- **Evidence.** Client-side fetch + verify in
  [`crates/agent-primitives/`](../crates/agent-primitives/) `file_transfer.rs`.
- **Exit criteria.**
  - ✅ Client verifies pinned SHA-256 and aborts on mismatch.
  - ⏭ **Open — next session:** server-side `ArtifactURLSigner` in
    `services/control-plane/internal/signing/` that mints the time-bounded
    URLs, plus an end-to-end test with a stubbed object store. NFR-07
    resumable downloads (HTTP `Range` retry loop) is also still open.

### Epic N — SCRIPT_EXECUTION + REBOOT primitives · ✅ Done

- **Scope.** Bounded-output script primitive (1 MiB ringbuf), `OTA_*` env
  enforcement (FR-30), REBOOT primitive.
- **Requirements.** FR-01, FR-02, FR-13 (script flavour), FR-21, FR-30.
- **Evidence.** [`crates/agent-primitives/`](../crates/agent-primitives/)
  (`script_execution.rs`, `reboot.rs`, `ringbuf.rs`).
- **Exit criteria.** ✅ 6 primitive tests green (FILE_TRANSFER ×2,
  SCRIPT_EXECUTION FR-30 env enforcement, REBOOT, ringbuf bound, fail-on-
  nonzero-exit).

### Epic O — `yocto-wic-ab` reference device profile · ✅ Done

- **Scope.** Signed bash scripts implementing the A/B WIC update flow
  (ADR-0004): flash inactive bank, mutate `grubenv`, post-boot health check,
  rollback by restoring `grubenv`.
- **Requirements.** FR-03, FR-04, FR-05, FR-06, FR-30, NFR-05.
- **Evidence.** [`device-profiles/yocto-wic-ab/`](../device-profiles/yocto-wic-ab/)
  (`flash-wic-to-bank.sh`, `update-grubenv.sh`, `restore-grubenv.sh`,
  `post-boot-health-check.sh`), enforced by
  [`scripts/lint-device-profile-scripts.sh`](../scripts/lint-device-profile-scripts.sh).
- **Exit criteria.** ✅ All four scripts present, FR-30 lint green in CI.

---

## 5. Milestone M4 — Agent ↔ control-plane integration & audit (Open)

> **This is the next session's primary scope.**

### Epic H — Claim accept on the agent · 🟡 Scaffold ready, real NATS pending

- **Scope.** Wire the Rust agent to a real `async-nats` connection so it can
  receive `claim.assigned`, accept the lock, run the manifest, and emit
  acknowledgements.
- **Requirements.** FR-08, NFR-04 (network partition recovery), NFR-11.
- **Evidence (today).** Trait + in-memory bus in
  [`crates/agent-nats/`](../crates/agent-nats/); state machine in
  [`crates/agent-core/`](../crates/agent-core/).
- **Exit criteria — open work for next session.**
  1. Add an `async-nats` adapter behind the existing `agent-nats` trait.
  2. Plug the verifier (Epic J) and rollback gate (Epic K) into the accept
     path before the engine (Epic L) runs.
  3. Integration test against an embedded `nats-server` (or `nats-server`
     binary in CI).
  4. NFR-04 partition-recovery test: drop the connection mid-run, prove the
     agent reconnects and resumes the state machine.

### Epic P — DeploymentAck + audit trail · 🟡 Migration only

- **Scope.** `DeploymentAck` proto message, server endpoint, append-only
  `audit_deployments` table, `oc audit` query CLI.
- **Requirements.** FR-11, FR-12, NFR-01, NFR-13.
- **Evidence (today).** `audit_deployments` table is in
  [`db/migrations/0001_initial.up.sql`](../db/migrations/0001_initial.up.sql)
  with belt-and-braces CHECK constraint and the comment that
  `UPDATE`/`DELETE` are revoked at runtime.
- **Exit criteria — open work for next session.**
  1. `DeploymentAck` message in `proto/`, regenerate Go bindings.
  2. Agent emits ack on terminal state (success / halt / rollback) including
     manifest hash, primitive results, exit codes, captured ringbuf head/tail.
  3. Server appends to `audit_deployments`; bootstrap script revokes
     `UPDATE`/`DELETE` from the application role.
  4. `oc audit list / get` subcommand in
     [`services/control-plane/cmd/oc/`](../services/control-plane/cmd/oc/).
  5. Test: any forbidden mutation against `audit_deployments` is rejected.

---

## 6. Milestone M5 — End-to-end, telemetry, release (Open, post-MVP)

### Epic Q — End-to-end QEMU smoke test · ⏭ Open

- **Scope.** Boot a yocto-wic-ab QEMU image, run the agent, drive a manifest
  through control plane → claim → accept → primitives → ack, assert health
  check passes and rollback works on injected failure.
- **Requirements.** FR-03..FR-06, FR-21, NFR-05.
- **Exit criteria.** Reproducible CI job (nightly is fine) that exercises the
  full path on QEMU.

### Epic R — OTLP telemetry · ⏭ Open

- **Scope.** OpenTelemetry traces + metrics from agent and control plane;
  structured logs with correlation IDs across the claim lifecycle.
- **Requirements.** NFR-13, partial NFR-04/NFR-08 observability.
- **Exit criteria.** Spans for `claim.create → assigned → accepted → primitive.* →
  ack` visible in a stock OTLP collector; key SLO metrics exported.

### Epic S — Release engineering · ⏭ Open

- **Scope.** Reproducible builds, container images, SBOM (cargo-deny /
  govulncheck / cyclonedx), signing of release artifacts, version policy.
- **Requirements.** NFR-01, NFR-13, NFR-14 (cloud-agnostic packaging).
- **Exit criteria.** Tagged release produces signed images + SBOM; CI gate
  blocks regressions.

---

## 7. Explicitly deferred (🚫, per MVP descope rules)

These are tracked here so they don't silently re-enter scope.

| Item | Why deferred | Re-entry trigger |
|------|--------------|------------------|
| FR-13 SYSTEM_SERVICE primitive | Not on the WIC critical path | First non-WIC profile |
| FR-19 agent self-update | Need a stable agent first | Post-MVP hardening |
| FR-20 Btrfs subvolume primitive | A/B WIC covers UC-01 today | Btrfs deployment lands |
| FR-23 Docker primitive | Out of MVP scope | UC-02 ROS2 epic |
| FR-27 offline bundle imports | Air-gap is post-MVP | Air-gapped HIL pilot |
| FR-28 `applies_if` predicates | Manifest stays linear in MVP | Multi-profile fleet |
| NFR-12 SELinux strict policy | PI 5 work | Hardening sprint |
| NFR-16 TPM-backed `max_seen_version` | File-backed store behind a trait is sufficient for MVP | Class C medical device pilot |
| Full WORM object-lock audit storage | Append-only Postgres table is the staging ground | Regulator requires immutable storage |

---

## 8. Next session — recommended order of attack

Pick these up first; everything else is either done or post-MVP.

1. **Epic H — async-nats adapter for the agent**
   (unlocks every end-to-end flow). See exit criteria in §5.
2. **Epic D / E / F — Postgres adapters**
   (re-use the existing migration and the in-memory store interfaces).
3. **Epic P — `DeploymentAck` + audit trail**
   (closes the FR-11 / FR-12 traceability loop).
4. **Epic M — `ArtifactURLSigner` server side + resumable downloads**
   (closes NFR-07 and the artifact-pin loop).
5. **Epic B — Rust prost wiring**
   (small, but unblocks anything in 1/3 that wants the generated types).
6. Then start **M5** (Q → R → S) for post-MVP hardening.

When you finish any of the above, flip its status emoji here and update the
README's epic table to match.
