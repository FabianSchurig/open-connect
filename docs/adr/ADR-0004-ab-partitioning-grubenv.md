# ADR-0004 — Boot-redundancy reference device profiles (ext4 A/B, dual-disk chainload, Btrfs snapshot)

- **Status:** Proposed
- **Date:** 2026-04-21 (amended from original title "ext4 A/B partitioning with GRUB `grubenv` boot-counter (Btrfs snapshot variant)")
- **Deciders:** Architecture Working Group, Embedded Engineering
- **Related Requirements:** [FR-03](../requirements/functional.md#fr-03--dual-banking-ext4-root), [FR-04](../requirements/functional.md#fr-04--inactive-partition-writes-only), [FR-05](../requirements/functional.md#fr-05--grubenv-boot-toggling), [FR-06](../requirements/functional.md#fr-06--automated-bootloader-fallback), [FR-20](../requirements/functional.md#fr-20--btrfs-atomic-snapshot-variant-from-brief-deliverable-32), [FR-30](../requirements/functional.md#fr-30--device-profile-script-authoring-conventions), [TC-06](../arc42/02-architecture-constraints.md#22-technical-constraints)
- **Related Use Cases:** [UC-01](../use-cases/UC-01-ab-ota-medical.md)
- **Related ADRs:** consumed by [ADR-0008](ADR-0008-config-driven-primitive-engine.md) (every profile below is expressed as signed scripts, not compiled agent code)

## Context

Quality-Goal #1 — **un-brickability** — demands every OS update can be reverted automatically if the new image fails to operate. The platform must support heterogeneous hardware: medical devices with custom partition layouts, industrial PCs with a secondary disk, and robots using Btrfs-rooted Ubuntu images. GRUB is mandated by [TC-06](../arc42/02-architecture-constraints.md#22-technical-constraints).

A single "canonical" boot-redundancy scheme does not fit all of these. We therefore catalog a small set of **reference device profiles**; each is a named, documented pattern that a device class selects at provisioning, expressed entirely as signed scripts invoked via [ADR-0008](ADR-0008-config-driven-primitive-engine.md) primitives.

## Decision

Maintain a **Device Profile Catalog** with at least three reference profiles; the agent itself is profile-agnostic.

### Profile 1 — `ext4-partition-ab` (default for green-field devices)

Dual-banking **A/B ext4 partitions on one disk**, controlled by GRUB `grubenv` with a boot-counter and fallback. Default choice when we control the disk layout at provisioning.

| Partition | Purpose |
|-----------|---------|
| ESP (FAT32) | GRUB EFI binaries, UKI staging, `grubenv` |
| Root A (ext4) | Bank A |
| Root B (ext4) | Bank B |
| Persistent (ext4) | State preserved across both banks |

`grubenv` variables: `boot_part`, `boot_count`, `boot_success`, `previous_part`. See [§5.9](../arc42/05-building-block-view.md#59-detailed-design--on-disk--partition-layout) for semantics.

### Profile 2 — `dual-disk-chainload` (for hardware with two physical disks)

Two **physical disks** (typically `/dev/sda` and `/dev/sdb`), each fully self-contained with its own bootloader, kernel, and rootfs. The primary GRUB chainloads the inactive disk; GRUB's `grub-reboot <entry>` sets a **one-shot** `next_entry` so only the next boot attempts the new disk.

Update flow (captured from the field-tested prototype):

1. Unpack artifact (e.g., OVA → VMDK → block device via `qemu-nbd`) onto the inactive disk with `dd` (or equivalent).
2. Write `/etc/grub.d/41_custom` with a chainload entry for the inactive disk:
   ```text
   menuentry "Boot from sdb" {
       set root=(hd1)
       chainloader +1
   }
   ```
3. `update-grub` (or distro equivalent) to regenerate `grub.cfg`.
4. `grub-reboot "Boot from sdb"` → sets `next_entry` (one-shot).
5. Return control to the agent; the agent's `REBOOT` primitive reboots.
6. Post-boot health check determines whether to commit (make permanent via `grub-set-default`) or revert (boot back to the previous disk).

**Advantages vs. partition A/B:** simpler mental model; each disk is fully independent (different filesystems / vendors are OK); no partition-layout-at-provisioning constraint; any bootloader difference is isolated to one disk.

**Trade-offs:** 2× disk cost (not 2× partition cost); the `grub-reboot` one-shot gives a single attempt (compare to the multi-attempt boot-counter on Profile 1 — see §Grubenv Patterns below); two bootloaders to maintain.

### Profile 3 — `btrfs-snapshot` (for brownfield Btrfs-rooted devices)

Single root partition with subvolume `@`. Updates take a read-only snapshot, mutate a candidate subvol, swap the default via `btrfs subvolume set-default`, reboot. Rollback restores the prior default. Preferred when adding a second root partition is impractical on already-shipped hardware.

### Grubenv patterns — both supported

Two GRUB-side mechanisms are in use across profiles, and our documentation must cover **both**:

| Pattern | Variable(s) | Semantics | Rollback story |
|---------|-------------|-----------|----------------|
| **Boot-counter** (Profile 1 default) | `boot_part`, `boot_count`, `boot_success`, `previous_part` | GRUB decrements `boot_count` each boot and falls back to `previous_part` when the counter hits 0 without a success ack. | **Multi-attempt**: up to N auto-reverts without any operator involvement. |
| **One-shot `next_entry`** (Profile 2 default) | `next_entry`, `saved_entry` | `grub-reboot <entry>` sets `next_entry` for exactly one boot; next boot clears it and reverts to `saved_entry`. | **Single-attempt**: if the new disk boots, the agent commits via `grub-set-default`; otherwise the previous disk is already the fallback on the very next boot. |

Boot-counter is preferred where achievable (stronger un-brickability). `next_entry` is valid when the BIOS/GRUB doesn't reliably support the counter script, or when chainloading makes counter semantics awkward.

### Boot-counter logic (Profile 1 GRUB script)

```text
load_env
if boot_count == 0 && boot_success == 0:
    boot_part = previous_part
    save_env
chainload_partition(boot_part)
boot_count = max(0, boot_count - 1)
save_env
```

## Consequences

### Positive

- **Atomic OS swap** on the device regardless of profile: only the inactive bank / disk / subvolume is mutated; the active side is always known-good ([FR-04](../requirements/functional.md#fr-04--inactive-partition-writes-only)).
- **Automatic rollback** (Profile 1/3 boot-counter) or **single-attempt fallback** (Profile 2) without operator intervention ([FR-06](../requirements/functional.md#fr-06--automated-bootloader-fallback)).
- **Agent is profile-agnostic** — each profile is a library of signed scripts, not Rust code ([ADR-0008](ADR-0008-config-driven-primitive-engine.md)); new profiles require **no agent release**.
- **Real-world fit.** The dual-disk chainload profile matches the shape of actual field deployments (OVA/VMDK distribution, chainload between physical disks) — our catalog explicitly supports it rather than treating it as an exception.
- **GRUB is the lowest-common-denominator bootloader** for x86 and modern ARM EFI hardware; no custom bootloader code.

### Negative

- **Storage overhead** — 2× root on Profile 1, 2× whole-disk on Profile 2. Acceptable at the device classes specified in deployment requirements.
- **`grubenv` corruption risk** mid-write (power loss); mitigated by atomic write semantics in `grub2-editenv` and read-back verification.
- **Profile proliferation risk** — each added profile is a new library of scripts to maintain and QA. Mitigated by keeping the catalog small (3 reference profiles in v1), gating additions on an ADR amendment, and enforcing authoring conventions ([FR-30](../requirements/functional.md#fr-30--device-profile-script-authoring-conventions)).
- **Profile 2 single-attempt fallback is weaker** than a multi-attempt counter. Acceptable when paired with the agent's post-boot health check (§8.6.3) which commits on success.

### Neutral

- A future variant could use Unified Kernel Images (UKIs) signed for SecureBoot; compatible with Profiles 1 and 2 (the ESP / bootable disk holds the UKI in either case).

## Alternatives Considered

### A. Single canonical profile (pick one)
- **Pros:** Minimizes the library of scripts to maintain.
- **Cons:** Rejects either green-field devices (if we pick dual-disk) or already-shipped dual-disk devices (if we pick partition A/B). Real-world deployments are heterogeneous.
- **Verdict:** Rejected — the cost of maintaining three well-scoped profile libraries is lower than excluding real hardware.

### B. OSTree
- **Pros:** Atomic file-tree commits; deduplication; great rollback.
- **Cons:** Imposes an OS image format; not friendly to "bring your own ext4 / VMDK" devices; deeper integration than the brief assumes.
- **Verdict:** Rejected for v1 (candidate for a future Profile 4 if a green-field hardware program adopts it).

### C. RAUC / SWUpdate / Mender
- **Pros:** Mature OTA frameworks with A/B built in.
- **Cons:** Each imposes its own update artifact format and slot-management conventions; we'd be married to one; the modular step model doesn't map cleanly; we lose control of step execution semantics (FR-01, FR-02).
- **Verdict:** Rejected; we adopt the *concept* of A/B but keep the engine.

### D. Single-partition with squashfs overlay
- **Pros:** Simpler partition layout; immutable base.
- **Cons:** Rollback requires snapshotting the overlay; overlayfs corruption is harder to recover from; doesn't help when we need to update the kernel/initramfs.
- **Verdict:** Rejected.

### E. systemd-boot instead of GRUB
- **Pros:** Simpler config; native to systemd ecosystem.
- **Cons:** Mandated GRUB by [TC-06](../arc42/02-architecture-constraints.md#22-technical-constraints); weaker counter/rollback story than what we script in GRUB.
- **Verdict:** Rejected by constraint.

## Compliance Notes

- Every profile's script library is a **design history file artifact** (versioned; changes reviewed jointly with QA / Regulatory); additions require an ADR amendment.
- The boot-counter window (`boot_count` initial value) is a configurable risk parameter per profile; default `3` is documented and revisited per device class.
- `--unrestricted` / password-less GRUB menu entries are **prohibited** on production medical devices unless explicitly risk-accepted per device class; see [FR-30](../requirements/functional.md#fr-30--device-profile-script-authoring-conventions).
- Destructive operations in profile scripts (e.g., `dd` to a block device) MUST include a target-safety interlock (e.g., refuse when target device equals the currently-mounted root, serial/model cross-check where feasible) — see [FR-30](../requirements/functional.md#fr-30--device-profile-script-authoring-conventions).
