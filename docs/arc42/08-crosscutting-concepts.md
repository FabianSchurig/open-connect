# 8. Crosscutting Concepts

These concepts span more than one building block. Each subsection cites the requirements it satisfies and the runtime scenarios where it manifests.

---

## 8.1 Security

### 8.1.1 Transport Security

- **mTLS everywhere.** All NATS connections (device↔hub, leaf↔hub, control-plane↔hub) and all REST API calls require mutual TLS. Bearer JWTs may be layered on REST for human-friendly principals but only after mTLS termination at the API gateway. → [NFR-11](../requirements/non-functional.md#nfr-11--all-transport-authenticated-from-fr-08-fr-10).
- **NATS NKey second factor.** Every NATS connection requires both a valid mTLS certificate **and** a successful NKey challenge-response. Subject-level authorization (ACLs) is bound to the **NKey public key**, not the cert DN. Decentralized JWT-based Operator/Account/User claims govern who may publish/subscribe where. → [ADR-0009](../adr/ADR-0009-nats-nkey-authentication.md).
- **No plaintext mode**, including dev. CI fails any config that omits TLS.

### 8.1.2 Identity & Trust Stores

| Identity | Holder | Format | Rotation |
|----------|--------|--------|----------|
| Device mTLS cert | Device | X.509 | Short-lived (≤ 30 days), auto-renewed via long-lived device-key-bootstrapped flow. |
| Device long-lived key | Device | Ed25519 | Rotated only at servicing; basis for ack signatures (FR-11). |
| Device NATS NKey | Device | Ed25519 (NATS NKey encoding) | Long-lived; revocable via JWT account revocation list ([ADR-0009](../adr/ADR-0009-nats-nkey-authentication.md)). |
| NATS Operator / Account JWT signing keys | Fleet operator | Ed25519 NKey | Custody per [ADR-0003](../adr/ADR-0003-jws-ed25519-manifests.md) TD-01 (HSM in v2). |
| Control-plane pod cert | K8s pods | X.509 | cert-manager rotates. |
| Manifest signing key | Release Mgr / Pipeline service account | Ed25519 | Rotation procedure documented; v1 manual, v2 automated via short-lived signing service. |
| Device-profile script signing key | Release Mgr | Ed25519 | Same custody as manifest signing key; individual script bodies verified via `script_sha256` in the signed manifest. |

The on-device **trust store** (`/etc/ota-agent/trust-store/`) is a directory of PEM public keys keyed by `kid`. Adding/removing keys is itself a signed operation (`UpdateTrustStore` step type — planned for v1.1).

### 8.1.3 Authorization (RBAC)

Roles enforced at the API gateway:

| Role | Capabilities |
|------|--------------|
| `device:register` | Create/retire devices. |
| `device:read` | List/inspect devices. |
| `device:tag` | Manage tags. |
| `release:publish` | Publish signed `DesiredState`. |
| `release:read` | Read deployment status. |
| `pipeline:create-claim` | Create/release claims. *(FR-16)* |
| `pipeline:read-claim` | Poll claim status. |
| `audit:read` | Query audit log. |
| `audit:export` | Generate signed evidence bundles. |
| `admin` | Superset; assigned sparingly. |

NATS subject ACLs encode the device-self-only invariant: a device may only publish/subscribe under `device.<its-own-serial>.*`.

### 8.1.4 SELinux Strict Confinement

`ota_agent.pp` (custom Type Enforcement module) is loaded on every device:

- `ota_agent_t` confines the agent process. Allowed transitions enumerated explicitly.
- `ota_update_script_t` is assigned to scripts staged by the agent under `/var/lib/ota/spool/` via explicit **`type_transition` rules** (kernel-enforced auto-labeling on create). The agent additionally invokes `restorecon`-equivalent logic as belt-and-braces. Scripts execute in this confined domain; they cannot perform operations not permitted by the policy (no raw block-device access outside of policy rules, no kernel module load, etc.).
- **No `execmem`, no `execstack`, no `execheap`** — the agent never allocates executable anonymous memory. Consequently, embedded scripting runtimes, JITs, and in-memory module loaders are off the table. This constraint *enables* the config-driven model ([ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md)) because all new behaviour ships as separately labeled scripts, not as in-process code.
- **No runtime domain transitions** — the agent never calls `setexeccon()`; all transitions are policy-driven at exec time against statically labeled targets.
- Narrow per-primitive rules: `SYSTEM_SERVICE` gets a `systemctl` exec transition; `DOCKER_CONTAINER` gets a narrow socket rule to `container_var_run_t`; `REBOOT` gets a `reboot_t` chain. See the primitive → domain mapping in [ADR-0006](../adr/ADR-0006-selinux-strict-policy.md).
- → [NFR-12](../requirements/non-functional.md#nfr-12--selinux-strict-confinement-from-brief-deliverable-51), [ADR-0006](../adr/ADR-0006-selinux-strict-policy.md).

### 8.1.5 Cryptographic Verification of Manifests

JWS Compact Serialization with `alg=EdDSA`. Verification rules: see [§5.8](05-building-block-view.md#58-detailed-design--jws-manifest-envelope). Notably, the agent **does not** consult a `jku` URL or fetch keys at verification time; the trust store is local and managed out-of-band, eliminating signature-injection-via-network attacks.

### 8.1.6 Anti-Rollback

See [ADR-0010](../adr/ADR-0010-anti-rollback-enforcement.md), [FR-25](../requirements/functional.md#fr-25--anti-rollback-version-monotonicity), [FR-26](../requirements/functional.md#fr-26--lower_limit_version-gating), [NFR-16](../requirements/non-functional.md#nfr-16--tamper-resistant-rollback-state).

Three layered checks enforce that the fleet cannot silently regress:

1. **`lower_limit_version` gate** (manifest-declared). Rejects manifests that would run on a device older than the release engineer declared as the minimum starting point. Enables staged migrations (e.g., forces a fleet through `2.0.0` before accepting `3.x`).
2. **Monotonic `max_seen_version`** (device-persisted). Every successful deployment bumps `max_seen_version`. Subsequent manifests with `desired_version < max_seen_version` are rejected unless the manifest explicitly carries `allow_downgrade: true` + `downgrade_justification`, and is signed by a **different** `kid` that holds the `downgrade` capability in the trust store.
3. **Tamper-resistant storage** for `max_seen_version` — preference order: TPM 2.0 NV index → ATECC608 / secure element → SELinux-confined on-disk journal with hash chain. Divergence between TPM value and on-disk value triggers an integrity alert.

Every accept / reject / downgrade decision produces an audit record with `rollback_decision ∈ {accepted, rejected_below_lower_limit, rejected_rollback, accepted_downgrade}`.

### 8.1.7 Threat Model (summary)

| Threat | Mitigation |
|--------|------------|
| Tampered manifest in transit | JWS Ed25519 verification (FR-10). |
| Compromised NATS hub injecting commands | Manifest signature verification is independent of transport; commands without valid signatures are rejected. |
| Malicious script in a signed manifest | SELinux `ota_update_script_t` confinement; capability inventory bounded to the primitive set ([§5.4.1](05-building-block-view.md#541-manifest--json-schema-sketch-jws-payload)). |
| Device key compromise | mTLS cert rotation; device retirement endpoint; ack signatures localize blast radius to one device. |
| Replay of old signed manifest | Manifests carry `deployment_id` and `issued_at`; agent rejects `issued_at` older than configured window or with a `deployment_id` already executed. **Plus:** anti-rollback gate ([§8.1.6](#816-anti-rollback)) rejects any manifest whose `desired_version < max_seen_version` absent a downgrade-signed override. |
| Valid-signed old manifest replayed via offline bundle | Anti-rollback gate applies uniformly to offline and online paths ([ADR-0011](../adr/ADR-0011-offline-bundle-format.md)). |
| Staged-migration bypass | `lower_limit_version` gate — release engineer declares required intermediate versions. |
| Tampering with on-disk rollback journal | SELinux `ota_rollback_t` confinement; hash-chained journal; TPM NV-index mirror (where available) catches divergence. |
| Malicious USB bundle | Bundle signature + anti-rollback + operator CLI challenge + optional factory-signed co-signature (v1.1). |
| Pipeline abuse (claim flooding) | RBAC + per-principal claim quota. |
| Audit tampering | WORM storage + cryptographic chain (NFR-13). |

---

## 8.2 Observability

The observability stack is **vendor-neutral by design**. All telemetry, logs, and (where propagated) traces travel in **OpenTelemetry Protocol (OTLP)** form over the existing NATS fabric; the control plane bridges these into whichever backend the operator chooses (Prometheus, Grafana Tempo, Loki, Elastic, Datadog, New Relic, …) without the agent needing to know.

### 8.2.1 Telemetry — OTLP over NATS

- **What is collected (metrics):** CPU, memory, disk usage, network connectivity to NATS, `agent_version`, `active_partition`, `current_deployment_id`, optional ROS2 node status (`/rosout`-derived counters), Docker container health. → [FR-18](../requirements/functional.md#fr-18--telemetry-stream-contract-from-uc-01-uc-02).
- **Encoding:** the agent builds an **OTLP payload** (`opentelemetry.proto.metrics.v1.ExportMetricsServiceRequest` / `logs.v1.ExportLogsServiceRequest`) and places it in the `otlp_payload` bytes field of the `Telemetry` Protobuf wrapper defined in [§5.4.4](05-building-block-view.md#544-machine-messages--protobuf).
- **Cadence:** default 30 s, configurable per device profile. Logs stream continuously.
- **Transport:** NATS subject `device.<serial>.telemetry`, JetStream-backed for durability.
- **Control-plane bridge:** a **Telemetry Collector** service (Go) subscribes to `device.*.telemetry`, enriches with device tags/hardware profile, and forwards via standard OTLP/gRPC to the configured backend. Swapping backends is a collector-configuration change, not an agent/control-plane change.
- **Local buffer:** quota-bounded ring (default 50 MiB on `/var/lib/persistent`); FIFO drop. → [NFR-06](../requirements/non-functional.md#nfr-06--robot-autonomy-during-cloud-outage-from-uc-02).
- **Decoupled from update lifecycle.** The `Telemetry Sampler` runs independently of the execution engine so hardware metrics and ROS2 health continue to flow during, and outside of, deployments.

```mermaid
flowchart LR
    Agent["Agent\n(OTLP encoder)"] -->|Telemetry{otlp_payload}| NATS
    NATS --> Collector["Telemetry Collector\n(Go, OTLP proxy)"]
    Collector -->|OTLP/gRPC| Backend[("Any OTLP backend\nPrometheus / Tempo / Loki / …")]
```

### 8.2.2 Logs

- Agent logs to `journald` and `/var/log/ota-agent/` (rotated). Structured JSON line format, with OTLP `LogRecord` equivalents also emitted onto NATS for centralized collection.
- Step stdout/stderr captured into `StepResult` (bounded, FR-21) and additionally retained on disk for forensic recovery.
- Control plane: structured JSON to stdout, also emitted as OTLP logs, scraped/forwarded by the standard collector.

### 8.2.3 Metrics

- Control plane exposes Prometheus `/metrics` for operator-friendly scraping: API latency, RBAC denials, claim TTL sweep counts, manifest publish rates, NATS publish errors.
- The same metrics are additionally emitted as OTLP metrics for OTLP-native stacks.
- Device-side metrics flow via OTLP-over-NATS (above); an optional Prometheus exposition can be enabled on a per-device basis where on-device scraping is permitted.

### 8.2.4 Tracing

- OpenTelemetry traces in the control plane (API → service → DB → NATS).
- **Device-boundary trace propagation** is opt-in: when enabled, the control plane injects a W3C `traceparent` into the JWS header; the agent extracts it and starts a new span for deployment execution, exporting spans via OTLP-over-NATS. Off by default (topology-disclosure concern); enabled per-fleet where operators accept the trade-off.
- `deployment_id` always serves as a correlation ID across the device boundary, even when tracing is disabled.

---

## 8.3 Resilience

### 8.3.1 Network Resilience

- **NFR-04.** Exponential backoff with jitter on all NATS reconnects (initial 1 s, cap 60 s).
- **Pull cadence.** Agents poll `device.<serial>.desired-state` every `poll_interval` (default 30 s) **and** opportunistically when reconnecting after disconnect.
- **JetStream durability.** Telemetry, step-results, and acks survive transient NATS outages.

### 8.3.2 Local-First Operation

- **NFR-06.** Robots' Leaf Nodes accept publishes without WAN connectivity; ROS2 traffic on the local LAN is unaffected.
- Telemetry buffers locally up to the configured quota; on reconnection, drains in-order.

### 8.3.3 Idempotency

- All device-initiated operations use deterministic IDs (`deployment_id`, `claim_id`, `step_id`, `attempt_id`) so the control plane can dedup retries.
- `ClaimLock` carries `attempt_id`; the registry rejects duplicates within a TTL.

### 8.3.4 Graceful Degradation

- If the artifact store is unreachable, `DownloadArtifact` retries with backoff; the deployment remains in `Executing` and is not marked failed until a per-step timeout expires.
- If signature verification fails (FR-10), the agent does **not** retry the same manifest; it requires a new signed publish.

---

## 8.4 Configuration & Desired-State Model

- **Desired state is declarative.** The control plane stores the *latest* `DesiredState` per device or per tag; the agent reconciles by comparing it to the device's recorded `current_deployment_id` **and** `current_deployed_version`.
- **Tag-targeted vs serial-targeted manifests.** Tag-targeted manifests resolve to a per-device manifest at publish time; per-device deployment IDs are derived (`{deployment_id}/{serial}`) for traceability.
- **Reconciliation property.** Re-publishing the same `deployment_id` is a no-op; publishing a new `deployment_id` supersedes any incomplete previous flow (with rollback of the in-flight one if applicable).
- **Capability-gated steps.** A single manifest can target a heterogeneous fleet by using `applies_if` predicates on individual steps (see [§5.4.1](05-building-block-view.md#541-manifest--json-schema-sketch-jws-payload)). Steps whose predicate evaluates false are **skipped, not failed**, allowing hot-pluggable accessories (e.g., AMS-style sub-components) and mixed device-profile fleets to share one release.

### 8.4.1 Delivery Channels

The same declarative manifest is accepted via three interchangeable channels; verification rules are identical in all cases.

| Channel | Transport | Trigger | Typical use |
|---------|-----------|---------|-------------|
| **Online (default)** | NATS pull on `device.<serial>.desired-state`; artifacts via signed HTTPS URLs | Cloud publish | Normal fleet operations |
| **Offline bundle (drop)** | Zip on local FS (`/var/lib/ota/bundles/`); artifacts via `bundle://` URLs | inotify on drop-dir | Air-gapped manufacturing, field service ([ADR-0011](../adr/ADR-0011-offline-bundle-format.md)) |
| **Offline bundle (CLI)** | Zip at path given to `ota-agent apply-bundle` | Authorized operator | Supervised manual update |

### 8.4.2 Asset-Bundle Pattern

Non-firmware assets (error-code dictionaries, translations, filament/calibration databases, ML models, ROS parameter sets) ride the **same** manifest pipeline as firmware — they are simply manifests whose deployment steps compose `FILE_TRANSFER` + `SCRIPT_EXECUTION`/`SYSTEM_SERVICE` (e.g., *"unzip into `/var/lib/app/assets/`, restart the service that rereads them"*). This eliminates the need for a separate asset CDN, gives assets signing / versioning / anti-rollback / audit for free, and keeps a single release cadence and evidence bundle.

---

## 8.5 Versioning

| Artifact | Versioning Rule |
|----------|-----------------|
| Agent binary | Semantic versioning (`MAJOR.MINOR.PATCH`). Major bumps require migration story. |
| Control-plane container image | SemVer; Helm chart `appVersion` tracks. |
| Protobuf schemas | Field numbers are immutable; deprecation marks fields `reserved`. `DesiredState.schema_version` increments on breaking semantic changes; agents reject unknown major versions. |
| REST API | URL-prefixed (`/v1`). Breaking changes go to `/v2`; old endpoints remain for one release minimum. |
| Manifest JOSE header `ver` | Tracks JWS envelope evolution independent of payload schema. |

**Compatibility windows:**

- Control plane MUST accept devices running agent versions within the last 2 minor releases.
- Devices MUST accept manifests with `schema_version` within the major version they implement.

---

## 8.6 Error Handling & Rollback

### 8.6.1 Step-Level Errors

- Non-zero exit code → halt sequence (FR-02), unless the step has `continue_on_error=true` (rare; reserved for explicitly idempotent cleanups).
- Each step has a `timeout_seconds`; expiry treated as failure.
- `StepResult` always emitted, regardless of success.

### 8.6.2 Deployment-Level Rollback

Rollback is expressed as a **`rollback_steps` array in the signed manifest** — the same primitive vocabulary as the forward path. The engine executes `rollback_steps` when a deployment-step fails and the failed step is not marked `continue_on_error`.

- For OS-level updates (device profile = ext4 A/B + GRUB): rollback is a `SCRIPT_EXECUTION` of `restore-grubenv.sh` that reverts `boot_part=previous_part`, plus optional re-initialization of the staged bank. If reboot already occurred, the boot-counter handles it (see 8.6.3) — no agent action needed.
- For Btrfs device profile: rollback is a `SCRIPT_EXECUTION` invoking `btrfs subvolume set-default` against the pre-update snapshot.
- For application-only updates (UC-02, UC-03): rollback composes `FILE_TRANSFER`/`SCRIPT_EXECUTION`/`SYSTEM_SERVICE` to restore previous artifacts and restart services. The engine snapshots replaced files into `/var/lib/ota-agent/staging/<deployment_id>/rollback/` before each `FILE_TRANSFER` overwriting an existing path, enabling even last-resort local recovery.
- For Docker-based updates (`DOCKER_CONTAINER` primitive, FR-23): rollback re-pulls the previous image digest and swaps containers atomically.

### 8.6.3 Boot-Time Rollback (two supported `grubenv` patterns)

Two complementary GRUB-side mechanisms are in use across the reference device profiles (see [ADR-0004](../adr/ADR-0004-ab-partitioning-grubenv.md), [§5.9](05-building-block-view.md#59-boot-redundancy-reference-device-profiles)). Both are covered by the agent.

| Pattern | Used by profile | Variables | Behaviour | Rollback attempts |
|---------|-----------------|-----------|-----------|-------------------|
| **Boot-counter** | `ext4-partition-ab` (default), `btrfs-snapshot` | `boot_part`, `boot_count`, `boot_success`, `previous_part` | GRUB script decrements `boot_count` each chainload; if it reaches 0 with `boot_success=0`, sets `boot_part=previous_part` and chainloads back. → [§6.2](06-runtime-view.md#62-uc-01-err-3--boot-counter-rollback). | Multi-attempt (N, default 3) |
| **One-shot `next_entry`** | `dual-disk-chainload` | `next_entry`, `saved_entry` | Agent sets `next_entry` via `grub-reboot <menuentry>` for one boot only. On success the agent promotes to `saved_entry` via `grub-set-default`; on failure GRUB has already cleared `next_entry`, so the next boot reverts automatically. | Single-attempt |

Boot-counter is the preferred choice where achievable (stronger un-brickability). One-shot is valid where BIOS/GRUB counter scripts are unreliable or chainloading makes counter semantics awkward.

In both cases, the agent on the recovered side publishes a `Reverted` deployment event.

### 8.6.4 Self-Update Failure

- Watchdog service (`ota-agent-watchdog.service`) restores `agent.bak` if the new binary fails to start within a watchdog window. → [§6.5](06-runtime-view.md#65-agent-self-update-fr-19).

### 8.6.5 Device-Profile Script Authoring Conventions

Device-profile scripts are **signed source code** ([ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md)) that the agent fork/execs. They are safety-critical; the following conventions are enforced by CI on the profile library repository and captured as a requirement in [FR-30](../requirements/functional.md#fr-30--device-profile-script-authoring-conventions).

| # | Convention | Rationale |
|---|------------|-----------|
| 1 | **Scripts MUST NOT call `reboot` / `systemctl reboot` / `shutdown`.** The `REBOOT` primitive is the agent's exclusive reboot mechanism. | A script that reboots prematurely prevents the agent from publishing `StepResult`, breaks rollback sequencing, and makes the deployment look failed to the control plane even on success. |
| 2 | **Scripts MUST start with `set -euo pipefail`.** | `-e` alone (as in the prototype) allows unset variables and pipeline failures through. `-euo pipefail` is the baseline. |
| 3 | **Scripts MUST install a `trap cleanup EXIT`** that tears down any mounts, loop/nbd devices, and temp dirs. | Prevents leaking block devices when a step fails mid-flight; the prototype's `qemu-nbd --disconnect` example is the canonical pattern. |
| 4 | **Scripts MUST receive parameters via `OTA_*`-prefixed environment variables** (as declared in the manifest's `SCRIPT_EXECUTION.parameters.env`), **not via positional arguments.** | Named inputs are self-documenting and harder to misorder. The `OTA_` prefix isolates them from host env. |
| 5 | **Scripts MUST log only to stdout/stderr.** The agent captures both as step output and streams via OTLP ([§8.2.1](#821-telemetry--otlp-over-nats)). Direct writes to `/var/log/...` are prohibited. | Centralized observability; no log paths to provision. |
| 6 | **Scripts MUST include a safety interlock before any destructive operation** (`dd`, `mkfs`, `wipefs`, `parted`, partition delete) that writes to a block device. The interlock MUST verify at minimum: target is **not** the currently-mounted root filesystem; target matches the expected device profile's role (e.g., "Bank B"); optionally, disk model/serial matches provisioning-time expectation. | A typo in `$OTA_TARGET_DISK` that points at the running root wipes the device. Interlocks catch this. |
| 7 | **Scripts MUST declare required host capabilities** in the manifest via `applies_if: has_capability: "<cap>"` predicates (see [§5.4.1](05-building-block-view.md#541-manifest--json-schema-sketch-jws-payload)). | Scripts that silently depend on `qemu-nbd` / `nbd` kernel module / specific `grub-reboot` version fail late; predicates move detection to manifest verification time. |
| 8 | **Scripts SHOULD be idempotent / re-runnable** when physically possible. Where impossible (e.g., `dd` over a live partition), the script MUST short-circuit if it detects the effect is already applied. | The engine may re-run a step after a transient retry; non-idempotent scripts double-apply. |
| 9 | **Scripts MUST NOT fork long-lived daemons or detach.** They run to completion, return an exit code, and release the agent's exec handle. | The agent waits on the child; background detachment leaks processes into the wrong SELinux domain and bypasses timeout enforcement. |
| 10 | **Production scripts MUST NOT use `--unrestricted` or password-less GRUB menu entries** unless explicitly risk-accepted for the device class. | In the prototype, `--unrestricted` lets any console user select the alternate disk — an unacceptable posture on medical devices. |
| 11 | **Scripts MUST version themselves** (header comment: profile name, script version, target device-profile version). | Traceability in the design history file; enables the manifest linter to cross-check script version against a profile's manifest. |

#### Example — the prototype refactored under these conventions

Before (prototype, `deploy_rootfs.sh`):

```bash
#!/bin/bash
set -e
OVA_PATH="$1"
TARGET_DISK="${2:-/dev/sda}"
LOGFILE="/var/log/tem-live-controller.log"
exec >> "$LOGFILE" 2>&1
# ...
dd if="/dev/nbd0" of="$TARGET_DISK" bs=4M status=progress
# ...
sync
reboot
```

After (conformant):

```bash
#!/bin/bash
# device-profile: dual-disk-chainload
# script:         deploy-rootfs-from-ova.sh
# version:        1.0.0
set -euo pipefail

: "${OTA_OVA_PATH:?OTA_OVA_PATH must be set by the manifest}"
: "${OTA_TARGET_DISK:?OTA_TARGET_DISK must be set by the manifest}"
: "${OTA_EXPECTED_TARGET_ROLE:?must be Bank-A or Bank-B}"

TMPDIR="$(mktemp -d)"
cleanup() {
    qemu-nbd --disconnect /dev/nbd0 2>/dev/null || true
    rm -rf "$TMPDIR"
}
trap cleanup EXIT

# --- Safety interlock (Convention 6) ---
current_root_src="$(findmnt -no SOURCE /)"
current_root_disk="$(lsblk -no PKNAME "$current_root_src")"
if [ "/dev/$current_root_disk" = "$OTA_TARGET_DISK" ]; then
    echo "FATAL: target $OTA_TARGET_DISK is the currently-mounted root disk; refusing." >&2
    exit 2
fi
expected_disk_role="$(lsblk -no LABEL "$OTA_TARGET_DISK" | head -n1)"
if [ "$expected_disk_role" != "$OTA_EXPECTED_TARGET_ROLE" ]; then
    echo "FATAL: target $OTA_TARGET_DISK has role '$expected_disk_role', expected '$OTA_EXPECTED_TARGET_ROLE'." >&2
    exit 2
fi

echo "deploy-rootfs-from-ova: unpacking OVA $OTA_OVA_PATH"
tar -xf "$OTA_OVA_PATH" -C "$TMPDIR"
VMDK_FILE="$(find "$TMPDIR" -name '*.vmdk' -print -quit)"
[ -n "$VMDK_FILE" ] || { echo "FATAL: no VMDK in OVA" >&2; exit 3; }

echo "deploy-rootfs-from-ova: attaching $VMDK_FILE via qemu-nbd"
modprobe nbd max_part=8
qemu-nbd --connect=/dev/nbd0 "$VMDK_FILE"
# Poll for readiness instead of sleep
for _ in $(seq 1 20); do [ -b /dev/nbd0 ] && break; sleep 0.25; done
[ -b /dev/nbd0 ] || { echo "FATAL: /dev/nbd0 not ready" >&2; exit 4; }

echo "deploy-rootfs-from-ova: copying /dev/nbd0 -> $OTA_TARGET_DISK"
dd if=/dev/nbd0 of="$OTA_TARGET_DISK" bs=4M status=progress conv=fdatasync
sync
echo "deploy-rootfs-from-ova: complete. Agent will issue REBOOT primitive."
# NOTE: NO reboot call here — Convention 1.
```

Paired with a manifest step declaring required capabilities:

```json
{
  "step_id": "02-deploy-rootfs",
  "primitive": "SCRIPT_EXECUTION",
  "applies_if": { "all_of": [
    { "device_profile": "dual-disk-chainload" },
    { "has_capability": "qemu-nbd" },
    { "has_capability": "kmod-nbd" }
  ]},
  "parameters": {
    "interpreter": "/bin/bash",
    "script_ref": "device-profiles/dual-disk-chainload/deploy-rootfs-from-ova.sh",
    "script_sha256": "…",
    "env": {
      "OTA_OVA_PATH": "/var/lib/ota/spool/rootfs-v2.4.ova",
      "OTA_TARGET_DISK": "/dev/sdb",
      "OTA_EXPECTED_TARGET_ROLE": "Bank-B"
    }
  },
  "timeout_seconds": 1800
}
```

And a separate manifest step — **not** a call inside the script — handles the reboot:

```json
{ "step_id": "04-reboot", "primitive": "REBOOT", "parameters": { "grace_seconds": 30 } }
```

This refactor is the canonical shape the prototype will take when it moves into the profile library.

---

## 8.7 Audit & Compliance

### 8.7.1 Audit Record Shape

Every audit record is a JSON object (NDJSON in exports):

```json
{
  "schema": "otap.audit.v1",
  "kind": "deployment_event",
  "deployment_id": "uuid",
  "device_serial": "string",
  "manifest_hash": "sha256:...",
  "requested_by": "principal-subject",
  "requested_at": "RFC3339",
  "event": "requested|step_succeeded|step_failed|reverted|acknowledged",
  "step_id": "string|null",
  "exit_code": 0,
  "agent_signature": "base64url|null",
  "prev_record_hash": "sha256:...",
  "this_record_hash": "sha256:..."
}
```

`prev_record_hash` + `this_record_hash` form a **hash chain** so any tampering with intermediate records is detectable on export. The chain is anchored daily into the export bundle (and, optionally, an external timestamp service).

### 8.7.2 Storage Immutability

- Application-level: `audit-svc` exposes only `INSERT` and `SELECT` to the audit store.
- Storage-level: object lock (S3-compatible) with `Compliance` retention mode for the configured retention period. → [NFR-13](../requirements/non-functional.md#nfr-13--append-only-audit-immutability-reinforces-fr-12).

### 8.7.3 Evidence Bundles (IEC 62304 / ISO 13485 / ISO 81001-5-1)

`audit-export` CronJob produces signed daily bundles per `deployment_id` (or on demand via `/v1/audit/export`). A bundle contains:

1. The original signed `DesiredState` (JWS envelope).
2. All `StepResult` records for the deployment.
3. The `DeploymentAck` from each target device (verified Ed25519 signature included).
4. Hash-chain anchors for the records included.
5. A bundle-level signature by an evidence-export key.

Reviewers verify the bundle without access to live infrastructure, satisfying audit independence.

---

## 8.8 Testing Strategy (foreshadowed)

Implementation phase will cover:

- Unit tests in Rust for each step runner and the state machine.
- Integration tests with a NATS test container + a Postgres test container.
- HIL smoke tests using the platform's own claim mechanism (eat-our-own-dogfood).
- Chaos tests for NFR-04 (network partitions) and NFR-08/09 (concurrent claims).
- SELinux policy tests (`audit2allow` should be empty during normal operation).
- Boot-counter tests on a VM that fakes successive failures.

---

## 8.9 Internationalization & Accessibility

Out of scope for v1 ([OC-05](02-architecture-constraints.md#23-organizational-constraints)).
