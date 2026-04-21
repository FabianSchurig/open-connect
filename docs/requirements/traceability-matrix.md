# Traceability Matrix

This matrix is the single source of truth for cross-references between **Use Cases**, **Requirements**, **arc42 sections**, **Project Increments / Epics / Tasks** (from the project brief), and **ADRs**.

It is the artifact a regulator or auditor inspects to verify that every requirement is sourced from a stakeholder need and lands in a concrete part of the architecture.

## Conventions

- `UC-NN` — [Use Case](../use-cases/) `NN`.
- `FR-NN`, `NFR-NN` — [Functional](functional.md) / [Non-Functional](non-functional.md) requirement.
- `§N` — arc42 section `N` (under [`../arc42/`](../arc42/)).
- `ADR-NNNN` — [Architectural Decision Record](../adr/).
- `PI N / D N.M` — Program Increment N / Deliverable N.M, from the project brief.
- `Epic N / T N.M` — Epic N / Task N.M, from the project brief.

---

## 1. Use Case → Requirements

| Use Case | Functional Requirements | Non-Functional Requirements |
|----------|-------------------------|------------------------------|
| [UC-01](../use-cases/UC-01-ab-ota-medical.md) | FR-01, FR-02, FR-03, FR-04, FR-05, FR-06, FR-10, FR-11, FR-12, FR-18, FR-21, FR-24, FR-25, FR-26, FR-27, FR-29, FR-30 | NFR-01, NFR-02, NFR-04, NFR-05, NFR-11, NFR-13, NFR-15, NFR-16 |
| [UC-02](../use-cases/UC-02-ros2-modular-deploy.md) | FR-01, FR-02, FR-10, FR-13, FR-14, FR-18, FR-21, FR-23, FR-24, FR-25, FR-28, FR-29, FR-30 | NFR-01, NFR-02, NFR-04, NFR-06, NFR-07, NFR-11, NFR-15, NFR-16 |
| [UC-03](../use-cases/UC-03-cicd-hil-claiming.md) | FR-07, FR-08, FR-09, FR-10, FR-15, FR-16, FR-17, FR-22, FR-24, FR-25, FR-27, FR-29 | NFR-01, NFR-04, NFR-08, NFR-09, NFR-11, NFR-15, NFR-16 |

**Coverage check.** Every UC produces ≥ 4 FRs and ≥ 3 NFRs.

---

## 2. Requirement → arc42 Section, ADR, Epic/Task, Quality Scenario

### 2.1 Functional Requirements

