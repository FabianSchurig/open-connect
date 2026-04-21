# UC-01 — Modular A/B OTA Update for Medical Devices

| Field | Value |
|-------|-------|
| **ID** | UC-01 |
| **Title** | Modular A/B OTA Update for Medical Devices |
| **Primary Actor** | Release Manager (human, via Go control plane UI/API) |
| **Secondary Actors** | Rust Edge Agent, NATS Fabric, GRUB Bootloader, QA / Regulatory Affairs (consumer of audit log) |
| **Scope** | End-to-end OS update of a fielded ext4-based medical device |
| **Level** | User-goal |
| **Status** | Baseline (from project brief) |

---

## 1. Brief

A critical patch must be deployed to a fleet of ext4-based medical devices. The update must be **un-brickable** (automatic rollback on failure), **cryptographically verified** before execution, and produce **audit evidence** that satisfies IEC 62304 / ISO 13485 traceability.

## 2. Stakeholders & Interests

| Stakeholder | Interest |
|-------------|----------|
| Release Manager | Push the patch quickly with confidence it cannot brick devices. |
| QA / Regulatory Affairs | Receive a tamper-evident record linking patch → device → outcome. |
| Patient / Clinician (indirect) | Device remains operational; failures roll back invisibly. |
| Security Officer | Only signed manifests execute; no inbound device ports. |

## 3. Preconditions

- Device is registered in the control plane and tagged `production`.
- Device runs ext4 A/B layout with active partition Bank A.
- Device holds the public Ed25519 key used to verify update manifests.
- NATS connectivity to the cloud is available (or will return — see UC-01 Alt-1).
- A signed update artifact (`.img`) is uploaded to artifact storage and reachable.

## 4. Trigger

Release Manager submits a new `DesiredState` for the target tag through the Go Control Plane, referencing a modular update flow consisting of:

1. `DownloadArtifact` (pre-signed URL → checksum verified)
2. `RunScript` (pre-install hook)
3. `FlashPartition` (write `.img` to inactive bank)
4. `UpdateGrubEnv` (set `boot_part=B`, `boot_count=0`, `boot_success=0`)
5. `Reboot`

## 5. Main Success Flow

1. Release Manager creates the modular flow and signs the manifest envelope (JWS, EdDSA).
2. Control Plane stores the `DesiredState` keyed by device tag and emits availability on `device.<id>.desired-state`.
3. Edge Agent's polling loop fetches the new `DesiredState` via NATS request-reply.
4. Edge Agent **verifies the JWS signature** against the on-device public key. *(FR-10)*
5. Agent executes steps sequentially, capturing stdout/stderr per step and publishing each `StepResult` to `device.<id>.step-result`. *(FR-01)*
6. Agent flashes the `.img` exclusively to Bank B (inactive) and verifies filesystem integrity. *(FR-04)*
7. Agent updates `grubenv`: `boot_part=B`, `boot_count=3`, `boot_success=0`. *(FR-05)*
8. Agent issues controlled reboot.
9. GRUB chainloads Bank B; on each boot, `boot_count` is decremented.
10. Once systemd reaches the health-check target, agent sets `boot_success=1` and `boot_count=0`. *(FR-06, supports rollback path)*
11. Agent publishes a final `StepResult{success=true}` plus a cryptographic acknowledgment of the deployed software version on `audit.pipeline.<deployment_id>`. *(NFR-01)*
12. Control Plane records the acknowledgment in the audit log linked to the original `DesiredState` and Release Manager identity. *(NFR-01)*

**Post-Condition (success):** Device is running new image from Bank B; audit log contains the full chain `request → signed manifest → step results → cryptographic ack`.

## 6. Alternative & Error Flows

### Alt-1 — Network partition during polling
- 4a. Agent has no NATS connectivity. It continues to retry with jittered backoff and queues telemetry locally.
- 4b. When connectivity returns, the agent reconciles state and resumes from the last known step. *(NFR-04)*

### Err-1 — Signature verification fails
- 4c. Agent rejects the manifest, publishes a `SecurityEvent`, does **not** execute any step, remains on Bank A.
- **Post-Condition:** Audit log records the rejection and key fingerprint mismatch.

### Err-2 — Step returns non-zero exit code
- 5a. Agent halts the sequence immediately. *(FR-02)*
- 5b. Agent triggers rollback: clears `grubenv` toggle (reverts `boot_part=A`), discards Bank B contents.
- 5c. Agent publishes `StepResult{success=false, step_index=N, exit_code=E, stderr=…}`.
- **Post-Condition:** Device unchanged; audit log records the failed step.

### Err-3 — New image fails to boot or fails health check
- 9a. `boot_count` reaches 0 without `boot_success=1`. GRUB falls back to Bank A on next boot. *(FR-06)*
- 9b. Agent on Bank A reports the failed transition; control plane marks deployment as `Reverted`.
- **Post-Condition:** Device is running pre-update Bank A; audit log records automatic revert.

### Err-4 — Artifact checksum mismatch
- 5d. `DownloadArtifact` step fails on checksum compare; treated as Err-2.

## 7. Derived Requirements

- **Functional:** FR-01, FR-02, FR-03, FR-04, FR-05, FR-06, FR-10
- **Non-Functional:** NFR-01 (Traceability), NFR-02 (Memory Safety), NFR-04 (Network Resilience)
- **New requirements raised by walkthrough:**
  - FR-11 *(new)* — Agent must publish a cryptographic acknowledgment per deployment containing `(device_serial, manifest_hash, deployed_version, timestamp, signature)`.
  - FR-12 *(new)* — Control plane must persist audit records in append-only storage with deployment_id ↔ device_serial ↔ manifest_hash ↔ outcome.
  - NFR-05 *(new)* — Time from successful Bank B boot to `boot_success=1` health confirmation must be ≤ 120 s by default (configurable per profile).

## 8. Related Architecture

- Sequence diagram: [arc42 §06 — UC-01 sequence](../arc42/06-runtime-view.md#uc-01--ab-ota-medical-update)
- Components touched: Manifest Verifier, Execution Engine, Partition Manager, GRUB Manager, Telemetry, Audit Service, RBAC.
- ADRs: [ADR-0003 JWS/Ed25519](../adr/ADR-0003-jws-ed25519-manifests.md), [ADR-0004 A/B + grubenv](../adr/ADR-0004-ab-partitioning-grubenv.md), [ADR-0007 Protobuf](../adr/ADR-0007-protobuf-contracts.md)

## 9. Open Issues

- Key rotation procedure (carried into ADR-0003 follow-up).
- Rollback policy when *both* banks are in a degraded state (out of scope for v1).
