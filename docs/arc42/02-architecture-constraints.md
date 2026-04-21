# 2. Architecture Constraints

Constraints are non-negotiable inputs to the architecture. They cannot be traded off through ADRs; they bound the solution space.

## 2.1 Regulatory Constraints

| ID | Constraint | Impact |
|----|------------|--------|
| RC-01 | **IEC 62304** — Medical device software lifecycle. | Mandates documented requirements, traceability, risk management, configuration management. Drives [NFR-01](../requirements/non-functional.md#nfr-01--traceability-for-medical-compliance), [FR-12](../requirements/functional.md#fr-12--append-only-audit-storage-from-uc-01). |
| RC-02 | **ISO 13485** — Medical device QMS. | Mandates controlled change records and design history files; audit log must satisfy. |
| RC-03 | **ISO 81001-5-1** — Health software security activities. | Mandates secure-by-design, signed artifacts, and update integrity. Drives [FR-10](../requirements/functional.md#fr-10--cryptographic-verification-before-execution), [NFR-11](../requirements/non-functional.md#nfr-11--all-transport-authenticated-from-fr-08-fr-10), [NFR-12](../requirements/non-functional.md#nfr-12--selinux-strict-confinement-from-brief-deliverable-51). |
| RC-04 | **GDPR / patient-data adjacency** | Telemetry payloads must not include patient-identifying fields; audit logs must support data subject inquiries. |

## 2.2 Technical Constraints

| ID | Constraint | Impact |
|----|------------|--------|
| TC-01 | **Cloud-agnostic deployment.** | Control plane runs on any CNCF Kubernetes; no managed cloud-vendor services. → [NFR-14](../requirements/non-functional.md#nfr-14--cloud-agnostic-deployment). |
| TC-02 | **No inbound network ports on devices.** | Pull-only communication; eliminates NAT/firewall negotiation. → [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance). |
| TC-03 | **ARM and x86 hardware targets.** | Mandates cross-compilation matrix (`aarch64-musl`, `x86_64-musl`). → [NFR-03](../requirements/non-functional.md#nfr-03--single-statically-linked-binary-arm--x86). |
| TC-04 | **Linux base userspace only.** | The agent may rely on `bash`, `coreutils`, `systemd`, `grub2`, `selinux-policy-targeted`. No language runtimes (no Python, no JVM). |
| TC-05 | **Single-binary distribution.** | The agent ships as one statically linked executable; distribution-package-managed configuration is allowed but not required. |
| TC-06 | **GRUB as the bootloader.** | Decision predates this architecture; ext4 A/B partitioning is built around `grubenv` semantics. → [FR-05](../requirements/functional.md#fr-05--grubenv-boot-toggling), [FR-06](../requirements/functional.md#fr-06--automated-bootloader-fallback). |

## 2.3 Organizational Constraints

| ID | Constraint | Impact |
|----|------------|--------|
| OC-01 | **Stack: Go (control plane), Rust (edge agent).** | Fixed; not subject to ADR. |
| OC-02 | **Protobuf as the wire contract language.** | All NATS messages and inter-service contracts are Protobuf. → [ADR-0007](../adr/ADR-0007-protobuf-contracts.md). |
| OC-03 | **JWS/EdDSA (Ed25519) for manifest signing.** | Curve choice fixed by security review; key custody is out of scope for v1 (HSM in v2). → [ADR-0003](../adr/ADR-0003-jws-ed25519-manifests.md). |
| OC-04 | **NATS as the messaging fabric.** | Chosen for Leaf Node topology; not subject to ADR re-litigation. → [ADR-0001](../adr/ADR-0001-nats-over-http-rest.md). |
| OC-05 | **English-only documentation in this pass.** | Localization is post-v1. |
| OC-06 | **`unsafe` Rust banned outside reviewed FFI shims.** | Enforced by CI lint. → [NFR-02](../requirements/non-functional.md#nfr-02--rust-for-memory-safety). |

## 2.4 Convention Constraints

- **Semantic versioning** for all artifacts (agent binary, control plane image, manifest schema).
- **Protobuf forward/backward compatibility** rules: never re-use field numbers, never change field types, additions only.
- **Conventional Commits** in the implementation phase.
- **Time is UTC**, durations in ISO-8601, all log entries include monotonic + wall clock.