| FR | Source UC(s) | arc42 Home | ADR(s) | PI / Deliverable | Epic / Task |
|----|--------------|------------|--------|------------------|-------------|
| FR-01 | UC-01, UC-02 | §05.3, §05.4, §08.6 | ADR-0007, ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.1, T 1.3 |
| FR-02 | UC-01, UC-02 | §05.3, §08.6 | ADR-0007, ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.4 |
| FR-03 | UC-01 | §05.9, §07.4 | ADR-0004 | PI 3 / D 3.2 | Epic 2 / T 2.1 |
| FR-04 | UC-01 | §05.9, §08.1.4 | ADR-0004, ADR-0006, ADR-0008 | PI 3 / D 3.2 | Epic 2 / T 2.2 |
| FR-05 | UC-01 | §05.9, §08.6.3 | ADR-0004, ADR-0008 | PI 3 / D 3.3 | Epic 2 / T 2.3 |
| FR-06 | UC-01 | §06.2, §08.6.3 | ADR-0004 | PI 3 / D 3.3 | Epic 2 / T 2.4 |
| FR-07 | UC-03 | §05.6, §06.4 | ADR-0005 | PI 4 / D 4.1 | Epic 3 / T 3.1, T 3.2 |
| FR-08 | UC-03 | §05.5, §06.4 | ADR-0001, ADR-0005, ADR-0009 | PI 2 / D 2.1, PI 4 / D 4.2 | Epic 3 / T 3.4 |
| FR-09 | UC-03 | §05.6, §06.4 | ADR-0005 | PI 4 / D 4.1 | Epic 3 / T 3.3 |
| FR-10 | UC-01, UC-02, UC-03 | §05.8, §08.1.5 | ADR-0003 | PI 2 / D 2.2 | Epic 4 / T 4.1 |
| FR-11 | UC-01 | §05.4, §08.7.1 | ADR-0003 | PI 5 / D 5.2 | Epic 4 / T 4.2 |
| FR-12 | UC-01 | §08.7.2 | — | PI 5 / D 5.2 | Epic 4 / T 4.2 |
| FR-13 | UC-02 | §05.3, §05.4 | ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.1, T 1.3 |
| FR-14 | UC-02 | §05.4, §08.6.2 | ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.3 |
| FR-15 | UC-03 | §05.7.2, §06.4 | ADR-0005 | PI 4 / D 4.1 | Epic 3 / T 3.1 |
| FR-16 | UC-03 | §08.1.3 | ADR-0005 | PI 5 / D 5.2 | Epic 4 / T 4.3 |
| FR-17 | UC-03 | §05.6, §06.4 | ADR-0005 | PI 4 / D 4.1 | Epic 3 / T 3.3 |
| FR-18 | UC-01, UC-02 | §05.4, §08.2.1 | ADR-0007 | PI 2 / D 2.3 | Epic 1 / T 1.3 |
| FR-19 | UC-01 (self-update flow) | §06.5, §08.6.4 | ADR-0002, ADR-0003, ADR-0008 | PI 5 / D 5.3 | (cross-cutting) |
| FR-20 | UC-01 (Btrfs variant) | §05.9, §07.4 | ADR-0004, ADR-0008 | PI 3 / D 3.2 | Epic 2 / T 2.2 |
| FR-21 | UC-01, UC-02 | §05.3, §08.2.2 | — | PI 2 / D 2.3, PI 3 / D 3.1 | Epic 1 / T 1.3 |
| FR-22 | UC-03 (registration prerequisite) | §05.2, §05.6 | — | PI 1 / D 1.3 | (cross-cutting) |
| FR-23 | UC-02 | §05.3, §05.4, §08.6.2 | ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.3 |
| FR-24 | UC-01, UC-02, UC-03 | §04.1, §05.3, §05.4 | ADR-0002, ADR-0008 | PI 3 / D 3.1 | Epic 1 / T 1.3 |
| FR-25 | UC-01, UC-02, UC-03 | §05.4.1, §05.8, §08.1.6 | ADR-0010 | PI 5 / D 5.2 | Epic 4 / T 4.1 |
| FR-26 | UC-01, UC-03 | §05.4.1, §05.8, §08.1.6 | ADR-0010 | PI 5 / D 5.2 | Epic 4 / T 4.1 |
| FR-27 | UC-01 (manufacturing), UC-03 (air-gapped HIL) | §05.4, §05.10, §08.4.1 | ADR-0011 | PI 4 / D 4.3 | (cross-cutting) |
| FR-28 | UC-02 | §05.4.1, §08.4 | ADR-0008, ADR-0011 | PI 3 / D 3.1 | Epic 1 / T 1.3 |
| FR-29 | UC-01, UC-02, UC-03 | §05.4.1, §05.8 | ADR-0011 | PI 2 / D 2.2 | Epic 4 / T 4.1 |
| FR-30 | UC-01, UC-02 | §08.6.5, §05.9 | ADR-0004, ADR-0008 | PI 3 / D 3.1, PI 3 / D 3.2 | Epic 2 / T 2.2, T 2.3 |

**Coverage check.** Every FR has at least one UC source and at least one arc42 home. ADR coverage is provided where a load-bearing decision exists; FRs without an ADR are implementation-detail level.

### 2.2 Non-Functional Requirements

