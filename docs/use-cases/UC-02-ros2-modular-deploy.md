# UC-02 — ROS2 Autonomous AI Validation (Modular Deployment)

| Field | Value |
|-------|-------|
| **ID** | UC-02 |
| **Title** | ROS2 Autonomous AI Validation (Modular Deployment) |
| **Primary Actor** | AI / Robotics Engineer |
| **Secondary Actors** | Rust Edge Agent (on robot), NATS Leaf Node (on robot), NATS Hub (cloud), Control Plane |
| **Scope** | Hot-swap of ROS2 nodes and neural-network weights without flashing the OS |
| **Level** | User-goal |
| **Status** | Baseline (from project brief) |

---

## 1. Brief

An engineer needs to deploy a specific combination of AI models and ROS2 nodes to a test robot, *without* flashing the entire ext4 OS. Updates target only application components and must survive intermittent cellular/Wi-Fi outages.

## 2. Stakeholders & Interests

| Stakeholder | Interest |
|-------------|----------|
| AI / Robotics Engineer | Fast iteration on models and node code. |
| Test Robot Operator | Robot remains responsive locally even when the cloud link drops. |
| QA | Audit trail of which model + node combo ran on which robot. |

## 3. Preconditions

- Robot is registered with tags `robot`, `ros2`, and a hardware tag (e.g., `arm64-jetson`).
- Robot runs a NATS Leaf Node that federates to the cloud NATS cluster.
- ROS2 daemon is supervised by systemd; agent has unit-restart privileges via SELinux policy.
- Public verification key for the engineer's signing identity is provisioned on the robot.

## 4. Trigger

Engineer publishes a new `DesiredState` containing a modular flow:

1. `RunScript` — stop current ROS2 nodes (`systemctl stop ros2-app.service`)
2. `DownloadArtifact` — fetch new neural-network weights to a versioned path under `/var/lib/ota/models/<version>/`
3. `RunCommand` — atomic symlink swap to new model
4. `SystemdRestart` — restart `ros2-app.service`

## 5. Main Success Flow

1. Engineer authors and signs the modular manifest (JWS, EdDSA).
2. Control Plane stores the `DesiredState` for the target robot.
3. NATS Hub forwards availability to the robot's Leaf Node.
4. Edge Agent fetches the manifest via local Leaf request-reply (low-latency, in-LAN).
5. Agent verifies JWS signature. *(FR-10)*
6. Agent executes steps 1–4 sequentially, capturing per-step logs and emitting `StepResult` after each. *(FR-01)*
7. After `SystemdRestart`, the agent waits for `ros2-app.service` to reach `active (running)` within a configurable timeout.
8. Agent publishes a final cryptographic acknowledgment `(device_serial, manifest_hash, model_version, node_revision, timestamp, signature)` on `audit.deployment.<deployment_id>`. *(NFR-01)*

**Post-Condition (success):** Robot is running new ROS2 nodes against new model weights; OS partitions are untouched; audit trail is complete.

## 6. Alternative & Error Flows

### Alt-1 — Cellular/Wi-Fi link drops mid-download
- 4a. Agent's HTTP fetch fails. The Leaf Node continues to buffer outbound telemetry.
- 4b. Agent retries with backoff; if download was partial it resumes via HTTP `Range` (where supported) or restarts.
- 4c. When the cloud link returns, queued telemetry flushes from the Leaf to the Hub. *(NFR-04, see also UC-03 Alt-1)*

### Alt-2 — Local ROS2 communication during cloud outage
- The robot's local ROS2 nodes continue to publish to the **local Leaf Node** without cloud dependency. Operators on the robot's LAN can still observe topics. *(NFR-04)*

### Err-1 — `RunScript` to stop ROS2 nodes returns non-zero
- Agent halts the sequence, does not download or restart anything. *(FR-02)*
- Agent publishes `StepResult{success=false}` and remains on previous configuration.

### Err-2 — Model artifact signature/checksum mismatch
- Agent rejects the manifest at step 0 *(verification)* or at step 2 *(checksum)*; ROS2 service is not restarted.

### Err-3 — `ros2-app.service` fails to reach `active` within timeout
- Agent treats this as step failure, attempts symlink rollback to previous model version, restarts the service, and reports `Reverted`.

## 7. Derived Requirements

- **Functional:** FR-01, FR-02, FR-10
- **Non-Functional:** NFR-01, NFR-02, NFR-04
- **New requirements raised by walkthrough:**
  - FR-13 *(new)* — Agent must support a `SystemdRestart` step type with a configurable readiness timeout and readiness probe (default: unit `active (running)`).
  - FR-14 *(new)* — Modular flows shall support application-only updates (no partition writes, no reboot).
  - NFR-06 *(new)* — Robots must remain operable on local ROS2 traffic during cloud outages of up to 24 hours; telemetry buffering shall be bounded by configured local disk quota.
  - NFR-07 *(new)* — `DownloadArtifact` shall support resumable downloads where the artifact server advertises HTTP `Range`.

## 8. Related Architecture

- Sequence diagram: [arc42 §06 — UC-02 sequence](../arc42/06-runtime-view.md#uc-02--ros2-modular-deployment)
- NATS topology with Leaf Nodes: [arc42 §07 Deployment View](../arc42/07-deployment-view.md#nats-topology)
- ADRs: [ADR-0001 NATS](../adr/ADR-0001-nats-over-http-rest.md), [ADR-0003 JWS](../adr/ADR-0003-jws-ed25519-manifests.md)

## 9. Open Issues

- Whether ROS2 DDS discovery needs to be tunneled through NATS Leaf or remain on multicast (deferred — operational decision).
- Atomic-swap semantics for very large model files on small `/var` partitions.
