# 1. Introduction and Goals

## 1.1 Requirements Overview

The Modernized Autonomous OTA & Fleet Management Platform delivers **safe, signed, modular, un-brickable software updates** to a heterogeneous fleet of Linux-based devices: medical instruments, autonomous robots, and HIL test rigs.

The platform is composed of three operational planes:

1. **Control Plane** — a Kubernetes-deployable Go service for device registration, tagging, desired-state management, claim orchestration, and audit.
2. **Messaging Fabric** — a mTLS-secured NATS cluster (with on-device Leaf Nodes) carrying telemetry and pull-style command traffic.
3. **Edge Agent** — a Rust daemon that pulls signed manifests, verifies them, and executes a fixed, finite set of generic primitives (script execution, file transfer, system service, container, self-update, reboot). All OS-specific logic — A/B partitioning, GRUB `grubenv` toggling, Btrfs snapshots — is delivered as **signed device-profile scripts inside the manifest**, not compiled into the agent (see [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md)). Results and telemetry flow back over NATS.

The complete requirements catalogue lives at:

- [Functional Requirements](../requirements/functional.md) (FR-01 … FR-30)
- [Non-Functional Requirements](../requirements/non-functional.md) (NFR-01 … NFR-16)
- [Use Cases](../use-cases/) (UC-01, UC-02, UC-03)

## 1.2 Quality Goals

The top-3 quality goals (in priority order) drive every architectural decision:

| Rank | Goal | Why it dominates |
|------|------|------------------|
| 1 | **Safety / Un-brickability** | Devices in clinical or autonomous-robotic settings must never become inoperable due to an update; rollback must be automatic and bounded. |
| 2 | **Traceable Compliance** | Every deployment must produce regulator-grade evidence linking *who requested what*, *what ran where*, and *what the device cryptographically acknowledged*. (IEC 62304 / ISO 13485 / ISO 81001-5-1) |
| 3 | **Operational Simplicity at the Edge** | Devices behind NAT, on cellular, or in air-gapped clinical networks must work without inbound ports, without manual intervention, and survive 24-hour outages. |

Three further goals are mandatory and shape the same set of load-bearing decisions; they are listed separately because they are subordinate when conflicts arise but are referenced directly by the [Decision Map (§9.3)](09-architectural-decisions.md#93-decision-map):

| Rank | Goal | Why it matters |
|------|------|----------------|
| 4 | **Hardware Portability** | The same agent binary must run on heterogeneous Linux targets (Yocto / Ubuntu / Debian, x86 / ARM) without recompilation per device class — a direct consequence of [ADR-0008](../adr/ADR-0008-config-driven-primitive-engine.md). |
| 5 | **Offline / Air-Gapped Capability** | Air-gapped manufacturing, HIL labs behind data diodes, and field service must use the same signing, verification, and audit path as online deployments — see [ADR-0011](../adr/ADR-0011-offline-bundle-format.md). |
| 6 | **Anti-Tamper / Anti-Rollback** | The fleet must not silently regress, even when presented with a correctly-signed but older manifest — see [ADR-0010](../adr/ADR-0010-anti-rollback-enforcement.md). |

Other secondary qualities (cloud-agnostic deployment, RBAC, low-latency local communication for robotics) flow from the goals above and from the constraints in [§02](02-architecture-constraints.md).

## 1.3 Stakeholders

| Role | Expectations / Concerns | Primary Interface |
|------|-------------------------|-------------------|
| Release Manager / QA | Push verified updates safely, see rollback events, prove compliance. | Control plane API/UI; audit reports. |
| AI / Robotics Engineer | Iterate quickly on ROS2 nodes & model weights without OS reflashes. | Manifest authoring CLI; control plane API. |
| CI/CD Pipeline (automated) | Reserve N rigs deterministically; no inbound coupling; fail-fast TTLs. | REST API (`/v1/claims`). |
| Site Reliability Engineer | Operate the control plane and NATS fabric; observe fleet health. | Helm charts; Grafana / OpenTelemetry. |
| Regulatory Affairs | Export tamper-evident audit packages on demand. | Audit export API. |
| Security Officer | Enforce key custody, RBAC, SELinux confinement, mTLS everywhere. | Policy review; RBAC config. |
| Field Service Technician | Recover bricked devices is a non-event; logs explain what happened. | Local console; telemetry replays. |

## 1.4 Success Metrics

- **Zero bricks** in production over a 12-month operational window across the medical fleet.
- **100% audit coverage** — every deployment present in audit log with cryptographic ack.
- **≤ 30 s** force-release latency for stale claims (NFR-08).
- **≤ 60 s** state-reconciliation time after a 24 h NATS outage (NFR-04).
- **0** dynamic-link dependencies in the agent binary (NFR-03, NFR-10).
