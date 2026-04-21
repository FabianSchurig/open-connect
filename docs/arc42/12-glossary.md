# 12. Glossary

| Term | Definition |
|------|------------|
| **A/B Banking** | Dual-partition pattern (Bank A and Bank B) where one partition is "active" while the other is the "inactive" target for the next OS update. The roles swap on a successful update; on failure, the bootloader reverts. |
| **ACL (NATS Subject ACL)** | Access-control rule restricting which authenticated identity may publish or subscribe on which NATS subject pattern. |
| **ADR** | Architectural Decision Record. A short document capturing one architectural decision in a consistent format. |
| **Agent / Edge Agent** | The Rust daemon running on every managed device. Pulls signed manifests, executes modular steps, manages partitions, reports back. |
| **arc42** | Open architecture documentation template with twelve standard sections; used as the structure for this docs set. |
| **Audit Bundle** | A signed export package containing all audit evidence (manifest + step results + ack + hash anchors) for a specific deployment, suitable for regulator review. |
| **Audit Hash Chain** | A chain of SHA-256 hashes in the audit log where each record's hash includes the previous record's hash, making post-hoc tampering detectable. |
| **Audit Store** | Append-only / WORM storage backing the audit log. |
| **Bank A / Bank B** | The two ext4 root partitions in the A/B layout. |
| **Boot Counter** | The `boot_count` GRUB environment variable that the bootloader script decrements on each chainload of the staged bank; reaching zero without success triggers fallback. |
| **Btrfs Subvolume** | Btrfs's snapshot-capable directory tree; the agent uses subvolumes (`@`, `@snapshots/<id>`) for the snapshot-variant update flow. |
| **Claim** | A reservation of N devices matching specific tags and software version, requested by a CI/CD pipeline. |
| **Claim Lock** | A device-side acknowledgment that it has accepted one slot of a claim. Lock has a preparation timeout. |
| **Claim Offer** | A NATS message published by the registry inviting matching devices to attempt a lock. |
| **Claim Registry** | The Go control-plane service that holds claims and arbitrates locks. |
| **Control Plane** | The Go services running on Kubernetes — API gateway, registration, desired-state, claim registry, audit, RBAC. |
| **Deployment ID** | Server-assigned UUID identifying a specific desired-state publication. Used as the correlation key across requests, step results, acks, and audit records. |
| **Desired State** | The declarative description of what should be true on a device or set of devices, expressed as an ordered list of modular steps. |
| **DDS** | Data Distribution Service. The publish-subscribe middleware ROS2 uses; not transported over NATS in v1. |
| **EdDSA / Ed25519** | Edwards-curve Digital Signature Algorithm using the Ed25519 curve. The signature scheme used for JWS-signed manifests and `DeploymentAck`. |
| **Edge Agent** | See *Agent*. |
| **ESP** | EFI System Partition. The FAT32 partition holding the GRUB EFI binaries and `grubenv`. |
| **`grubenv`** | The GRUB environment block file (`/boot/efi/EFI/.../grubenv`) holding persistent boot variables (`boot_part`, `boot_count`, `boot_success`, `previous_part`). |
| **HIL** | Hardware-In-the-Loop. Physical test rigs participating in CI/CD integration tests. |
| **HSM** | Hardware Security Module. Out of scope for v1 (see [TD-01](11-risks-and-technical-debt.md#112-accepted-technical-debt-v1)). |
| **IEC 62304** | International standard for medical device software lifecycle processes. |
| **ISO 13485** | International standard for medical device quality management systems. |
| **ISO 81001-5-1** | International standard for security in the health-software lifecycle. |
| **JetStream** | NATS's persistence/streaming layer; used here for durable telemetry, step results, and acks. |
| **JWS** | JSON Web Signature (RFC 7515). The signed envelope wrapping the Protobuf-encoded `DesiredState`. |
| **JWS Compact Serialization** | The `header.payload.signature` base64url-encoded form of a JWS. |
| **`kid`** | "Key ID" header parameter in a JWS, used by the agent to look up the verification key in its on-device trust store. |
| **Leaf Node** | A NATS server that federates outbound to a hub cluster, providing local-first messaging. Deployed on robots and sites that need offline tolerance. |
| **Manifest** | The signed `DesiredState` envelope. Sometimes used interchangeably with "Desired State"; this doc set says "manifest" when emphasizing the JWS envelope and "desired state" when emphasizing the payload. |
| **Manifest Hash** | SHA-256 of the JWS payload portion of a manifest. Used in `DeploymentAck` to bind acknowledgments to specific publications. |
| **mTLS** | Mutual TLS. Both parties present X.509 certificates and verify each other. |
| **Modular Step** | One unit of work in a `DesiredState`: `RunScript`, `RunCommand`, `DownloadArtifact`, `FlashPartition`, `UpdateGrubEnv`, `Reboot`, `SystemdRestart`. |
| **musl** | A minimalist libc implementation enabling fully static Linux binaries. The agent links against musl. |
| **NATS** | Cloud-native messaging system used as the platform's messaging fabric. |
| **NFR / FR** | Non-Functional / Functional Requirement. |
| **OTA** | Over-the-Air (update). Remote software update of a fielded device. |
| **`ota_agent_t` / `ota_script_t`** | SELinux Type Enforcement domain types for the agent process and for staged script execution. |
| **`ota-agent.service`** | systemd unit hosting the agent. |
| **`ota-agent-watchdog.service`** | Auxiliary systemd unit that restores `agent.bak` if the agent fails to restart after a self-update. |
| **Pull-based** | The device initiates communication; the server does not call into the device. Implemented via NATS request-reply and subscriptions. |
| **Protobuf** | Protocol Buffers. The canonical wire schema language for the platform. |
| **RBAC** | Role-Based Access Control. Enforced at the API gateway and at NATS subject ACLs. |
| **Reconciliation** | The process by which an agent compares its `current_deployment_id` to the latest `DesiredState` and converges. |
| **`restorecon`** | SELinux command (or equivalent libselinux call) that re-applies the correct type label to a file based on policy. Used by the agent to label staged scripts. |
| **ROS2** | Robot Operating System 2. |
| **SELinux Type Enforcement (TE)** | The principal SELinux access-control model based on labels (types) on subjects (processes) and objects (files, sockets, etc.). |
| **State Store** | Postgres database holding device, tag, desired-state, and claim records. |
| **Step Result** | The Protobuf record an agent emits per executed step (success/failure, exit code, captured stdout/stderr). |
| **Tag** | A string label attached to a device used as the primary selector for desired-state targeting and claim matching (e.g., `production`, `ros2-hil`, `x86`). |
| **Trust Store** | The on-device directory of public keys (`/etc/ota-agent/trust-store/`) used to verify manifest signatures. |
| **TTL** | Time To Live. Bound on how long a claim remains active. |
| **UKI** | Unified Kernel Image. A single executable bundling the kernel, initramfs, and command line; can be signed end-to-end. The platform supports UKI staging on the ESP. |
| **WORM** | Write-Once-Read-Many. Storage mode used for the audit log to satisfy immutability requirements. |
