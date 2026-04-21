# ADR-0003 — Sign manifests with JWS Compact + Ed25519 (EdDSA)

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Security Officer
- **Related Requirements:** [FR-10](../requirements/functional.md#fr-10--cryptographic-verification-before-execution), [FR-11](../requirements/functional.md#fr-11--cryptographic-deployment-acknowledgment-from-uc-01), [NFR-01](../requirements/non-functional.md#nfr-01--traceability-for-medical-compliance), [OC-03](../arc42/02-architecture-constraints.md#23-organizational-constraints), [RC-03](../arc42/02-architecture-constraints.md#21-regulatory-constraints)
- **Related Use Cases:** [UC-01](../use-cases/UC-01-ab-ota-medical.md), [UC-02](../use-cases/UC-02-ros2-modular-deploy.md)

## Context

Every manifest the agent executes — and every artifact it flashes — must be cryptographically verified before any side effect. The verification primitive must be:

- Fast on ARM cores without crypto accelerators.
- Compact on the wire (we publish frequently; bandwidth on cellular matters).
- Free of well-known footguns (algorithm confusion, nonce reuse).
- Standardized so that key management and audit tooling can use off-the-shelf libraries.

Devices acknowledge successful deployments with a signed `DeploymentAck`; this signature must use the same primitive so the device-side library is reused for both verification and signing.

## Decision

Use **JWS Compact Serialization (RFC 7515)** with `alg=EdDSA` (Ed25519, RFC 8037) over a **UTF-8 JSON** payload.

- Manifests: `BASE64URL(header) || '.' || BASE64URL(utf8_json_payload) || '.' || BASE64URL(signature)`.
- Header carries `kid` (key ID) used to look up the verification key in the on-device trust store.
- Trust store is a local directory of PEM-encoded public keys; it is **never** populated from a URL at verification time.
- `DeploymentAck` is signed with the device's long-lived Ed25519 key (separate from the device mTLS cert).

### Why JSON (not Protobuf) as the JWS payload

Earlier drafts used Protobuf-encoded payloads inside the JWS. Updated per [ADR-0008](ADR-0008-config-driven-primitive-engine.md):

- **Human-debuggable** — operators can `base64url -d` a manifest and read it; auditors can review the exact signed text.
- **Self-describing** — no schema fetch required to decode captured manifests; tamper-evident at a glance.
- **Composable with the config-driven primitive model** ([ADR-0008](ADR-0008-config-driven-primitive-engine.md)): the manifest is a JSON object with a `deployment_steps` array whose entries are `{ primitive, parameters, … }`. Adding a new primitive is a schema update, not a Protobuf `oneof` change.
- **Deterministic canonicalization** is required before signing to avoid signature-vs-whitespace drift: manifests MUST be serialized in **JCS** (JSON Canonicalization Scheme, RFC 8785) prior to Base64URL + signing. The agent re-canonicalizes the decoded payload for verification.

**Protobuf remains the wire contract** for high-volume machine-to-machine messages (telemetry, step results, claim messages) per [ADR-0007](ADR-0007-protobuf-contracts.md); JWS-signed *manifests* are the exception because human reviewability and primitive schema evolution dominate for that artifact.

## Consequences

### Positive

- **Standardized envelope**: tooling exists in every major language; auditors recognize JWS.
- **Algorithm pinning**: the agent rejects any `alg` other than `EdDSA`, neutralizing the classic JWS algorithm-confusion attack.
- **Deterministic signatures**: Ed25519 signing is deterministic; no RNG → no catastrophic nonce reuse.
- **Compact**: 64-byte signatures and 32-byte public keys keep on-the-wire and on-disk footprint small.
- **Verifies fast** on ARM Cortex-A class cores without hardware crypto.
- **Local trust store** removes signature-injection-via-network attack class entirely.

### Negative

- **`kid` rotation procedure** must be solved separately (out-of-band signed update of the trust store; a future `UpdateTrustStore` step type is planned).
- **No HSM in v1** for manifest signing keys — keys live on filesystem with strict ACLs and audit logging. Acceptable risk for v1, scheduled for v2.
- **JSON canonicalization discipline** (JCS) is required; casual producers may generate payloads with different whitespace/key ordering and break signatures. Mitigated by publishing a reference signer library.
- **JSON is larger on the wire** than Protobuf; manifests are low-frequency so this is acceptable (telemetry stays Protobuf).

### Neutral

- The audit hash chain ([§8.7](../arc42/08-crosscutting-concepts.md#871-audit-record-shape)) uses SHA-256 independent of the signature scheme.

## Alternatives Considered

### A. RSA-2048 / RSA-PSS
- **Pros:** Ubiquitous; well-supported.
- **Cons:** 256-byte signatures (4× larger); signing/verification considerably slower on ARM; key generation slow; padding schemes are footgun-prone.
- **Verdict:** Rejected.

### B. ECDSA P-256
- **Pros:** Smaller than RSA; standard.
- **Cons:** Non-deterministic by spec; bad RNG on a constrained device → catastrophic key compromise. Deterministic ECDSA (RFC 6979) helps but is less universally supported in JOSE libraries.
- **Verdict:** Rejected in favour of Ed25519.

### C. Sigstore / cosign-style transparency log
- **Pros:** Public auditability; emerging industry standard.
- **Cons:** Requires Internet access at verification time (the device cannot reach Rekor/Fulcio over a clinical network); operationally heavier; introduces a third-party dependency. We may add cosign verification of the underlying *artifacts* later as an optional layer, but the manifest envelope itself stays JWS.
- **Verdict:** Rejected for v1 manifest signing; revisit for artifact signing in v2.

### D. SSH signatures (`ssh-keygen -Y sign`)
- **Pros:** Familiar to operators; keys often already exist.
- **Cons:** No standard wire envelope; tooling for JOSE-style verification is sparse; weaker audit story.
- **Verdict:** Rejected.

## Compliance Notes

- ISO 81001-5-1 expects update integrity to use cryptographic signatures with documented key custody. JWS + Ed25519 + filesystem ACL on signing keys + audit logging on key access is the v1 minimum; HSM is v2.
- The trust-store-update procedure must itself be a signed operation; otherwise the trust anchor can be replaced silently. Captured as planned step type `UpdateTrustStore` (not yet specified beyond name).
