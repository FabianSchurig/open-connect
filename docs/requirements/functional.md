# Functional Requirements

This catalogue lists every functional requirement (FR) for the platform. IDs **FR-01 through FR-10** are baseline from the project brief. IDs **FR-11+** were derived during the use case walkthroughs in [`../use-cases/`](../use-cases/).

| Field | Meaning |
|-------|---------|
| **ID** | Stable identifier; never renumber. |
| **Statement** | Imperative requirement statement. |
| **Rationale** | Why this requirement exists. |
| **Source** | Originating UC(s) or project brief reference. |
| **Verification** | One of: Test, Inspection, Analysis, Demonstration, Review. |
| **Priority** | MoSCoW: Must, Should, Could, Won't. |

---

## Modular Execution Engine

### FR-01 — Modular manifest execution via generic primitives

- **Statement:** The edge agent shall parse and execute modular update manifests consisting of discrete, configurable steps drawn from a fixed primitive set: `SCRIPT_EXECUTION`, `FILE_TRANSFER`, `SYSTEM_SERVICE`, `DOCKER_CONTAINER`, `AGENT_SELF_UPDATE`, `REBOOT`. The agent shall **not** embed OS-, bootloader-, or filesystem-specific logic; such intelligence is delivered as signed scripts inside `SCRIPT_EXECUTION` steps.
- **Rationale:** Decouples *what* changes from *how* it is delivered; enables both OS- and application-level updates; enables hardware portability per [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md) and [FR-24](#fr-24--config-driven-workflow-hardware-independence).
- **Source:** Brief §2 Modular Execution Engine; UC-01, UC-02; [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md).
- **Verification:** Test (integration) — execute a manifest containing each primitive type and assert order + outcome; Inspection — confirm Rust source contains no hardcoded GRUB/ext4/Btrfs calls outside of (optional) signed reference scripts.
- **Priority:** Must.

### FR-02 — Halt-and-rollback on step failure

- **Statement:** The agent shall halt execution and trigger the rollback procedure if any individual step in the modular sequence returns a non-zero exit code.
- **Rationale:** Atomic-flow semantics; prevents partially applied updates.
- **Source:** Brief §2; UC-01 Err-2; UC-02 Err-1.
- **Verification:** Test (fault injection) — inject failures at each step index and assert rollback.
- **Priority:** Must.

---

## A/B Partitioning & GRUB Management

### FR-03 — Dual-banking ext4 root

- **Statement:** The system shall support ext4 filesystems utilising a dual-banking (A/B partition) architecture for the root filesystem.
- **Rationale:** Enables atomic OS swap and instant rollback.
- **Source:** Brief §2; UC-01.
- **Verification:** Inspection (partition layout) + Test (boot from each bank).
- **Priority:** Must.

### FR-04 — Inactive-bank writes only (profile-parameterized)

- **Statement:** For OS-level updates, the workflow (as expressed in the signed manifest's scripts) shall write the payload exclusively to the **inactive bank** — where "bank" is the active/inactive unit of the selected device profile: an ext4 partition (`ext4-partition-ab`), a physical disk (`dual-disk-chainload`), or a Btrfs subvolume (`btrfs-snapshot`). See [§5.9](../arc42/05-building-block-view.md#59-boot-redundancy-reference-device-profiles). The active bank shall not be modified during update. Policy enforcement is provided by (a) a **target-safety interlock** in the device-profile script per [FR-30](#fr-30--device-profile-script-authoring-conventions), and (b) SELinux (`ota_partition_writer_t` rule) such that deviations from this invariant fail safely at the kernel.
- **Rationale:** Guarantees the system can always boot the previously known-good image. Profile-parameterized wording accommodates the real-world mix of partition-A/B, dual-disk, and Btrfs-snapshot deployments.
- **Source:** Brief §2; UC-01 step 6; field-tested prototype (`deploy_rootfs.sh`, dual-disk case). *(Implemented by reference device-profile scripts — see [FR-01](#fr-01--modular-manifest-execution-via-generic-primitives), [FR-24](#fr-24--config-driven-workflow-hardware-independence), [FR-30](#fr-30--device-profile-script-authoring-conventions), and [ADR-0006](../adr/ADR-0006-selinux-strict-policy.md).)*
- **Verification:** Test — for each catalogued profile, assert the device-profile script's safety interlock rejects writes to the active bank; assert SELinux policy denies the write at kernel level if the interlock is bypassed.
- **Priority:** Must.

### FR-05 — `grubenv` boot toggling (boot-counter and one-shot variants)

- **Statement:** For GRUB-based device profiles, the update workflow shall manage boot sequences by modifying the GRUB environment block (`grubenv`). Two patterns are supported and chosen per device profile per [ADR-0004](../adr/ADR-0004-ab-partitioning-grubenv.md):
  - **Boot-counter pattern** (default for `ext4-partition-ab` and `btrfs-snapshot`): manipulate `boot_part`, `boot_count`, `boot_success`, `previous_part`; GRUB script decrements the counter and auto-reverts on exhaustion.
  - **One-shot pattern** (default for `dual-disk-chainload`): use `grub-reboot <menuentry>` to set `next_entry` for exactly one boot; on success the agent promotes to `saved_entry` via `grub-set-default`; on failure the next boot already reverts automatically.

  Both patterns are performed by signed device-profile scripts invoked via the `SCRIPT_EXECUTION` primitive.
- **Rationale:** Covers the two grubenv usage patterns observed in real deployments. Keeping both in scripts (rather than compiled into the agent) allows adopting systemd-boot, U-Boot, UKI, or alternative GRUB strategies without an agent release.
- **Source:** Brief §2; UC-01 step 7; field-tested prototype (GRUB chainload script). *(Implemented by reference `update-grubenv.sh` / `restore-grubenv.sh` / `grub-reboot-chainload.sh` device-profile scripts — see [ADR-0004](../adr/ADR-0004-ab-partitioning-grubenv.md), [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md).)*
- **Verification:** Test — for each pattern, read-back `grubenv` after agent action and assert variable values; sabotage health and assert correct revert semantics (multi-attempt for the counter; single-attempt for the one-shot).
- **Priority:** Must.

### FR-06 — Automated bootloader fallback

- **Statement:** The bootloader configuration shall incorporate an automated fallback mechanism that reverts `boot_part` to the previous functioning partition if the new partition fails to reach the post-boot health-check target within the boot-count window.
- **Rationale:** Un-brickability — failed boots must self-heal without operator intervention.
- **Source:** Brief §2; UC-01 Err-3.
- **Verification:** Test — sabotage health-check, assert fallback after `boot_count` reboots.
- **Priority:** Must.

---

## Device Claiming & Pipeline Orchestration

### FR-07 — Claim Registry

- **Statement:** The Go backend shall provide a Claim Registry where CI/CD pipelines can request a specified quantity of devices matching specific tags and software versions.
- **Rationale:** Enables automated HIL workflows without manual hardware coordination.
- **Source:** Brief §2; UC-03.
- **Verification:** Test (API) + Demonstration (end-to-end with idle pool).
- **Priority:** Must.

### FR-08 — Pull-based claim acceptance

- **Statement:** The edge agent shall operate strictly on a pull-based polling mechanism over NATS to discover, accept, and lock open claims from the backend registry; no inbound network ports shall be required on the device.
- **Rationale:** Devices behind NAT, firewalls, or cellular links must operate without inbound exposure (security and feasibility).
- **Source:** Brief §2; UC-03 step 3-4.
- **Verification:** Inspection (firewall config) + Test (no listening ports on agent process).
- **Priority:** Must.

### FR-09 — Pipeline polling endpoint

- **Statement:** The backend shall expose a polling API endpoint allowing pipeline clients to monitor the preparation status of their claimed devices, including per-device state and last-known timestamp.
- **Rationale:** Asynchronous orchestration without server push to the pipeline.
- **Source:** Brief §2; UC-03 step 9.
- **Verification:** Test — concurrent pollers return consistent claim state.
- **Priority:** Must.

---

## Compliance & Security

### FR-10 — Cryptographic verification before execution

- **Statement:** The agent shall cryptographically verify the digital signature (Ed25519 over a JWS envelope) of all manifests, artifacts, and modular scripts before executing them; verification failure shall abort execution and emit a security event.
- **Rationale:** Prevents execution of tampered or untrusted instructions on regulated devices.
- **Source:** Brief §2; UC-01 Err-1; UC-02 step 5.
- **Verification:** Test (negative) — submit unsigned, expired, and wrong-key manifests; assert rejection.
- **Priority:** Must.

---

## Derived from Use Case Walkthroughs

### FR-11 — Cryptographic deployment acknowledgment *(from UC-01)*

- **Statement:** Upon successful completion of a deployment, the agent shall publish a signed acknowledgment containing `(device_serial, manifest_hash, deployed_version, timestamp, agent_signature)` to a dedicated audit subject.
- **Rationale:** Closes the IEC 62304 traceability loop with non-repudiable evidence of what ran where.
- **Source:** UC-01 step 11.
- **Verification:** Test — verify signature against device's public key; cross-check `manifest_hash` against the request.
- **Priority:** Must.

### FR-12 — Append-only audit storage *(from UC-01)*

- **Statement:** The control plane shall persist deployment audit records in append-only storage indexed by `(deployment_id, device_serial, manifest_hash, outcome, timestamp)`; records shall not be modifiable post-write.
- **Rationale:** Audit integrity for ISO 13485 / IEC 62304 evidence packages.
- **Source:** UC-01 step 12; NFR-01.
- **Verification:** Test — attempt modification through API and at storage layer; assert rejection.
- **Priority:** Must.

### FR-13 — `SystemdRestart` step with readiness probe *(from UC-02)*

- **Statement:** The execution engine shall provide a `SystemdRestart` step that restarts a configured unit and blocks until either (a) a configurable readiness condition is met (default: unit `active (running)`), or (b) a configurable timeout expires; timeout expiry is treated as step failure.
- **Rationale:** Application restarts must be observable and bounded.
- **Source:** UC-02 step 4 + Err-3.
- **Verification:** Test — restart a unit that delays activation; assert correct success/timeout behaviour.
- **Priority:** Must.

### FR-14 — Application-only update flows *(from UC-02)*

- **Statement:** The agent shall support modular flows that perform application-level updates (file swaps, service restarts, model loading) without writing to OS partitions or triggering reboots.
- **Rationale:** Fast iteration on robotic stacks does not warrant a full OS swap.
- **Source:** UC-02.
- **Verification:** Demonstration — run UC-02 manifest, assert no partition writes occur.
- **Priority:** Must.

### FR-15 — Claim TTL and lock preparation timeout *(from UC-03)*

- **Statement:** The Claim Registry shall enforce a per-claim TTL and a per-lock preparation timeout; on timeout, locks shall be released and slots re-offered to the pool.
- **Rationale:** Prevents orphaned reservations from disconnected devices or pipelines.
- **Source:** UC-03 Alt-2, Err-3.
- **Verification:** Test — simulate disconnect after lock; assert slot is re-offered within timeout window.
- **Priority:** Must.

### FR-16 — RBAC-protected claim creation *(from UC-03)*

- **Statement:** Claim creation, modification, and deletion APIs shall be protected by Role-Based Access Control; only principals holding the role `pipeline:create-claim` (or higher) may invoke them.
- **Rationale:** Prevents resource exhaustion / abuse by unauthorized clients; aligns with project Task 4.3.
- **Source:** UC-03 Err-1; Brief Task 4.3.
- **Verification:** Test (negative) — call API without role; assert `403`.
- **Priority:** Must.

### FR-17 — Per-device claim state in API responses *(from UC-03)*

- **Statement:** Claim status responses (`GET /v1/claims/{id}`) shall include a per-device sub-state map (`Pending|Preparing|Ready|Failed|InUse|Released`) with `last_update_timestamp` per device.
- **Rationale:** Pipeline observability into preparation progress.
- **Source:** UC-03 step 9; FR-09.
- **Verification:** Test (API contract) — assert response schema and correct state transitions.
- **Priority:** Must.

### FR-18 — Telemetry stream contract *(from UC-01, UC-02)*

- **Statement:** The agent shall stream a defined telemetry record set (CPU, memory, disk, NATS connectivity, agent version, active partition, last deployment id, ROS2 node status when applicable) to subject `device.<id>.telemetry` at a configurable interval (default: 30 s).
- **Rationale:** Enables fleet observability and feeds the readiness probes used by FR-09 / FR-15.
- **Source:** Brief Deliverable 2.3; UC-01, UC-02.
- **Verification:** Test — assert telemetry messages on subscribed subject conform to schema.
- **Priority:** Must.

### FR-19 — Agent self-update *(from Brief Deliverable 5.3)*

- **Statement:** The agent shall support atomic self-update by writing a new binary to a staging path, performing signature verification, atomically renaming over its installed path, and triggering a `systemd` restart of its own unit.
- **Rationale:** The agent must be field-updatable without site visits and without leaving the device in a non-managed state.
- **Source:** Brief Deliverable 5.3.
- **Verification:** Test — perform self-update from version N to N+1; assert restart and post-restart manifest acceptance.
- **Priority:** Must.

### FR-20 — Btrfs atomic snapshot variant *(from Brief Deliverable 3.2)*

- **Statement:** Where the device root filesystem is Btrfs, the agent shall implement atomic read-only snapshot creation before applying updates and rollback by snapshot promotion.
- **Rationale:** Btrfs offers a snapshot-based alternative to A/B that may be preferred on devices with constrained partition counts.
- **Source:** Brief Deliverable 3.2.
- **Verification:** Test — apply update on Btrfs root, sabotage health check, assert snapshot rollback.
- **Priority:** Should.

### FR-21 — Step output capture and reporting *(from Brief Task 1.3)*

- **Statement:** The execution engine shall capture stdout and stderr of each executed step, truncate to a configurable byte limit (default 1 MiB per stream per step), and include them in the `StepResult` published to the control plane.
- **Rationale:** Diagnosability without log shipping for every device.
- **Source:** Brief Task 1.3; UC-01 step 5.
- **Verification:** Test — assert `StepResult` payload contains captured streams and truncation marker when limits exceeded.
- **Priority:** Must.

### FR-22 — Device registration & tagging *(from Brief Deliverable 1.3)*

- **Statement:** The control plane shall provide APIs for device registration (mTLS-bootstrapped), tag assignment, and tag-based lookup; tags shall be used as the primary selector for desired-state targeting and claim matching.
- **Rationale:** Foundational for all targeting flows (production vs hil-testing, hardware classes).
- **Source:** Brief Deliverable 1.3.
- **Verification:** Test (API) — register device, assign tags, query by tag.
- **Priority:** Must.

---

## Config-Driven Engine Extensions

### FR-23 — Docker container primitive

- **Statement:** The agent shall implement a `DOCKER_CONTAINER` primitive that can (a) pull an OCI image by digest via the local container engine API, (b) verify that the pulled image's digest matches the value declared in the signed manifest, and then either (c1) cache that image for a later coordinated cutover or (c2) stop the previously running container of the same logical name and start the new container with manifest-specified arguments, networks, and volumes. Multi-container `docker compose` style deployments shall be expressed as a sequence of digest-pinned image pulls followed by a signed `SCRIPT_EXECUTION`/`SYSTEM_SERVICE` cutover step that performs the controlled `docker compose down` / `docker compose up` (or equivalent) once all required images are present.
- **Rationale:** Supports both single-container swaps and coordinated multi-service application updates for ROS2/robotics deployments (UC-02) without requiring an OS-level update, and makes containerized control-plane adjacent workloads first-class.
- **Source:** UC-02; [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md).
- **Verification:** Test (integration) — issue a manifest specifying a `DOCKER_CONTAINER` primitive in both cache-only and container-replace modes; assert image digest match, coordinated compose cutover or container swap, and rollback on failure.
- **Priority:** Should.

### FR-24 — Config-driven workflow (hardware independence)

- **Statement:** The agent's Rust source code shall contain **no** hardcoded references to GRUB, ext4, Btrfs, systemd-boot, U-Boot, specific partition layouts, or specific distributions. All such hardware/OS-specific behaviour shall be delivered through signed manifest scripts invoked via the `SCRIPT_EXECUTION` primitive. Introducing support for a new bootloader, filesystem, or distribution shall require **no agent source change**.
- **Rationale:** Enables the same agent binary to run across Yocto, Ubuntu, Debian, RHEL, and future distributions; decouples release cadence of the agent from OS-strategy evolution; enforces Open-Closed Principle.
- **Source:** [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md); UC-01, UC-02, UC-03.
- **Verification:** Inspection (source-level grep in CI) — search `crates/agent/` for forbidden tokens (`grub`, `grubenv`, `grub2-editenv`, `mkfs`, `btrfs`, `ext4`, distribution IDs) and fail build on any match outside a comments-only allowlist. Demonstration — same binary passes integration tests on at least Ubuntu 22.04, Debian 12, and a Yocto-built reference image.
- **Priority:** Must.

---

## Anti-Rollback, Offline Delivery & Conditional Execution

### FR-25 — Anti-rollback version monotonicity

- **Statement:** The agent shall persist the `desired_version` of every successfully applied manifest as `max_seen_version` in tamper-resistant storage (see [NFR-16](non-functional.md#nfr-16--tamper-resistant-rollback-state)). The agent shall reject any subsequent manifest whose `desired_version` is less than `max_seen_version` (per semver comparison), **except** when the manifest explicitly carries `allow_downgrade: true`, a non-empty `downgrade_justification`, **and** is signed by a key whose on-device trust-store entry carries the `downgrade` capability.
- **Rationale:** Prevents replay of correctly-signed but outdated manifests (the classic "signed-but-old" downgrade attack); makes emergency downgrades deliberate, separately-authorized, and auditable.
- **Source:** [ADR-0010](../adr/ADR-0010-anti-rollback-enforcement.md); Bambu Lab OTA reverse-engineering (`ota_anti` / `ota_ver` pattern); Android Verified Boot rollback index.
- **Verification:** Test (security) — sign a manifest with `desired_version = 2.1.0`, apply successfully; then replay a correctly-signed `desired_version = 2.0.0` manifest; assert `REJECTED_ROLLBACK` and audit record. Also test the authorized downgrade path with a `downgrade`-capability key.
- **Priority:** Must.

### FR-26 — `lower_limit_version` gating

- **Statement:** Every manifest shall carry a `lower_limit_version` field (semver). The agent shall refuse the manifest (error `REJECTED_BELOW_LOWER_LIMIT`) if the device's `current_deployed_version` is less than `lower_limit_version`. This gate is evaluated **before** any step executes.
- **Rationale:** Enables staged migration paths (e.g., forces a fleet through an intermediate version that performs schema migration or trust-store rotation).
- **Source:** [ADR-0010](../adr/ADR-0010-anti-rollback-enforcement.md); Bambu `lower_limit`.
- **Verification:** Test — publish a manifest with `lower_limit_version = 2.0.0` to a device on `1.9.0`; assert rejection with correct error code; upgrade device to `2.0.0`; re-apply; assert success.
- **Priority:** Must.

### FR-27 — Offline / air-gapped bundle mode

- **Statement:** The agent shall support applying a deployment from a signed **offline bundle** (a deterministic `.zip` containing `manifest.jws` and an `artifacts/` directory) via three paths: (a) drop-directory `inotify` watch on `/var/lib/ota/bundles/`; (b) authenticated local CLI invocation `ota-agent apply-bundle <path>`; (c) a NATS `desired-state` that references a staged bundle via a `bundle://` URL. Offline bundles MAY carry the same artifact mix as online releases, including raw/Yocto/VMDK disk images, signed device-profile scripts, compose descriptors, and offline Debian/Ubuntu package sets (`.deb` payloads plus apt metadata) when the manifest's signed steps know how to consume them. All verification rules ([§5.8](../arc42/05-building-block-view.md#58-jws-manifest-envelope-adr-0003--adr-0008)) — including anti-rollback — shall apply identically to the offline path.
- **Rationale:** Covers air-gapped medical manufacturing, HIL labs behind data diodes, field service of robots with intermittent WAN, and regulatory environments that mandate offline-only updates.
- **Source:** [ADR-0011](../adr/ADR-0011-offline-bundle-format.md); Bambu offline OTA zip.
- **Verification:** Test (integration) — build a bundle from the online pipeline; mount to a device over USB; verify drop-dir pickup, signature check, anti-rollback gate, artifact SHA-256 verification, successful execution of representative image/package/container artifacts, and `DeploymentAck` queuing in the outbox with replay on NATS reconnect.
- **Priority:** Should.

### FR-28 — Conditional step execution (`applies_if` predicate)

- **Statement:** Each step in `deployment_steps` and `rollback_steps` MAY carry an `applies_if` predicate expressed in the grammar defined in [§5.4.1](../arc42/05-building-block-view.md#541-manifest--json-schema-sketch-jws-payload). If the predicate evaluates **false** against the device's observed state (tags, capabilities, device profile), the step is **skipped** (not failed) and a `STEP_SKIPPED` audit event is emitted. Unknown predicate kinds shall cause the manifest to be rejected (fail-closed).
- **Rationale:** Enables a single manifest to target a heterogeneous fleet with hot-pluggable accessories (AMS-style sub-components, optional Docker runtime, mixed device profiles) without the cloud pre-rendering per-device manifests.
- **Source:** Bambu sub-descriptor pattern (accessory-specific descriptors skipped when accessory absent).
- **Verification:** Test — publish a manifest with a step gated on `has_capability: "docker"`; apply to a device without Docker; assert `STEP_SKIPPED`; apply to a device with Docker; assert executed.
- **Priority:** Should.

### FR-29 — Manifest-level artifact pin index

- **Statement:** Every manifest shall carry a top-level `artifacts[]` array listing every referenced artifact (script, image, binary, asset blob) with its logical name, size, and SHA-256. For each step parameter that references an artifact (`script_ref`, `FILE_TRANSFER.url` with scheme `bundle://`, `AGENT_SELF_UPDATE.url`, `DOCKER_CONTAINER.image_digest_manifest`), the per-step hash and the `artifacts[]` entry MUST agree; mismatch rejects the manifest. The cloud publish pipeline and the bundle build tool shall refuse to emit a manifest/bundle whose `artifacts[]` is not a superset of everything the steps reference.
- **Rationale:** Single root-of-trust property — one JWS signature + a small pin index gates N cheap SHA-256 checks at fetch time; eliminates the failure mode where a step hash was updated but a mirror was not (or vice versa); matches the Bambu `ota-package-list.json` pattern in our JSON+JWS dialect.
- **Source:** Bambu `ota-package-list.json` root-manifest pattern; [ADR-0011](../adr/ADR-0011-offline-bundle-format.md).
- **Verification:** Test — publish a manifest with a step referencing an artifact missing from `artifacts[]`; assert CI pipeline rejects at build time. Test — tamper with a pinned artifact after signing; assert agent rejects with SHA-256 mismatch.
- **Priority:** Must.

---

## Device-Profile Authoring

### FR-30 — Device-profile script authoring conventions

- **Statement:** Every script delivered via a signed device-profile library and executed by the agent's `SCRIPT_EXECUTION` primitive SHALL conform to the conventions codified in [§8.6.5](../arc42/08-crosscutting-concepts.md#865-device-profile-script-authoring-conventions), including (at a minimum):
  1. The script MUST NOT invoke `reboot`, `systemctl reboot`, or `shutdown`; the `REBOOT` primitive is the exclusive mechanism for triggering reboots.
  2. The script MUST begin with `set -euo pipefail` and install a `trap cleanup EXIT` that tears down any mounts, loop/nbd devices, and temporary directories.
  3. The script MUST receive parameters via `OTA_*`-prefixed environment variables declared in the manifest's `SCRIPT_EXECUTION.parameters.env`, not via positional arguments.
  4. The script MUST log only to stdout/stderr (captured by the agent and streamed via OTLP); it MUST NOT write directly to `/var/log/`.
  5. Before any destructive block-device operation (`dd`, `mkfs`, `wipefs`, `parted`, partition delete), the script MUST perform a **target-safety interlock** verifying at minimum that the target device is not the currently-mounted root and that it matches the expected device-profile role; the script MUST abort with a non-zero exit if either check fails.
  6. The script MUST declare its required host capabilities in the manifest via `applies_if: { has_capability: ... }` predicates ([FR-28](#fr-28--conditional-step-execution-applies_if-predicate)).
  7. The script MUST NOT fork long-lived daemons or detach from the agent's exec handle.
  8. Production scripts MUST NOT emit `--unrestricted` / password-less GRUB menu entries absent an explicit documented risk acceptance for the device class.
  9. The script MUST carry a version header (device-profile name, script name, script version) to enable design-history-file traceability.
- **Rationale:** Learned from field-tested prototype scripts, which revealed concrete anti-patterns (in-script `reboot`, hard-coded log paths, positional args, absent safety interlocks, undeclared host dependencies). Codifying these as enforceable conventions prevents a class of deployment bugs and bricking incidents.
- **Source:** Prototype `deploy_rootfs.sh` / GRUB chainload scripts; [ADR-0004](../adr/ADR-0004-ab-partitioning-grubenv.md); [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md).
- **Verification:** Inspection (CI linter on the profile library repository; rejects on convention violation) — parse every script for prohibited tokens (`\breboot\b`, `\bshutdown\b`, missing `set -euo pipefail`, missing `trap`, literal `$1`/`$2` references, `/var/log` writes, `--unrestricted` flags). Test — publish a deliberately non-conformant script to a staging profile; assert CI rejects before signing.
- **Priority:** Must.

---

## Cross-Reference

See the [traceability matrix](traceability-matrix.md) for the full UC ↔ FR ↔ arc42 ↔ Epic mapping.
