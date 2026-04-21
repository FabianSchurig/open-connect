# ADR-0009 — NATS NKey authentication (complementing mTLS)

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Security Officer
- **Related Requirements:** [NFR-11](../requirements/non-functional.md#nfr-11--all-transport-authenticated-from-fr-08-fr-10), [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance)
- **Related ADRs:** [ADR-0001](ADR-0001-nats-over-http-rest.md)

## Context

[ADR-0001](ADR-0001-nats-over-http-rest.md) mandates **mTLS** on every NATS connection. mTLS authenticates that the peer holds a private key corresponding to a certificate issued by a trusted CA — an **identity tied to the PKI trust chain**.

However, our Zero-Trust posture would benefit from a second, orthogonal authentication factor at the NATS protocol layer, specifically one that:

- Does **not** rely on a shared CA (decentralized).
- Binds NATS-level permissions (subject ACLs) to a **per-entity Ed25519 key**, not a cert DN.
- Survives CA compromise — a breached CA cannot alone authorize a device to publish on its subjects.
- Composes cleanly with JWS-signed manifests (same Ed25519 curve, same signing libraries).

NATS provides **NKeys** (Ed25519 public-key-based authentication, sometimes paired with signed JWT claims for decentralized auth).

## Decision

Require **both mTLS and NATS NKey authentication** on every NATS connection:

1. **Transport layer (mTLS):** encrypts traffic, provides first-factor identity via X.509.
2. **Protocol layer (NKey):** second-factor identity where the client signs a NATS-provided nonce with its Ed25519 NKey; server verifies against a pinned public NKey per principal.
3. **Authorization:** subject ACLs are bound to the **NKey public key**, not the TLS cert DN. This means an attacker who compromises the CA but not the NKey private key cannot authorize traffic.
4. **JWT-signed account claims** (decentralized NATS auth) are adopted for the hub deployment: the Operator JWT signs Account JWTs; Account JWTs sign User JWTs; each User JWT pins an NKey and its permitted subject set. The hub is configured in **resolver = URL / nats-account-server** mode so accounts can be revoked without restart.

### Identity Model

| Principal | mTLS Identity | NKey | JWT Account |
|-----------|---------------|------|-------------|
| Device | Per-device short-lived cert (≤ 30d, cert-manager-like rotation) | Long-lived Ed25519 NKey (bootstrapped at provisioning, same curve as device signing key in FR-11) | `Fleet.Devices` account, subject ACL `device.<serial>.>` |
| Control plane pod | Pod cert from cert-manager | Ed25519 NKey injected via sealed secret | `Fleet.ControlPlane` account |
| NATS Leaf Node | Site cert | Site NKey (per-site) | `Fleet.Leaves.<site>` account |

### Revocation

- **Cert revocation** via CRL / OCSP.
- **NKey revocation** via JWT revocation list pushed by the Operator; effective immediately across the cluster.
- A compromise requires **both** to be reissued.

## Consequences

### Positive

- **Defence in depth** at two independent authentication layers. Neither a CA compromise nor an NKey leak alone grants subject access.
- **Decentralized trust model** — the NATS cluster does not require a central identity provider. Operator → Account → User JWT chains are self-contained and verifiable offline.
- **Same curve everywhere** (Ed25519 — [ADR-0003](ADR-0003-jws-ed25519-manifests.md)): one crypto library, one key management story, one set of HSM-ready interfaces for v2.
- **Precise authorization** at the NATS subject level, decoupled from the cert identity lifecycle.
- **Fine-grained revocation** without regenerating certificates.

### Negative

- **Two identity systems to operate** (X.509 PKI and NKey/JWT). Automation is required so operators do not drift.
- **Bootstrapping complexity** at device provisioning — the NKey must be generated on-device (or provisioned securely) and its public key registered with the control plane alongside the device's signing key. Mitigated by a single provisioning flow that registers both.
- **Learning curve** for SREs unfamiliar with NATS decentralized auth.

### Neutral

- Existing ACL design ([§5.5](../arc42/05-building-block-view.md#55-detailed-design--nats-subject-hierarchy)) is unchanged; only the *identity* backing the ACL shifts from DN to NKey.
- Pipeline → REST API remains mTLS + JWT bearer; NKey is NATS-specific.

## Alternatives Considered

### A. mTLS only
- **Pros:** Simpler operationally.
- **Cons:** Single identity factor; CA compromise is a single point of failure; doesn't align with Zero-Trust best practice for the threat model.
- **Verdict:** Rejected.

### B. Static username/password or token-based NATS auth
- **Pros:** Simplest configuration.
- **Cons:** Shared secrets leak; no forward secrecy; incompatible with our Zero-Trust stance.
- **Verdict:** Rejected.

### C. OAuth2/OIDC-federated NATS auth
- **Pros:** Leverages existing IdPs.
- **Cons:** Introduces an IdP dependency on the device path (brittle under WAN outage); token lifetime management at the edge is awkward; contradicts [NFR-06](../requirements/non-functional.md#nfr-06--robot-autonomy-during-cloud-outage-from-uc-02) local-first operation.
- **Verdict:** Rejected for device connections (fine for human operators on REST).

## Compliance Notes

- Two-factor authentication of transport/protocol aligns with ISO 81001-5-1 "defence in depth" expectations for health-software communications.
- NKey generation on-device must use the OS CSPRNG; audited.
- Operator / Account JWT signing keys follow the same custody policy as manifest signing keys ([ADR-0003](ADR-0003-jws-ed25519-manifests.md) TD-01): filesystem with strict ACLs in v1, HSM in v2.
