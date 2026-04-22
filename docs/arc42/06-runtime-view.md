# 6. Runtime View

This section walks through the platform at runtime, one scenario per subsection. Each scenario is anchored in a use case from [`../use-cases/`](../use-cases/) and references the building blocks in [§05](05-building-block-view.md).

Participants:

- **RM** — Release Manager (human)
- **ENG** — AI / Robotics Engineer (human)
- **CI** — CI/CD Pipeline (automated client)
- **API** — Go Control Plane API
- **DSS** — Desired-State Service
- **CR** — Claim Registry
- **AS** — Audit Service
- **NATS** — NATS Hub
- **LEAF** — NATS Leaf Node (on-robot)
- **AGENT** — Rust Edge Agent
- **GRUB** — Bootloader (with boot-counter script)
- **ART** — Artifact Store

---

## 6.1 UC-01 — A/B OTA Medical Update

Anchored in [UC-01](../use-cases/UC-01-ab-ota-medical.md). Happy path including ack and audit closure.

```mermaid
sequenceDiagram
    autonumber
    actor RM as Release Manager
    participant API
    participant DSS
    participant AS
    participant NATS
    participant AGENT as Rust Agent
    participant ART as Artifact Store
    participant GRUB

    RM->>API: PUT /v1/desired-state/by-tag/production (JWS envelope)
    API->>DSS: persist DesiredState
    DSS->>AS: record "deployment requested" (deployment_id)
    DSS->>NATS: publish availability marker (per device)

    AGENT->>NATS: request device.<id>.desired-state
    NATS-->>AGENT: JWS envelope
    AGENT->>AGENT: verify Ed25519 signature
    Note right of AGENT: Reject if invalid (Err-1, see 6.2)

    loop for each step (sequential)
        AGENT->>ART: GET artifact (resumable, sha256 verified)
        AGENT->>AGENT: execute step (capture stdout/stderr)
        AGENT->>NATS: publish StepResult
        NATS->>AS: index step result
    end

    AGENT->>GRUB: write grubenv (boot_part=B, boot_count=3, boot_success=0)
    AGENT->>AGENT: controlled reboot
    Note over AGENT,GRUB: GRUB chainloads Bank B
    AGENT->>AGENT: post-boot health checks pass
    AGENT->>GRUB: set boot_success=1, boot_count=0
    AGENT->>NATS: publish DeploymentAck (signed)
    NATS->>AS: append ack to audit chain
    AS-->>API: linkable evidence record
```

---

## 6.2 UC-01 Err-3 — Boot-Counter Rollback

What happens when the new image boots but fails its health check.

```mermaid
sequenceDiagram
    autonumber
    participant AGENT as Rust Agent (pre-update on Bank A)
    participant GRUB
    participant AGENT_B as Rust Agent (post-update on Bank B)
    participant NATS
    participant AS

    AGENT->>GRUB: write grubenv: boot_part=B, boot_count=3, boot_success=0
    AGENT->>AGENT: reboot
    GRUB->>GRUB: chainload Bank B (boot_count -> 2)
    AGENT_B->>AGENT_B: boot, attempt health check
    Note over AGENT_B: health check fails (e.g., service won't start)
    AGENT_B->>AGENT_B: reboot
    GRUB->>GRUB: chainload Bank B (boot_count -> 1)
    AGENT_B->>AGENT_B: still failing, reboot
    GRUB->>GRUB: chainload Bank B (boot_count -> 0)
    AGENT_B->>AGENT_B: still failing, reboot
    GRUB->>GRUB: boot_count==0 + boot_success==0 -> set boot_part=previous_part (A)
    GRUB->>GRUB: chainload Bank A
    AGENT->>AGENT: boot on Bank A, detect "Reverted" condition
    AGENT->>NATS: publish StepResult{success=false, reason="post_boot_revert"}
    NATS->>AS: append revert event
    AS-->>AS: deployment marked Reverted in audit chain
```

---

## 6.3 UC-02 — ROS2 Modular Deployment

