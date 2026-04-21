# Modernized Autonomous OTA & Fleet Management Platform — Documentation

This directory contains the **foundational specification** for the platform, organized so that every line of code that is written later can be traced back to a regulatory requirement, a stakeholder need, and an explicit architectural decision.

The structure follows the [arc42](https://arc42.org/) architecture template, complemented by separate **Use Case** and **Requirements** catalogues so that traceability stays clean for IEC 62304 / ISO 13485 / ISO 81001-5-1 audits.

> **Status:** Draft / pre-implementation. No source code, infrastructure manifests, or signing keys exist yet. ADRs are in `Proposed` status pending review.

---

## Reading Order

| # | Document | Purpose |
|---|----------|---------|
| 1 | [Use Cases](use-cases/) | What humans and pipelines need to accomplish. |
| 2 | [Requirements](requirements/) | Functional and non-functional requirements derived from the use cases. |
| 3 | [arc42 §01 Introduction & Goals](arc42/01-introduction-and-goals.md) | Vision, top quality goals, stakeholders. |
| 4 | [arc42 §02 Constraints](arc42/02-architecture-constraints.md) | Regulatory, technical, organizational. |
| 5 | [arc42 §03 Scope & Context](arc42/03-system-scope-and-context.md) | Business and technical context boundaries. |
| 6 | [arc42 §04 Solution Strategy](arc42/04-solution-strategy.md) | The core approach in one page. |
| 7 | [arc42 §05 Building Block View](arc42/05-building-block-view.md) | Component decomposition + interface contracts. |
| 8 | [arc42 §06 Runtime View](arc42/06-runtime-view.md) | Sequence diagrams for the use cases. |
| 9 | [arc42 §07 Deployment View](arc42/07-deployment-view.md) | Cluster topology, device disk layout. |
| 10 | [arc42 §08 Crosscutting Concepts](arc42/08-crosscutting-concepts.md) | Security, observability, resilience, versioning. |
| 11 | [arc42 §09 Architectural Decisions](arc42/09-architectural-decisions.md) | Index of ADRs. |
| 12 | [arc42 §10 Quality Requirements](arc42/10-quality-requirements.md) | Measurable quality scenarios. |
| 13 | [arc42 §11 Risks & Technical Debt](arc42/11-risks-and-technical-debt.md) | Known risks. |
| 14 | [arc42 §12 Glossary](arc42/12-glossary.md) | Domain & technical terms. |
| 15 | [Architectural Decision Records](adr/) | Full ADR documents. |
| 16 | [Traceability Matrix](requirements/traceability-matrix.md) | UC ↔ FR/NFR ↔ arc42 ↔ Epic/Task ↔ ADR. |

---

## Document Conventions

- **IDs are stable** once assigned. Never renumber UCs, FRs, NFRs, ADRs.
- **Cross-references use markdown links** with full repo-relative paths.
- **Diagrams** use Mermaid embedded in the markdown.
- **MoSCoW priorities** (Must / Should / Could / Won't) are used on every requirement.
- **Verification methods** are one of: `Test`, `Inspection`, `Analysis`, `Demonstration`, `Review`.

---

## What is *Not* Here

- `.proto` files — sketched in [§05](arc42/05-building-block-view.md), not yet generated.
- Go / Rust source — directory layout is described in [§07](arc42/07-deployment-view.md).
- Kubernetes manifests, SELinux `.te` files, signing keys — implementation phase.

---

## Stakeholders

| Role | Interest |
|------|----------|
| Release Manager / QA | Push verified updates safely, prove compliance. |
| AI / Robotics Engineer | Iterate quickly on ROS2 nodes & model weights. |
| CI/CD Pipeline (automated) | Reserve hardware, run integration tests deterministically. |
| Site Reliability Engineer | Operate the control plane and NATS fabric. |
| Regulatory Affairs | Produce audit evidence for IEC 62304 / ISO 13485. |
| Security Officer | Enforce key custody, RBAC, SELinux confinement. |
