# ADR-0010 — Anti-rollback enforcement (`lower_limit` + monotonic version counter)

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Security Officer, Regulatory Lead
- **Related Requirements:** [FR-25](../requirements/functional.md#fr-25--anti-rollback-version-monotonicity), [FR-26](../requirements/functional.md#fr-26--lower_limit_version-gating), [NFR-16](../requirements/non-functional.md#nfr-16--tamper-resistant-rollback-state)
- **Related ADRs:** builds on [ADR-0003](ADR-0003-jws-ed25519-manifests.md), [ADR-0008](ADR-0008-config-driven-primitive-engine.md)
- **External inspiration:** Bambu Lab OTA package format (`ota_ver` + `lower_limit` fields in `ota-package-list.json`); TUF §5.2.1 timestamp metadata; Android Verified Boot rollback index.

## Context

Our current design ([ADR-0003](ADR-0003-jws-ed25519-manifests.md), §05.8 verification rules) rejects tampered or unsigned manifests and de-duplicates by `deployment_id`. It does **not** prevent a correctly-signed but **older** manifest from being accepted.

Threat scenarios this leaves open:

1. **Signed-but-old replay.** An attacker with access to the control plane (or the artifact store) re-publishes `v1.0.0` to a device currently running `v2.1.0` which has an unpatched `v1.0.0` CVE. The signature is valid; our current rules accept it.
2. **Accidental downgrade.** Release engineer re-pins an old manifest by mistake; the fleet regresses silently.
3. **Unstaged migration.** A device on `v1.5.0` jumps to `v3.0.0` when the release path assumed a mandatory intermediate `v2.0.0` (needed for schema migration, trust-store rotation, etc.).

The Bambu reference addresses (1) and (2) with a single `lower_limit` field in the signed root manifest and (3) with monotonic `ota_ver`. Android Verified Boot addresses (1) with a per-image rollback index burned into tamper-resistant storage. Both are cheap to implement; both deliver a large security / safety win.

## Decision

Introduce **three coupled mechanisms**, all driven from the signed manifest:

### 1. Monotonic manifest `desired_version` (semver)

Manifests carry a `desired_version` (already a field per §05.4.1). Elevate it from informational to **enforced**:

```json
{
  "schema_version": 1,
  "deployment_id": "…",
  "desired_version": "2.4.0",
  "lower_limit_version": "2.0.0",
  "allow_downgrade": false,
  "downgrade_justification": null,
  ...
}
```

### 2. `lower_limit_version` gating

The signed manifest asserts the **minimum version the device must currently be running** to accept this manifest. The agent compares to its persisted `current_deployed_version` and **rejects** the manifest (with an audit event) if `current < lower_limit_version`. This enables staged migrations and anti-rollback windows.

### 3. Rollback index (monotonic counter) in tamper-resistant storage

On successful deployment, the agent persists the installed `desired_version` as `max_seen_version` in **tamper-resistant storage** (see [NFR-16](../requirements/non-functional.md#nfr-16--tamper-resistant-rollback-state)), in this preference order:

1. **TPM 2.0 NV index** with `TPMA_NV_WRITE_STCLEAR` + policy binding to the agent's measured state (preferred; hardware-rooted).
2. **ATECC608 / secure element** one-way counter for devices without TPM.
3. **Journaled on-disk counter** (`/var/lib/ota-agent/rollback-index`) protected by SELinux (`ota_rollback_t`) and a hash-chained append-only journal, with detection of physical tampering called out in the risk register. Fallback only; requires explicit acceptance.

The agent refuses any manifest whose `desired_version < max_seen_version` unless:

- `allow_downgrade: true` is set in the manifest, **and**
- `downgrade_justification` is a non-empty string (captured in the audit log), **and**
- The manifest is signed by a **key with the `downgrade` capability** in the on-device trust store (distinct `kid` from normal release keys).

This ensures downgrade is a deliberate, auditable, differently-authorized action — not an accident.

### Persisted State Shape (agent)

```
/var/lib/ota-agent/rollback-state/
├── max_seen_version      # semver string, ASCII
├── max_seen_deployment   # deployment_id that installed max_seen_version
├── journal.jsonl         # append-only hash-chained; every update event recorded
└── journal.sig           # agent-key Ed25519 over the latest journal hash
```

On TPM-equipped devices, `max_seen_version` is mirrored to an NV index that the on-disk file is cross-checked against; divergence triggers an integrity alert.

### Verification Rule Additions (§05.8)

Two new rules appended:

- **Rule 10.** `semver(current_deployed_version) ≥ semver(lower_limit_version)` — else reject with `REJECTED_BELOW_LOWER_LIMIT`.
- **Rule 11.** `semver(desired_version) ≥ semver(max_seen_version)`, unless the manifest's `allow_downgrade == true`, `downgrade_justification` is present, and `kid` resolves to a key with the `downgrade` capability — else reject with `REJECTED_ROLLBACK`.

## Consequences

### Positive

- **Closes signed-replay attack vector.** A leaked-but-valid old manifest cannot regress a device.
- **Enforces staged migration.** Release engineering can guarantee the upgrade path (`1.x → 2.0 → 2.x → 3.0 → …`).
- **Auditable downgrade.** Emergency downgrades remain possible but require a specifically-authorized key and leave a bright trace in the audit log.
- **Compatible with offline bundles** ([ADR-0011](ADR-0011-offline-bundle-format.md)) — the manifest carries the version info; transport is irrelevant.
- **Cheap on resource-constrained devices.** The only on-device addition is ~50 LoC of semver comparison + one TPM NV write (or equivalent) per successful deployment.

### Negative

- **Bricking risk if version state is lost.** A corrupted rollback-state directory could lock a device to its current version forever. Mitigated by:
  - Journaling + hash chain so partial writes don't destroy the state.
  - A service-mode recovery path that requires physical presence + a one-time factory-signed recovery manifest.
- **TPM integration adds device provisioning cost.** Operators without TPM must accept the fallback journaled-file risk, documented in §11.
- **Requires a second signing-key role** (`release` vs `downgrade`) — process overhead, but a security best-practice regardless.

### Neutral

- No primitive changes; enforcement lives in the manifest verifier ([§05.3](../arc42/05-building-block-view.md#53-level-2--rust-edge-agent-whitebox)).
- Audit records gain a `rollback_decision` field per deployment event.

## Alternatives Considered

### A. `deployment_id` de-duplication only (status quo)
- **Pros:** Simple.
- **Cons:** Does not prevent re-publishing an old, differently-IDed manifest carrying old artifacts.
- **Verdict:** Rejected — does not address the threat.

### B. Monotonic counter only (no `lower_limit_version`)
- **Pros:** Simpler.
- **Cons:** Loses staged-migration capability; cannot express "must pass through `2.0.0`".
- **Verdict:** Rejected — `lower_limit_version` is cheap and valuable.

### C. `lower_limit_version` only (no rollback index)
- **Pros:** No tamper-resistant storage requirement.
- **Cons:** Does not protect against replay of a manifest signed *after* a vulnerable version was current. An attacker with a valid `v2.1.0` manifest (which set `lower_limit=2.0.0`) could replay it even after the device reached `v3.0.0`.
- **Verdict:** Rejected — pairing both is the canonical design.

### D. TUF-full (timestamp metadata + expiration)
- **Pros:** Industry-standard, addresses freshness too.
- **Cons:** Heavy metadata machinery; we would need timestamp/root/targets/snapshot roles; overkill for v1.
- **Verdict:** Deferred — ADR-0003 already covers the single-sign-root concern; revisit in v2 if we expand to a broader mirror-network deployment.

## Compliance Notes

- Anti-rollback is an explicit expectation in ISO 81001-5-1 for health-software update channels; this ADR is the design control that satisfies it.
- The distinct `downgrade` key role is recorded in the design history file; rotation procedure matches [ADR-0003](ADR-0003-jws-ed25519-manifests.md) TD-01.
- Every accept / reject / downgrade decision produces an audit record per §08.7; evidence bundles include the rollback index at deployment time.
