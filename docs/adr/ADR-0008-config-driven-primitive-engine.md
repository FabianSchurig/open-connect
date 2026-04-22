# ADR-0008 — Config-driven primitive execution engine ("dumb agent" principle)

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Embedded Engineering, Security Officer
- **Related Requirements:** [FR-01](../requirements/functional.md#fr-01--modular-manifest-execution), [FR-23](../requirements/functional.md#fr-23--docker-container-primitive), [FR-24](../requirements/functional.md#fr-24--config-driven-workflow-hardware-independence), [NFR-02](../requirements/non-functional.md#nfr-02--rust-for-memory-safety), [NFR-15](../requirements/non-functional.md#nfr-15--distribution-agnostic-single-binary)
- **Related Use Cases:** [UC-01](../use-cases/UC-01-ab-ota-medical.md), [UC-02](../use-cases/UC-02-ros2-modular-deploy.md), [UC-03](../use-cases/UC-03-cicd-hil-claiming.md)
- **Related ADRs:** amends [ADR-0002](ADR-0002-rust-for-edge-agent.md); amended by [ADR-0003](ADR-0003-jws-ed25519-manifests.md); extends [ADR-0007](ADR-0007-protobuf-contracts.md)

## Context

Earlier drafts of the architecture ([§05 Building Block View](../arc42/05-building-block-view.md)) enumerated **OS-specific step types** — `FlashPartition`, `UpdateGrubEnv` — as first-class, compiled-in variants of the modular step `oneof`. While expedient, this approach hard-codes bootloader, partitioning, and distro semantics into the Rust binary. Consequences:

- Migrating a fleet from ext4 A/B to Btrfs snapshots (or from GRUB to systemd-boot) requires **rebuilding and redeploying the agent**.
- Supporting a new hardware family with a bespoke bootloader requires a code change.
- The compiled agent knows "too much" — violating the Open-Closed Principle and expanding the attack surface.

The platform's target spans medical devices (Yocto), autonomous robots (Ubuntu), and HIL rigs (Debian/RHEL). A compiled-in strategy for *every* combination does not scale.

## Decision

Refactor the agent's execution model to a **small, fixed set of generic primitives**. All OS-specific intelligence — how to flash a partition, how to toggle `grubenv`, how to snapshot Btrfs — is **delivered as scripts inside the signed manifest**, not compiled into the agent.

### The Primitive Set (v1)

| Primitive | Purpose | Inputs |
|-----------|---------|--------|
| `SCRIPT_EXECUTION` | Run a shell / interpreter script staged from the manifest. | interpreter, signed script body, env, cwd, timeout |
| `FILE_TRANSFER` | Download a file (with checksum, resumable HTTP `Range`) and place it at a target path. | source URL, SHA-256, dest path, mode |
| `SYSTEM_SERVICE` | Start / stop / restart a systemd (or SysV) unit with a readiness probe. | unit name, action, readiness condition, timeout |
| `DOCKER_CONTAINER` | Pull an OCI image, then either cache it for later cutover or stop the previous container and start the new one. ([FR-23](../requirements/functional.md#fr-23--docker-container-primitive)) | image ref, digest, `mode` (`CACHE_ONLY` \| `RECREATE`), plus mode-specific parameters defined below |
| `AGENT_SELF_UPDATE` | Atomic binary swap + `systemctl restart ota-agent`. ([FR-19](../requirements/functional.md#fr-19--agent-self-update-from-brief-deliverable-53)) | source URL, SHA-256, signature |
| `REBOOT` | Controlled reboot with grace period. | grace seconds |

#### `DOCKER_CONTAINER` contract (normative)

The `DOCKER_CONTAINER.parameters.mode` field is a required enum with the following allowed values:

- `CACHE_ONLY`: pull and verify the OCI image into the local container runtime cache, but do **not** stop, remove, create, or start any container.
- `RECREATE`: pull and verify the OCI image, then replace the running container by stopping/removing the previous instance and creating/starting a new one from the supplied image and runtime settings.

Common parameter requirements for all `DOCKER_CONTAINER` steps:

- `image_ref` — **required**; OCI image reference to pull.
- `digest` — **required**; immutable image digest that **must** match the pulled image.
- `mode` — **required**; one of `CACHE_ONLY` or `RECREATE`.

Conditional parameter requirements:

- When `mode = CACHE_ONLY`:
  - `container_name` — **ignored** if provided.
  - `args` — **ignored** if provided.
  - `networks` — **ignored** if provided.
  - Runtime-specific container creation fields added in future revisions — **ignored** unless explicitly defined for `CACHE_ONLY`.
- When `mode = RECREATE`:
  - `container_name` — **required**; name of the container instance to replace/create.
  - `args` — **optional**; command arguments / runtime arguments for the new container. If omitted, the image default command is used.
  - `networks` — **optional**; list of networks to attach to the new container. If omitted, the runtime default network behavior applies.

Validation and execution rules:

- A manifest using any `mode` value other than `CACHE_ONLY` or `RECREATE` is invalid.
- In `CACHE_ONLY`, supplying `container_name`, `args`, or `networks` does not change behavior and must not trigger container replacement.
- In `RECREATE`, omission of `container_name` is a validation error.
- Both modes must fail the step if the pulled image does not resolve to `digest`.

No primitive has knowledge of partitions, bootloaders, filesystems, or distro conventions. A full A/B OS update is expressed as a *composition* of primitives in the JSON manifest:

```json
{
  "deployment_steps": [
    { "primitive": "FILE_TRANSFER", "parameters": { "url": "...", "sha256": "...", "dest_path": "/var/lib/ota/spool/root-b.img" } },
    { "primitive": "SCRIPT_EXECUTION", "parameters": { "interpreter": "/bin/bash", "script_ref": "pre-flash-checks.sh" } },
    { "primitive": "SCRIPT_EXECUTION", "parameters": { "interpreter": "/bin/bash", "script_ref": "flash-inactive-partition.sh", "env": { "TARGET_DEV": "/dev/sda3" } } },
    { "primitive": "SCRIPT_EXECUTION", "parameters": { "interpreter": "/bin/bash", "script_ref": "update-grubenv.sh", "env": { "BOOT_PART": "B", "BOOT_COUNT": "3" } } },
    { "primitive": "REBOOT", "parameters": { "grace_seconds": 30 } }
  ],
  "rollback_steps": [
    { "primitive": "SCRIPT_EXECUTION", "parameters": { "interpreter": "/bin/bash", "script_ref": "restore-grubenv.sh" } }
  ]
}
```

The scripts (`flash-inactive-partition.sh`, `update-grubenv.sh`, …) are authored once per *device profile*, signed with the same Ed25519 key as the manifest, and kept in the artifact store — not the agent binary.

### Agent Implementation (Strategy / Command Pattern)

```rust
// sketch
pub trait Primitive: Send + Sync {
    fn kind(&self) -> &'static str;
    fn execute(&self, ctx: &ExecCtx, params: &StepParams) -> Result<StepResult, StepError>;
}

pub struct ConfigDrivenEngine {
    registry: HashMap<&'static str, Arc<dyn Primitive>>,
}

impl ConfigDrivenEngine {
    pub fn run_manifest(&self, manifest: &SignedManifest) -> Result<ManifestOutcome, EngineError> {
        self.verify(manifest)?;
        for step in &manifest.deployment_steps {
            let p = self.registry.get(step.primitive.as_str()).ok_or(EngineError::UnknownPrimitive)?;
            let result = p.execute(&self.ctx, &step.parameters)?;
            self.publish_step_result(&result)?;
            if !result.success && !step.continue_on_error { return self.run_rollback(manifest); }
        }
        Ok(ManifestOutcome::Success)
    }
}
```

Registering a new primitive is a compile-time operation **in the agent**; the manifest author cannot invent a new primitive. But composing existing primitives to achieve new workflows is purely a manifest-side change.

## Consequences

### Positive

- **True hardware independence.** The same `aarch64-unknown-linux-musl` binary runs on Yocto-built medical devices, Ubuntu-based robots, and Debian/RHEL HIL rigs. → [NFR-15](../requirements/non-functional.md#nfr-15--distribution-agnostic-single-binary).
- **Dynamic workflows.** Migrating ext4→Btrfs, or GRUB→systemd-boot, is a **manifest** change, not an **agent** change. Fleet can adopt new OS strategies without an agent redeploy.
- **Compose-style cutovers stay declarative.** The agent only needs digest-pinned image pull/cutover primitives; staged `docker compose down` / `up` workflows remain signed manifest logic rather than a separate hard-coded subsystem.
- **Smaller compiled surface.** The Rust agent has six primitives to test; every new distro/workflow adds 0 lines of Rust.
- **Cleaner security model.** The agent's SELinux confinement (see [ADR-0006](ADR-0006-selinux-strict-policy.md)) is tied to primitives, not specific workflows. "Allowed system calls" becomes a finite, auditable set.
- **Encourages script reuse.** Device profiles are libraries of signed scripts; the manifest composes them.

### Negative

- **Responsibility shifts to manifest authors.** Writing correct `flash-inactive-partition.sh` is now a release-engineering deliverable, not a compiler-enforced invariant. Mitigated by:
  - Shipping a **reference library** of signed device-profile scripts per supported hardware class.
  - `buf`-equivalent CI for manifest schemas.
  - Mandatory code review on device-profile scripts, treated as source code.
- **Lost compile-time exhaustiveness** on step types. We can no longer guarantee at compile time that every OS operation is handled; we rely on testing + device-profile coverage.
- **Script signing key custody** becomes operationally important (scripts are signed with the same key as the manifest, or a sibling key — policy decided at rollout).

### Neutral

- The boot-counter rollback behaviour ([§6.2](../arc42/06-runtime-view.md#62-uc-01-err-3--boot-counter-rollback)) is unaffected: it is bootloader logic, executed independently of the agent.
- Telemetry, claims, audit flows all still work — they never depended on OS-specific steps.

## Alternatives Considered

### A. Keep compiled-in OS-specific step variants
- **Pros:** Compile-time exhaustiveness; simpler mental model for Rust developers.
- **Cons:** Hard-codes distro/bootloader decisions; every new OS family costs a release; violates Open-Closed Principle.
- **Verdict:** Rejected. This was the original design; this ADR explicitly replaces it.

### B. Pluggable native modules (dlopen)
- **Pros:** Native performance; new capabilities without touching the core.
- **Cons:** Incompatible with static musl linking ([NFR-03](../requirements/non-functional.md#nfr-03--single-statically-linked-binary-arm--x86)); explodes the attack surface; SELinux labeling becomes fragile.
- **Verdict:** Rejected.

### C. Embedded scripting runtime (Lua, WASM) in-process
- **Pros:** Portable logic without shelling out.
- **Cons:** Requires `execmem`-style privileges in SELinux policy (see [ADR-0006](ADR-0006-selinux-strict-policy.md)); violates the "no dynamic execution" posture of our MAC profile; adds a significant dependency.
- **Verdict:** Rejected. Out-of-process fork/exec against statically labeled scripts is the safer model.

### D. Config-driven + a small number of OS-specific "superprimitives"
- **Pros:** Compromise; keep `FlashPartition` but make others scriptable.
- **Cons:** Still hard-codes enough to require agent redeploys when bootloader changes; half-measure with no clear benefit over full config-driven.
- **Verdict:** Rejected.

## Compliance Notes

- The set of primitives is itself a configuration-management artifact (controlled via CODEOWNERS + release process). Adding one is a versioned agent change.
- Device-profile script libraries are versioned, signed, and reviewed; each script is an audit artifact.
- The primitive → SELinux domain mapping is specified in [ADR-0006](ADR-0006-selinux-strict-policy.md) so that defence-in-depth remains intact.

## Migration Notes

For the existing §05.4 Protobuf sketches: the `ModularStep.oneof` is refactored. `RunScript`/`RunCommand` become `SCRIPT_EXECUTION`; `DownloadArtifact` becomes `FILE_TRANSFER`; `SystemdRestart` becomes `SYSTEM_SERVICE`; `Reboot` is unchanged. `FlashPartition` and `UpdateGrubEnv` are **removed** — their work is done by `SCRIPT_EXECUTION` with device-profile scripts. Two new primitives are added: `DOCKER_CONTAINER` and `AGENT_SELF_UPDATE`.
