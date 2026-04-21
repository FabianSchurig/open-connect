# ADR-0001 — Use NATS (with Leaf Nodes) instead of HTTP polling for device traffic

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group
- **Related Requirements:** [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance), [NFR-04](../requirements/non-functional.md#nfr-04--polling-resilience-over-nats), [NFR-06](../requirements/non-functional.md#nfr-06--robot-autonomy-during-cloud-outage-from-uc-02), [NFR-11](../requirements/non-functional.md#nfr-11--all-transport-authenticated-from-fr-08-fr-10), [TC-02](../arc42/02-architecture-constraints.md#22-technical-constraints)
- **Related Use Cases:** [UC-01](../use-cases/UC-01-ab-ota-medical.md), [UC-02](../use-cases/UC-02-ros2-modular-deploy.md), [UC-03](../use-cases/UC-03-cicd-hil-claiming.md)

## Context

Devices live behind NAT, on cellular links, or in air-gapped clinical networks. They cannot reliably accept inbound connections, but they must (a) discover new desired state, (b) compete for claim slots, and (c) stream telemetry / step results / acks back to the cloud — all while gracefully surviving extended WAN outages.

Robots additionally need **local-first** communication: ROS2 traffic must continue, and outbound telemetry must buffer when the WAN drops.

The candidate transports are HTTP long-polling, gRPC server-streaming with reverse tunnels, MQTT, Kafka, and NATS.

## Decision

Adopt **NATS** as the messaging fabric, with **mTLS-secured Leaf Nodes** deployed on robots / sites that need local-first operation. Devices that do not require local-first behaviour (e.g., medical devices in clinical networks) connect directly to the NATS hub.

**JetStream** is enabled for durability on `device.*.telemetry`, `device.*.step-result`, `device.*.ack`, and `audit.deployment.*`.

All command-style interactions (pull desired state, lock claim) use NATS **request-reply**, which preserves the pull-only invariant from the device's perspective.

## Consequences

### Positive

- **Zero inbound ports** on devices and sites — satisfies [TC-02](../arc42/02-architecture-constraints.md#22-technical-constraints) and [FR-08](../requirements/functional.md#fr-08--pull-based-claim-acceptance).
- **Leaf Nodes** give us local-first behaviour and outage buffering "for free" — directly satisfies [NFR-06](../requirements/non-functional.md#nfr-06--robot-autonomy-during-cloud-outage-from-uc-02).
- **Subject ACLs** map cleanly onto our security model (device-self-only).
- **Single transport** for both publish/subscribe (telemetry) and request/reply (commands), reducing operational complexity vs. running e.g. MQTT for fanout + REST for commands.
- **Operational maturity**: NATS is CNCF-graduated, with broad production usage.

### Negative

- **NATS expertise** required by the SRE team (Postgres-level skill is much more common).
- **JetStream tuning** (retention, replicas, max-bytes per stream) must be done carefully or storage explodes silently.
- **Subject hierarchy lock-in** — getting `device.<serial>.*` semantics wrong is painful to migrate (mitigated by versioning subjects: `v1.device.<serial>.*` if needed in v2).

### Neutral

- Direct device-to-hub connections (no leaf) work too; the leaf is a pure performance/resilience optimization.
- Pipeline ↔ control-plane uses REST (HTTP/JSON) — NATS is not pushed onto external automation.

## Alternatives Considered

### A. HTTP long-polling
- **Pros:** Universally understood; trivial to debug with curl.
- **Cons:** No fan-out (per-device subscriptions don't scale to a single tag-targeted publish); no native local-first; we'd reinvent buffering and ack semantics; HTTP keep-alive over flaky cellular is its own headache.
- **Verdict:** Rejected.

### B. gRPC server-streaming with reverse tunnel (e.g., grpc-tunnel)
- **Pros:** Strong typing; widespread adoption; bi-directional.
- **Cons:** Reverse tunneling requires either an inbound port at the site or a tunnel broker (which is itself a NATS-shaped problem); operational complexity is higher; no offline buffering primitive comparable to JetStream.
- **Verdict:** Rejected.

### C. MQTT (with MQTT bridge for site-local)
- **Pros:** Industry-standard for IoT; small client footprint.
- **Cons:** No native request-reply (must layer on top); QoS semantics are subtle; brokers vary; we'd need separate audit/log fanout pipeline.
- **Verdict:** Rejected, but reconsider if a future regulator mandates MQTT specifically.

### D. Kafka
- **Pros:** Battle-tested log durability.
- **Cons:** Designed for high-throughput analytics; per-device subjects with low traffic make for inefficient partitions; client libraries are heavy; not designed for request-reply semantics.
- **Verdict:** Rejected for this use shape.

## Compliance Notes

- mTLS satisfies [NFR-11](../requirements/non-functional.md#nfr-11--all-transport-authenticated-from-fr-08-fr-10).
- Subject ACL design and operator runbook will be reviewed before Accepted status.