Anchored in [UC-02](../use-cases/UC-02-ros2-modular-deploy.md). Includes the Leaf Node and shows local-first behaviour.

```mermaid
sequenceDiagram
    autonumber
    actor ENG as Robotics Engineer
    participant API
    participant DSS
    participant NATS as NATS Hub
    participant LEAF as NATS Leaf (on robot)
    participant AGENT as Rust Agent (on robot)
    participant ART as Artifact Store
    participant ROS as ROS2 Stack
    participant AS

    ENG->>API: PUT /v1/desired-state/by-serial/<robot> (JWS envelope)
    API->>DSS: persist DesiredState
    DSS->>NATS: publish availability marker
    NATS->>LEAF: federated forward
    AGENT->>LEAF: request device.<id>.desired-state (low-latency local)
    LEAF-->>AGENT: JWS envelope
    AGENT->>AGENT: verify signature

    AGENT->>ROS: systemctl stop ros2-app.service (SCRIPT_EXECUTION)
    AGENT->>LEAF: publish StepResult
    AGENT->>ART: GET model_v3.bin (FILE_TRANSFER, resumable)
    Note right of AGENT: link drops; agent retries with backoff
    AGENT->>ART: GET model_v3.bin (resume from offset)
    AGENT->>LEAF: publish StepResult
    AGENT->>AGENT: atomic symlink swap (SCRIPT_EXECUTION)
    AGENT->>LEAF: publish StepResult
    AGENT->>ROS: systemctl restart ros2-app.service (SYSTEM_SERVICE)
    AGENT->>ROS: poll until active (within readiness timeout)
    AGENT->>LEAF: publish StepResult{success=true}
    AGENT->>LEAF: publish DeploymentAck (signed)

    Note over LEAF,NATS: WAN reconnects; Leaf flushes buffered messages
    LEAF->>NATS: forward queued StepResults + Ack
    NATS->>AS: append to audit chain
```

---

## 6.4 UC-03 — CI/CD HIL Device Claiming

Anchored in [UC-03](../use-cases/UC-03-cicd-hil-claiming.md). Concurrent lockers and TTL behaviour shown.

```mermaid
sequenceDiagram
    autonumber
    actor CI as CI/CD Pipeline
    participant API
    participant CR as Claim Registry
    participant NATS
    participant A1 as Agent 1 (idle)
    participant A2 as Agent 2 (idle)
    participant A3 as Agent 3 (idle)
    participant DSS

    CI->>API: POST /v1/claims {count:2, tags:[ros2-hil,x86], version:v2.4, ttl:3600}
    API->>CR: validate RBAC, persist claim_id, state=Open
    CR->>NATS: publish on claim.offer.ros2-hil

    par offer fan-out
        NATS-->>A1: ClaimOffer(claim_id)
        NATS-->>A2: ClaimOffer(claim_id)
        NATS-->>A3: ClaimOffer(claim_id)
    end

    par concurrent lock attempts
        A1->>NATS: request claim.lock.<claim_id>
        A2->>NATS: request claim.lock.<claim_id>
        A3->>NATS: request claim.lock.<claim_id>
    end

    NATS->>CR: forward lock attempts (request-reply)
    CR->>CR: optimistic concurrency: grant first 2, deny 3rd
    CR-->>A1: granted, lease_id, prep_timeout
    CR-->>A2: granted, lease_id, prep_timeout
    CR-->>A3: denied (slots exhausted)

    par preparation
        A1->>DSS: request desired-state for v2.4
        DSS-->>A1: JWS envelope
        A1->>A1: verify, execute install steps
        A1->>NATS: publish StepResults + transition Ready
        A2->>DSS: request desired-state for v2.4
        DSS-->>A2: JWS envelope
        A2->>A2: verify, execute install steps
        A2->>NATS: publish StepResults + transition Ready
    end

    loop polling
        CI->>API: GET /v1/claims/<claim_id>
        API->>CR: read claim state + per-device map
        CR-->>API: {state: Preparing, devices:[A1: Preparing, A2: Preparing]}
        API-->>CI: JSON
    end

    CR-->>API: state -> Ready (both devices)
    CI->>API: GET /v1/claims/<claim_id>
    API-->>CI: {state: Ready, devices:[A1: Ready, A2: Ready, conn_info:{...}]}
    CI->>A1: run integration tests
    CI->>A2: run integration tests
    CI->>API: DELETE /v1/claims/<claim_id>
    API->>CR: release
    CR->>NATS: publish claim.release.<claim_id>
    NATS-->>A1: release
    NATS-->>A2: release
    A1->>A1: transition Released -> Idle
    A2->>A2: transition Released -> Idle
```

