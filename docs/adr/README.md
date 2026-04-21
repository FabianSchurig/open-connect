# Architectural Decision Records (ADRs)

This directory holds the architectural decisions that shape the platform. Each ADR follows a lightweight Context / Decision / Consequences / Alternatives format ([Michael Nygard style](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)).

## Statuses

- **Proposed** — drafted, awaiting decision-maker approval.
- **Accepted** — approved; implementation may proceed.
- **Deprecated** — no longer in force, kept for historical context.
- **Superseded by ADR-XXXX** — replaced by another ADR.

> All ADRs in this initial pass are **Proposed** pending review. They become **Accepted** when the foundation document set is signed off.

## Index

| ID | Title | Status |
|----|-------|--------|
| [ADR-0001](ADR-0001-nats-over-http-rest.md) | Use NATS (with Leaf Nodes) instead of HTTP polling for device traffic | Proposed |
| [ADR-0002](ADR-0002-rust-for-edge-agent.md) | Implement the edge agent in Rust (single statically linked musl binary) | Proposed |
| [ADR-0003](ADR-0003-jws-ed25519-manifests.md) | Sign manifests with JWS Compact + Ed25519 (EdDSA) | Proposed |
| [ADR-0004](ADR-0004-ab-partitioning-grubenv.md) | ext4 A/B partitioning with GRUB `grubenv` boot-counter (Btrfs snapshot variant) | Proposed |
| [ADR-0005](ADR-0005-pull-based-claim-model.md) | Pull-based asynchronous Claim Registry with optimistic concurrency | Proposed |
| [ADR-0006](ADR-0006-selinux-strict-policy.md) | SELinux strict Type Enforcement policy with `type_transition` and no `execmem` | Proposed |
| [ADR-0007](ADR-0007-protobuf-contracts.md) | Use Protobuf as the canonical wire contract language for machine messages | Proposed |
| [ADR-0008](ADR-0008-config-driven-primitive-engine.md) | Config-driven primitive execution engine ("dumb agent" principle) | Proposed |
| [ADR-0009](ADR-0009-nats-nkey-authentication.md) | Two-factor NATS authentication: mTLS + NATS NKey | Proposed |
| [ADR-0010](ADR-0010-anti-rollback-enforcement.md) | Anti-rollback enforcement (`lower_limit` + monotonic version counter) | Proposed |
| [ADR-0011](ADR-0011-offline-bundle-format.md) | Offline / air-gapped update bundle format (`bundle://` transport) | Proposed |

## Authoring Rules

- Never modify an Accepted ADR in place. To change a decision, write a new ADR that supersedes the old one.
- Each ADR cites the FRs/NFRs it satisfies and the arc42 sections that depend on it.
