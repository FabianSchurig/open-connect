# Non-Functional Requirements

This catalogue lists the non-functional requirements (NFR) for the platform. IDs **NFR-01 through NFR-04** are baseline from the project brief. IDs **NFR-05+** were derived during the use case walkthroughs.

Each NFR is paired with a measurable **Quality Scenario** in [arc42 §10](../arc42/10-quality-requirements.md).

| Field | Meaning |
|-------|---------|
| **ID** | Stable identifier; never renumber. |
| **Statement** | Imperative quality requirement. |
| **Rationale** | Why this requirement exists. |
| **Source** | Originating UC(s) or project brief reference. |
| **Verification** | One of: Test, Inspection, Analysis, Demonstration, Review. |
| **Priority** | MoSCoW: Must, Should, Could, Won't. |

---

## Compliance & Traceability

### NFR-01 — Traceability for medical compliance

- **Statement:** The backend must automatically generate audit logs linking pipeline deployment requests to the cryptographic acknowledgment of successful modular step execution on the device, satisfying IEC 62304, ISO 13485, and ISO 81001-5-1 standards.
- **Rationale:** Regulatory mandate for medical device software lifecycle documentation.
- **Source:** Brief §3 NFR-01; UC-01 Post-Condition.
- **Verification:** Review (audit log content, completeness, immutability) + Test (link traversal end-to-end).
- **Priority:** Must.
- **Quality Scenario:** see [§10 QS-01](../arc42/10-quality-requirements.md#qs-01-traceability-end-to-end).

---

## Memory & Type Safety

### NFR-02 — Rust for memory safety

- **Statement:** The edge agent must be implemented in Rust to mathematically eliminate memory corruption vulnerabilities at compile time, adhering to secure-by-design principles for medical devices. `unsafe` blocks shall be banned outside reviewed, isolated FFI shims with documented rationale.
- **Rationale:** Eliminates a major class of CVEs from the device-side attack surface.
- **Source:** Brief §3 NFR-02.
- **Verification:** Inspection (CI lint denies `unsafe` outside allowlist) + Review (FFI shims).
- **Priority:** Must.
- **Quality Scenario:** see [§10 QS-02](../arc42/10-quality-requirements.md#qs-02-memory-safety-enforcement).

---

## Footprint & Portability

### NFR-03 — Single statically linked binary, ARM + x86

- **Statement:** The Rust agent must compile to a single, statically linked executable with no dependencies beyond the standard Linux base userspace (bash, GRUB, systemd, coreutils) and shall be cross-compilable to at least `x86_64-unknown-linux-musl` and `aarch64-unknown-linux-musl`.
- **Rationale:** Predictable deployment across heterogeneous device fleets; minimal supply-chain surface.
- **Source:** Brief §3 NFR-03.
- **Verification:** Inspection (`ldd` shows no dynamic deps; CI matrix produces both targets) + Test (binary runs on bare distro).
- **Priority:** Must.
- **Quality Scenario:** see [§10 QS-03](../arc42/10-quality-requirements.md#qs-03-cross-compilation-and-static-linkage).

---

## Network Resilience

### NFR-04 — Polling resilience over NATS

- **Statement:** The polling mechanisms (device→backend, pipeline→backend) over NATS must gracefully handle network partitions and automatically resume state reconciliation when connectivity is restored, without operator intervention.
- **Rationale:** Cellular/Wi-Fi devices and field robots experience routine disconnects; the system must converge.
- **Source:** Brief §3 NFR-04; UC-01 Alt-1; UC-02 Alt-1.
- **Verification:** Test (chaos) — partition NATS for 1 h, 24 h; assert reconciliation upon recovery.
- **Priority:** Must.
- **Quality Scenario:** see [§10 QS-04](../arc42/10-quality-requirements.md#qs-04-network-partition-recovery).

---

## Derived from Use Case Walkthroughs

### NFR-05 — Health-check confirmation window *(from UC-01)*

- **Statement:** The default timeout from a successful boot of the new partition to receipt of the health-check `boot_success=1` confirmation shall be ≤ 120 seconds, configurable per device profile.
- **Rationale:** Bounds rollback latency; prevents indefinite "waiting to commit" states.
- **Source:** UC-01.
- **Verification:** Test — assert agent commits within 120 s on a healthy boot; assert rollback after `boot_count` reboots otherwise.
- **Priority:** Must.

### NFR-06 — Robot autonomy during cloud outage *(from UC-02)*

- **Statement:** Robots running a NATS Leaf Node shall continue to operate locally (ROS2 traffic, local diagnostics) for cloud-connectivity outages of up to 24 hours; outbound telemetry shall be buffered subject to a configured local-disk quota.
- **Rationale:** Field operations cannot pause every time a cellular link drops.
- **Source:** UC-02 Alt-1, Alt-2.
- **Verification:** Test (chaos) — disconnect cloud uplink for 24 h on a robot; assert local ROS2 keeps running and telemetry replays after reconnect within quota.
- **Priority:** Must.

### NFR-07 — Resumable artifact downloads *(from UC-02)*

- **Statement:** The `DownloadArtifact` step shall support resumable downloads (HTTP `Range`) when the artifact server advertises support, to bound bandwidth waste on flaky links.
- **Rationale:** Re-downloading a 2 GB model after a 90 % completion drop is unacceptable on cellular.
- **Source:** UC-02 Alt-1.
- **Verification:** Test — drop connection mid-download, assert resume-from-offset.
- **Priority:** Should.

### NFR-08 — Bounded force-release latency *(from UC-03)*

- **Statement:** Expired claims and locks shall be force-released within ≤ 30 seconds of their TTL/preparation-timeout expiry.
- **Rationale:** Predictable pool availability for fairness across pipelines.
- **Source:** UC-03 Err-3, Alt-2.
- **Verification:** Test — measure delta between TTL expiry and slot re-offer.
- **Priority:** Must.

### NFR-09 — Linearizable claim locks *(from UC-03)*

- **Statement:** Lock acquisition on a claim slot shall be linearizable: under N concurrent lockers competing for `count` slots, exactly `count` shall succeed and the remainder shall observe a deterministic failure.
- **Rationale:** Prevents over-allocation of physical hardware.
- **Source:** UC-03 step 4, Err-4.
- **Verification:** Test (concurrency) — fire N=100 concurrent lockers for `count=10`, assert exactly 10 successes.
- **Priority:** Must.

### NFR-10 — Agent binary size budget *(from NFR-03)*

- **Statement:** The release-mode agent binary shall not exceed **20 MiB** uncompressed for the `aarch64-unknown-linux-musl` target; CI shall fail builds exceeding the budget.
- **Rationale:** Footprint matters on constrained devices and during OTA self-update.
- **Source:** NFR-03 walkthrough; Brief §3.
- **Verification:** Inspection (CI size check).
- **Priority:** Should.

### NFR-11 — All transport authenticated *(from FR-08, FR-10)*

- **Statement:** All NATS connections shall use mutual TLS; all REST API calls to the control plane shall require mTLS or a short-lived bearer token bound to an mTLS-authenticated identity.
- **Rationale:** Zero-trust posture; no plaintext or anonymous access in any environment, including dev.
- **Source:** Brief Deliverable 1.1, 1.3; UC-03 Err-1.
- **Verification:** Inspection (TLS config) + Test (negative — connect without client cert; assert reject).
- **Priority:** Must.

### NFR-12 — SELinux strict confinement *(from Brief Deliverable 5.1)*

- **Statement:** The agent and its spawned step processes shall run under a custom SELinux Type Enforcement policy (`ota_agent.pp`) that enumerates required transitions; downloaded scripts shall be relabeled via `restorecon` so they execute under a confined script type, not `unconfined_t`.
- **Rationale:** Limits blast radius of a compromised manifest or supply-chain attack.
- **Source:** Brief Deliverable 5.1.
- **Verification:** Inspection (`semanage`/`audit2allow` output empty during normal operation) + Test (negative — agent attempts disallowed syscall, denied).
- **Priority:** Must.

### NFR-13 — Append-only audit immutability *(reinforces FR-12)*

- **Statement:** Audit log storage shall be append-only at the storage layer (e.g., WORM-mode object storage, or a database with revoked UPDATE/DELETE privileges for the application service account).
- **Rationale:** Defense in depth — application-level immutability alone is insufficient evidence under audit.
- **Source:** NFR-01; UC-01 Post-Condition.
- **Verification:** Inspection (storage configuration); Test (privileged user attempts modification).
- **Priority:** Must.

### NFR-14 — Cloud-agnostic deployment

- **Statement:** The control plane shall be deployable on any CNCF-conformant Kubernetes cluster without dependency on cloud-vendor proprietary services (no managed databases, no managed identity beyond standard OIDC).
- **Rationale:** Brief mandates cloud-agnostic design; portability is itself a regulatory and operational concern.
- **Source:** Brief PI 1 Goal.
- **Verification:** Inspection (Helm chart contents) + Demonstration (deploy to two distinct K8s providers).
- **Priority:** Must.

### NFR-15 — Distribution-agnostic single binary

- **Statement:** A single, bit-identical agent binary per CPU architecture (initially `aarch64-unknown-linux-musl` and `x86_64-unknown-linux-musl`) shall be deployable on **any** supported Linux distribution (Yocto, Ubuntu 22.04+, Debian 12+, RHEL 9+) without recompilation, reconfiguration at build time, or distribution-specific feature flags. All distribution-specific behaviour shall be expressed via signed device-profile scripts per [FR-24](functional.md#fr-24--config-driven-workflow-hardware-independence).
- **Rationale:** Enables a unified release train, single SBOM, and single signed artifact for audit, across a heterogeneous fleet.
- **Source:** [ADR-0002](../adr/ADR-0002-rust-for-edge-agent.md), [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md); UC-02.
- **Verification:** Demonstration — boot and execute a reference manifest against each supported distribution using the same binary hash; Test — CI pipeline runs integration suite against a matrix of distribution images.
- **Priority:** Must.

### NFR-16 — Tamper-resistant rollback state

- **Statement:** The device persistence of `max_seen_version` (see [FR-25](functional.md#fr-25--anti-rollback-version-monotonicity)) shall use tamper-resistant storage in this preference order:
  1. **TPM 2.0 NV index** with `TPMA_NV_WRITE_STCLEAR` and a policy that binds writes to the agent's measured state (where TPM 2.0 is available).
  2. **ATECC608 / secure element one-way counter** (where TPM is absent but a secure element is available).
  3. **SELinux-confined (`ota_rollback_t`), append-only hash-chained on-disk journal** at `/var/lib/ota-agent/rollback-state/` (fallback; requires documented risk acceptance per device class).

  On TPM-equipped devices, the on-disk journal shall additionally mirror the TPM NV-index value, and a divergence between the two shall raise an integrity alert and block further deployments pending manual recovery.
- **Rationale:** Without tamper resistance, an attacker with local write access could rewind `max_seen_version` and defeat the anti-rollback gate.
- **Source:** [ADR-0010](../adr/ADR-0010-anti-rollback-enforcement.md); ISO 81001-5-1.
- **Verification:** Test (security) — with SELinux enforcing, a user-level process MUST NOT be able to modify `/var/lib/ota-agent/rollback-state/`. On a TPM-equipped reference device, verify the NV index is set and policy-locked. Manually tamper with the on-disk journal on a TPM device; assert the agent detects divergence and refuses further applies.
- **Priority:** Must (TPM or secure-element path for medical-device class per UC-01); Should (journal fallback) for HIL/lab devices per UC-03.

---

## Cross-Reference

See the [traceability matrix](traceability-matrix.md) for the full UC ↔ NFR ↔ arc42 ↔ Epic mapping.
