# Open-Connect

Open-source over-the-air (OTA) update and device-fleet manager.

This repository delivers the **MVP foundation** described in the *Milestone Plan
& Epics — Open-Connect MVP* document. The split — Go control plane / NATS
fabric / Rust edge agent — is the one in *arc42 §5*.

> **Status:** MVP scaffolding. M0 (repo + contracts) and the core of M1–M3
> (control plane, agent crates, JWS verifier, anti-rollback, primitive engine,
> reference device profile) are in place and fully tested. Real `nats-server`
> + Postgres adapters and the QEMU end-to-end (Epic Q) are the next PRs.

## Repository layout

```
proto/                       # Protobuf source of truth (ADR-0007); buf-generated
gen/go/                      # Generated Go bindings
crates/                      # Rust workspace
  agent-core/                # State machine (arc42 §5.7.1)
  agent-engine/              # Config-driven execution engine (§5.4.3)
  agent-primitives/          # FILE_TRANSFER, SCRIPT_EXECUTION, REBOOT (FR-01/02/21/30)
  agent-rollback/            # Anti-rollback gate (FR-25/26, ADR-0010)
  agent-nats/                # NATS abstraction (in-memory bus + trait)
  agent-config/              # TOML config loader
  agent-cli/                 # `open-connect-agent` binary
  manifest-verifier/         # JWS / EdDSA / JCS verifier (§5.8, ADR-0003)
services/control-plane/      # Go API gateway, claim registry, oc / oc-sign CLIs
db/migrations/               # golang-migrate SQL migrations (Epic D / P)
device-profiles/yocto-wic-ab # Reference signed bash scripts (Epic O)
testdata/                    # Manifest fixtures + Ed25519 test keys
docs/{arc42,adr,…}           # Existing architecture docs
.github/workflows/ci.yml     # CI gates (Go, Rust, FR-30 lint, FR-24 grep)
```

## Quickstart

```bash
make bootstrap        # verify toolchains
make test             # run all hermetic tests (Go + Rust)
make lint             # vet + clippy + FR-30 + fmt
make build            # build all binaries

# Run the control plane (in-memory store + NATS — single-process demo).
go run ./services/control-plane/cmd/control-plane &

# Drive the API.
export OC_SUBJECT=dev-admin
go run ./services/control-plane/cmd/oc claim create \
    --tags yocto-wic-ab --count 1 --version 2.4.0 --ttl 60s
```

## What's in this PR (vs. the milestone plan)

| Epic | Status | Notes |
|------|--------|-------|
| A — Repo skeleton & devcontainer    | ✅ | devcontainer + Makefile |
| B — Proto contracts & codegen       | ✅ Go side | Rust prost wiring follows |
| C — CI & quality gates              | ✅ basic | Go vet+test, Rust fmt+clippy+test, FR-24, FR-30 |
| D — Go control plane skeleton       | ✅ scaffolding | mTLS+JWT middleware + Postgres adapter follow |
| E — Device & tagging                | ✅ in-memory | Postgres adapter follow |
| F — Claim Registry server side      | ✅ in-memory | Postgres `FOR UPDATE SKIP LOCKED` adapter follow |
| G — Rust agent skeleton             | ✅ | state machine + config + NATS abstraction |
| H — Claim accept on the agent       | ⏭ scaffold ready | needs real async-nats adapter |
| I — `oc` CLI                        | ✅ | `claim create/wait/release/get` |
| J — JWS / Ed25519 / JCS verifier    | ✅ | exhaustive negative test corpus |
| K — Anti-rollback gate              | ✅ | replay + lower-limit + authorised-downgrade tests |
| L — Engine + FILE_TRANSFER          | ✅ | halt-and-rollback tested |
| M — Artifactory auth in agent       | ✅ | server-side `ArtifactURLSigner` follows |
| N — SCRIPT_EXECUTION + REBOOT       | ✅ | 1 MiB ringbuf, OTA_* env enforcement |
| O — `yocto-wic-ab` scripts          | ✅ | + FR-30 lint |
| P — DeploymentAck + audit           | ⏭ migration in place; service follows |
| Q — End-to-end QEMU                 | ⏭ post-MVP |
| R — OTLP telemetry                  | ⏭ post-MVP |
| S — Release engineering             | ⏭ post-MVP |

## Test summary

```
Go:   9 packages green (RBAC, devices, claims, api, signing, …)
Rust: 28 tests across 8 crates green
      manifest-verifier  7   (alg / kid / tamper / canonicalisation / artifacts pin)
      agent-rollback     5   (replay attack, authorised downgrade, monotonic max_seen)
      agent-primitives   6   (FILE_TRANSFER, SCRIPT_EXECUTION FR-30, REBOOT, ringbuf)
      agent-engine       3   (happy / halt-and-rollback / unknown primitive)
      agent-core         4   (state-machine table)
      agent-nats         2   (pub-sub + request)
      agent-config       1
```

## Explicitly deferred (with rationale)

These are intentionally out of MVP per the *MVP descope rules* in the plan,
and tracked here so they don't get silently re-introduced:

- **FR-13** SYSTEM_SERVICE primitive — not on the WIC critical path.
- **FR-19** agent self-update.
- **FR-20** Btrfs subvolume primitive.
- **FR-23** Docker primitive.
- **FR-27** offline bundle imports.
- **FR-28** `applies_if` predicates.
- **NFR-12** SELinux strict policy (PI 5).
- **NFR-16** TPM-backed `max_seen_version` — file-backed `RollbackStore` is in
  place behind a trait, swap is one-line in MVP+.
- Full WORM object-lock audit storage — append-only Postgres table is the
  staging ground per the plan (Epic P).
