# ADR-0011 — Offline / air-gapped update bundle format

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Manufacturing/Field Ops, Security Officer
- **Related Requirements:** [FR-27](../requirements/functional.md#fr-27--offline--air-gapped-bundle-mode), [FR-29](../requirements/functional.md#fr-29--manifest-level-artifact-pin-index), [NFR-06](../requirements/non-functional.md#nfr-06--robot-autonomy-during-cloud-outage-from-uc-02), [NFR-14](../requirements/non-functional.md#nfr-14--cloud-agnostic-deployment)
- **Related ADRs:** composes with [ADR-0003](ADR-0003-jws-ed25519-manifests.md), [ADR-0008](ADR-0008-config-driven-primitive-engine.md), [ADR-0010](ADR-0010-anti-rollback-enforcement.md)
- **External inspiration:** Bambu Lab offline OTA zip (single `ota-package-list.json.sig` root manifest that pins every sibling file by SHA-256; artifacts delivered from `http://127.0.0.1/…` local mirror).

## Context

The v1 design assumes every device reaches the control plane via NATS (directly or via a Leaf Node). Several realistic deployment contexts break this assumption:

- **Medical manufacturing lines** where devices are provisioned and updated in an air-gapped factory cell before shipment.
- **HIL labs / cleanrooms** behind one-way data diodes that forbid outbound device traffic.
- **Field service** of robots at remote sites with intermittent or no WAN, where a technician arrives with a USB stick.
- **Regulatory environments** (defence, critical infrastructure) that mandate offline-only updates for a class of devices.

The current design cannot serve these contexts; operators would have to build bespoke tooling each time.

The Bambu reference handles this elegantly: a single zip with a **signed root manifest** that pins every artifact by strong hash; the on-device agent is handed either a local path or a `http://127.0.0.1/…` URL and the rest of the flow is identical to an online update.

## Decision

Define an **Offline Update Bundle** as a first-class delivery channel, co-equal with the online NATS flow. The same signed manifest schema ([§05.4.1](../arc42/05-building-block-view.md#541-manifest--json-schema-sketch-jws-payload)) and the same verification rules apply; only the **transport** differs.

### Bundle Format

A deterministic `.zip` archive:

```
device-update-v2.4.0-20261021T134502Z.zip
├── manifest.jws                       ← JWS-signed manifest (JCS JSON inside)
├── artifacts/
│   ├── root-v2.4.img                  ← OS image
│   ├── root-v2.4.vmdk                 ← direct hypervisor disk image
│   ├── flash-inactive-partition.sh    ← device-profile script
│   ├── docker-compose.yml             ← compose stack descriptor
│   ├── app_2.4.0_amd64.deb            ← offline apt package payload (direct install or local repo member)
│   ├── Packages.gz                    ← apt package index for local repo snapshot mode
│   ├── InRelease                      ← apt repo metadata + signature for local repo snapshot mode
│   ├── Release                        ← apt repo metadata for local repo snapshot mode
│   ├── update-grubenv.sh
│   ├── restore-grubenv.sh
│   └── <one file per manifest step_ref>
└── BUNDLE.toml                        ← unsigned metadata (created_at, tool_version, notes)
```

Properties:

- **`manifest.jws` is the single root of trust.** It contains the top-level `artifacts[]` pin index ([FR-29](../requirements/functional.md#fr-29--manifest-level-artifact-pin-index)) — every file under `artifacts/` is listed with its SHA-256.
- Offline APT content may be packaged either as direct `.deb` artifacts or as a local repo snapshot. If using the local repo snapshot flow, include the usual apt metadata set (`Packages*` plus `Release`/`InRelease`) in the bundle and pin those files in `manifest.jws` like any other artifact.
- Verifying the bundle = verifying the JWS + SHA-256 of every file against the pin index. One RSA-style signature check, N cheap hash checks. Matches the Bambu pattern.
- **No artifact encryption in v1.** Confidentiality, if required, is delegated to bundle-level transport (e.g., encrypted USB drive); see §11 tech-debt for a post-v1 proposal to add AEAD-at-rest per artifact.
- Filenames include both semantic version and a UTC build timestamp (`v2.4.0-20261021T134502Z`) to reject out-of-order releases without trusting filesystem clocks.
- `BUNDLE.toml` is unsigned human-readable metadata (build host, CI run URL, release notes). It is **not** consulted during verification.
- Bundles are intentionally **artifact-agnostic**: the signed manifest may reference raw/Yocto/VMDK disk images, device-profile scripts, compose descriptors, OCI-related metadata, or offline Debian/Ubuntu package sets (`.deb` plus apt index files), provided the referenced steps consume them through the existing primitive set.

### `bundle://` Transport Scheme

Extend the manifest's step parameters to accept `bundle://` URLs in addition to `https://`:

```json
{ "primitive": "FILE_TRANSFER",
  "parameters": {
    "url": "bundle://artifacts/root-v2.4.img",
    "sha256": "ab12…",
    "dest_path": "/var/lib/ota/spool/root-v2.4.img"
  }
}
```

When evaluating a `bundle://` URL, the `FILE_TRANSFER` primitive reads the artifact from the active bundle's in-memory index instead of opening an HTTPS connection. Online manifests use `https://`; offline manifests use `bundle://`; a manifest may mix both (hybrid mirror).

### Delivery Modes on Device

The agent supports three discovery mechanisms, any of which can feed the same execution engine:

1. **`/var/lib/ota/bundles/` drop-directory watch.** A technician mounts a USB stick or `scp`s a bundle; an inotify watcher in the agent picks it up, verifies, and (subject to anti-rollback per [ADR-0010](ADR-0010-anti-rollback-enforcement.md)) executes. Directory labeled `ota_bundle_t` in SELinux.
2. **Explicit local CLI invocation.** `ota-agent apply-bundle /path/to/update.zip` — requires `CAP_OTA_APPLY` group membership; used for supervised manual updates.
3. **NATS-directed bundle reference.** The control plane publishes a `desired-state` referencing `bundle:///path/to/update.zip`; useful when an operator has staged the bundle on the device out-of-band but wants the cloud to authorize the apply.

All three paths end at the same manifest verifier and execution engine; there is no parallel "offline-specific" code path.

### Control-plane tooling

Bundles are produced by the existing manifest pipeline with a new output flag: the same JSON manifest that would have been published to NATS, plus all referenced artifacts, zipped with a deterministic zipper (fixed timestamp 1980-01-01, sorted entries) so the same inputs always produce the bit-identical bundle. Bundle SHA-256 is itself recorded in the release ledger for traceability.

## Consequences

### Positive

- **Enables air-gapped deployment contexts** without forking the update mechanism.
- **Same verification surface.** No second signature format, no second trust-store story — the JWS manifest is the root of trust in both modes.
- **Reproducible bundles.** Deterministic zipping means any CI run can reproduce a bundle bit-for-bit, which is a quality-gate tool (attested builds, supply-chain audit).
- **Aligns with offline medical-device manufacturing** (UC-01 variant) and field-service robot updates (UC-02 variant).
- **Compatible with anti-rollback** ([ADR-0010](ADR-0010-anti-rollback-enforcement.md)) — `lower_limit_version` and `max_seen_version` work identically offline.

### Negative

- **Physical-access trust.** A malicious USB stick with a valid-signed old bundle is still caught by anti-rollback — **but only if the signing key is not compromised**. Compromise scenarios in this channel are more reachable than in NATS (no network-layer mTLS to filter). Mitigations: mandatory operator authentication on mode (2); CLI apply requires a challenge signed by a per-site operator key; bundles MAY require a second co-signature ("factory-signed" for class-A bundles) — tracked in §11.
- **Determinism discipline.** Any new step that references a new file must add it to `artifacts[]`. Mitigated by: manifest linter that walks every step param, collects `url`s beginning with `bundle://`, asserts all appear in `artifacts[]`; CI hard fail on drift.
- **Storage bulk on devices.** Bundles can be 100s of MB; the staging directory quota must be sized per device profile.

### Neutral

- Bundle verification shares 100% of the online verification logic; changes to JWS/JCS rules need update in only one place.
- The unsigned `BUNDLE.toml` is a pure observability aid; the system never branches on its contents.

## Alternatives Considered

### A. Do nothing — document a manual "operator procedure"
- **Pros:** No engineering cost.
- **Cons:** Every operator reinvents a fragile tooling stack; traceability is inconsistent; no audit story.
- **Verdict:** Rejected.

### B. Encrypted bundle format with per-device key wrap (à la AEAD + AES-KW)
- **Pros:** Confidentiality at rest; matches Bambu's encrypted payload direction.
- **Cons:** Adds a device-key-management dimension to every bundle; payload keys must be wrapped per-device; significant v1 effort.
- **Verdict:** Deferred. Signed-but-plaintext is equivalent to our current NATS-over-TLS posture (we already trust the device with plaintext post-decryption); confidentiality at rest is a valuable v2 addition.

### C. A proprietary binary container (BIMH-style) instead of a zip + JWS
- **Pros:** Single contiguous file; no container parsing dependency.
- **Cons:** Duplicates JWS in a new format; not human-inspectable; against our "auditor-readable manifests" posture ([ADR-0003](ADR-0003-jws-ed25519-manifests.md)).
- **Verdict:** Rejected.

### D. Tar instead of zip
- **Pros:** Streamable without a central directory.
- **Cons:** No deterministic tooling by default; harder to produce reproducible archives.
- **Verdict:** Rejected — deterministic zip producers are common; the streamability advantage doesn't apply (we verify the manifest first, then random-access lookups).

## Compliance Notes

- Offline deployment events are first-class audit records, including the bundle SHA-256 and the mode of application (drop-dir / CLI / cloud-directed).
- The bundle format is a design history file artifact (versioned; changes require ADR update).
- Deterministic bundles support attested-build / SBOM workflows required by SBOM-mandating regulations.