### UC-03 Alt-2 — Lock TTL Force-Release

```mermaid
sequenceDiagram
    autonumber
    participant CR as Claim Registry
    participant NATS
    participant A1 as Agent 1 (locked)

    A1-->>CR: ... silence (network loss, > preparation_timeout) ...
    CR->>CR: timer fires, lease expired
    CR->>NATS: publish claim.release.<claim_id>:<lease_id>
    CR->>NATS: republish on claim.offer.<tag> (slot reopened)
    Note over CR: NFR-08: force-release within 30s of expiry
    A1->>NATS: (later) reconnect, observe lease revoked
    A1->>A1: abort preparation, transition Idle
```

---

## 6.5 Agent Self-Update (FR-19)

```mermaid
sequenceDiagram
    autonumber
    participant AGENT as Rust Agent v1
    participant NATS
    participant ART
    participant SYSD as systemd
    participant FS as Filesystem
    participant AGENT2 as Rust Agent v2 (post-restart)

    AGENT->>NATS: pull desired state for self
    NATS-->>AGENT: JWS envelope (manifest with FlashAgent step)
    AGENT->>AGENT: verify signature
    AGENT->>ART: GET ota-agent-v2 (sha256, resumable)
    AGENT->>FS: write to /usr/local/lib/ota-agent/agent.new
    AGENT->>AGENT: verify Ed25519 signature on the binary itself
    AGENT->>FS: rename(agent.new, agent) - atomic
    AGENT->>SYSD: systemctl restart ota-agent.service
    Note over AGENT,SYSD: process exits; systemd respawns from new binary
    AGENT2->>NATS: heartbeat with new agent_version
    AGENT2->>NATS: publish DeploymentAck (signed by device key)
```

**Failure handling.**
- If the rename succeeds but the new binary fails to start, systemd retries up to a configured limit; on persistent failure, an out-of-band watchdog (separate `ota-agent-watchdog.service`) restores `/usr/local/lib/ota-agent/agent.bak` (the prior binary preserved during stage).
- The watchdog is intentionally **not** itself self-updatable in v1 to maintain a known-good recovery path.

---

## 6.6 Telemetry Loop (FR-18)

```mermaid
sequenceDiagram
    autonumber
    participant AGENT as Rust Agent
    participant TEL as Telemetry Sampler
    participant BUF as Local Disk Buffer
    participant LEAF
    participant NATS
    participant API

    loop every telemetry_interval (default 30s)
        TEL->>TEL: sample CPU, mem, disk, net, ROS2, agent_version
        TEL->>BUF: append (bounded by quota)
        TEL->>LEAF: publish device.<id>.telemetry
        LEAF->>NATS: forward
        NATS->>API: ingest into time-series store
        BUF->>BUF: drop oldest if quota exceeded
    end

    Note over LEAF,NATS: WAN drop -> Leaf accumulates locally
    Note over LEAF,NATS: WAN restored -> Leaf drains buffer
```

---

## 6.7 Cross-Cutting: NATS Reconnection Behaviour (NFR-04)

All agent NATS publishes use exponential backoff with jitter (initial 1s, cap 60s). Pull requests (`device.<id>.desired-state`) are issued on a fixed cadence (`poll_interval`, default 30s) plus on heartbeat-loss-recovery. JetStream subjects (`telemetry`, `step-result`, `ack`) are durable and survive disconnects without message loss within the JetStream retention policy.