| NFR | Source UC(s) / Brief | arc42 Home | ADR(s) | Quality Scenario | PI / Deliverable |
|-----|----------------------|------------|--------|-------------------|------------------|
| NFR-01 | UC-01, UC-02, UC-03 + Brief §3 | §08.7 | ADR-0003 | [QS-01](../arc42/10-quality-requirements.md#qs-01-traceability-end-to-end) | PI 5 / D 5.2 |
| NFR-02 | Brief §3 | §08.1.4, §05.3 | ADR-0002 | [QS-02](../arc42/10-quality-requirements.md#qs-02-memory-safety-enforcement) | PI 2 / D 2.1 |
| NFR-03 | Brief §3 | §07.6 | ADR-0002 | [QS-03](../arc42/10-quality-requirements.md#qs-03-cross-compilation-and-static-linkage) | PI 2 / D 2.1 |
| NFR-04 | UC-01 Alt-1, UC-02 Alt-1, Brief §3 | §08.3.1, §06.7 | ADR-0001 | [QS-04](../arc42/10-quality-requirements.md#qs-04-network-partition-recovery) | PI 1 / D 1.1 |
| NFR-05 | UC-01 | §06.2, §08.6.3 | ADR-0004 | [QS-05](../arc42/10-quality-requirements.md#qs-05-health-check-window) | PI 3 / D 3.3 |
| NFR-06 | UC-02 | §08.3.2, §07.3 | ADR-0001 | [QS-06](../arc42/10-quality-requirements.md#qs-06-robot-autonomy-during-cloud-outage) | PI 4 / D 4.3 |
| NFR-07 | UC-02 | §08.3, §06.3 | — | [QS-07](../arc42/10-quality-requirements.md#qs-07-resumable-artifact-download) | PI 3 / D 3.1 |
| NFR-08 | UC-03 | §07.2, §08.3 | ADR-0005 | [QS-08](../arc42/10-quality-requirements.md#qs-08-bounded-force-release-latency) | PI 4 / D 4.1 |
| NFR-09 | UC-03 | §05.6, §06.4 | ADR-0005 | [QS-09](../arc42/10-quality-requirements.md#qs-09-linearizable-claim-locks) | PI 4 / D 4.1 |
| NFR-10 | Brief §3 (NFR-03 walkthrough) | §07.6 | ADR-0002 | [QS-10](../arc42/10-quality-requirements.md#qs-10-agent-binary-size-budget) | PI 2 / D 2.1 |
| NFR-11 | UC-01 Err-1, UC-03 Err-1, Brief §1 | §08.1.1 | ADR-0001, ADR-0003, ADR-0009 | [QS-11](../arc42/10-quality-requirements.md#qs-11-authenticated-transport) | PI 1 / D 1.1, D 1.3 |
| NFR-12 | UC-01 (defence in depth), Brief §5 | §08.1.4 | ADR-0006, ADR-0008 | [QS-12](../arc42/10-quality-requirements.md#qs-12-selinux-confinement) | PI 5 / D 5.1 |
| NFR-13 | UC-01, NFR-01 | §08.7.2 | — | [QS-01](../arc42/10-quality-requirements.md#qs-01-traceability-end-to-end) | PI 5 / D 5.2 |
| NFR-14 | Brief PI 1 Goal | §07.1 | — | [QS-14](../arc42/10-quality-requirements.md#qs-14-cloud-agnostic-deployment) | PI 1 / D 1.3 |
| NFR-15 | UC-02 + Brief (ADR-0008 rationale) | §04.1, §07.6 | ADR-0002, ADR-0008 | (QS-15 to be added) | PI 2 / D 2.1, PI 3 / D 3.1 |
| NFR-16 | UC-01 (medical-device class) | §08.1.6, §05.8 | ADR-0010 | (QS-16 to be added) | PI 5 / D 5.1 |

**Coverage check.** Every NFR has a QS, an arc42 home, and a PI/Deliverable. NFRs without an ADR are constraints derived directly from the brief.

---

## 3. Epic / Task → Requirements (Reverse Lookup)

| Epic / Task (Brief) | Requirements satisfied |
|---------------------|------------------------|
| Epic 1 / T 1.1 — Protobuf for modular steps | FR-01, FR-13, FR-18 |
| Epic 1 / T 1.2 — Rust NATS client polling | FR-08 (cooperatively), FR-18 |
| Epic 1 / T 1.3 — Execution engine + step output capture | FR-01, FR-13, FR-14, FR-18, FR-21 |
| Epic 1 / T 1.4 — Step failure + rollback | FR-02 |
| Epic 2 / T 2.1 — Disk layout | FR-03 |
| Epic 2 / T 2.2 — Active/inactive partition flashing & verify | FR-04, FR-20 |
| Epic 2 / T 2.3 — `grubenv` modification | FR-05 |
| Epic 2 / T 2.4 — Boot-counter fallback script | FR-06, NFR-05 |
| Epic 3 / T 3.1 — Claim Registry concurrency | FR-07, FR-15, NFR-08, NFR-09 |
| Epic 3 / T 3.2 — Pipeline POST claim API | FR-07, FR-16 |
| Epic 3 / T 3.3 — Claim status polling | FR-09, FR-17 |
| Epic 3 / T 3.4 — Node-side claim accept/lock | FR-08 |
| Epic 4 / T 4.1 — Cryptographic verification | FR-10 |
| Epic 4 / T 4.2 — Traceability matrix generator | FR-11, FR-12, NFR-01, NFR-13 |
| Epic 4 / T 4.3 — RBAC | FR-16 |

---

## 4. Program Increment → Deliverable → arc42 Anchor

| PI | Deliverable | arc42 Anchor | Primary ADR(s) |
|----|-------------|--------------|-----------------|
| PI 1 | D 1.1 NATS broker (mTLS) | §07.3, §08.1.1 | ADR-0001 |
| PI 1 | D 1.2 Protobuf contracts (Go + Rust bindings) | §05.4 | ADR-0007 |
| PI 1 | D 1.3 Go control plane (registration, tagging) | §05.2, §07.2 | (ADR-0007) |
| PI 2 | D 2.1 Rust agent skeleton (systemd, polling) | §05.3, §07.5 | ADR-0001, ADR-0002 |
| PI 2 | D 2.2 Cryptographic verification (JWS/Ed25519) | §05.8, §08.1.5 | ADR-0003 |
| PI 2 | D 2.3 Telemetry engine | §08.2.1 | ADR-0007 |
| PI 3 | D 3.1 Modular execution engine | §05.3, §08.6 | ADR-0007 |
| PI 3 | D 3.2 Btrfs / ext4 A/B partitioning | §05.9, §07.4 | ADR-0004 |
| PI 3 | D 3.3 Bootloader fallback (GRUB chainload + boot-counter) | §06.2, §08.6.3 | ADR-0004 |
| PI 4 | D 4.1 Asynchronous Claim Registry | §05.6, §06.4 | ADR-0005 |
| PI 4 | D 4.2 Edge node claiming | §05.5, §06.4 | ADR-0001, ADR-0005 |
| PI 4 | D 4.3 ROS2 + NATS Leaf Nodes | §07.3 | ADR-0001 |
| PI 5 | D 5.1 SELinux strict policies (`ota_agent.pp`) | §08.1.4 | ADR-0006 |
| PI 5 | D 5.2 Traceability & compliance reporting | §08.7 | ADR-0003 |
| PI 5 | D 5.3 Agent self-update | §06.5, §08.6.4 | ADR-0002, ADR-0003 |

---

## 5. Coverage Verification (self-check)

| Check | Result |
|-------|--------|
| Every UC produces at least one FR | OK (each UC ≥ 4 FRs) |
| Every UC produces at least one NFR | OK (each UC ≥ 3 NFRs) |
| Every FR (FR-01..FR-30) has ≥ 1 UC source | OK |
| Every FR (FR-01..FR-30) has ≥ 1 arc42 section | OK |
| Every NFR (NFR-01..NFR-16) has ≥ 1 UC or brief source | OK |
| Every NFR (NFR-01..NFR-16) has ≥ 1 arc42 section | OK |
| Every NFR has a Quality Scenario in §10 | OK for NFR-01..NFR-14 (QS-01..QS-14, NFR-13 shares QS-01); **NFR-15 QS and NFR-16 QS to be authored in §10 (QS-15, QS-16)** |
| Every Brief Deliverable (D 1.1..D 5.3) has an arc42 anchor | OK |
| Every Brief Epic Task has at least one FR/NFR mapping | OK |
| Every ADR (ADR-0001..ADR-0011) is referenced by ≥ 1 FR or NFR | OK |
| All quality goals (Un-brickability, Traceability, Edge Simplicity, Hardware Portability, Offline Capability, Anti-Rollback) trace to ADRs in §09.3 | OK |

---

## 6. How to Use This Matrix

- **Adding a new requirement.** Append to [functional.md](functional.md) or [non-functional.md](non-functional.md) with a stable ID; add a row in §2 here; if it justifies a new architectural decision, draft an ADR and add to §09.2.
- **Adding a new use case.** Author the UC document; ensure each derived requirement gets an ID and a row in §2; add the UC to §1.
- **Auditing.** Pick any FR/NFR; trace to UC (proves it is needed), to arc42 section (proves it has a home), to PI/Deliverable (proves it is scheduled), and to QS-NN (proves how it is verified).
- **CI gate (future).** A linter can parse this matrix and assert that no FR/NFR is orphaned and that no new arc42 section is added without back-references.
