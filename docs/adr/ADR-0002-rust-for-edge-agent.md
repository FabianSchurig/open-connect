# ADR-0002 — Implement the edge agent in Rust (single statically linked musl binary)

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** Architecture Working Group, Security Officer
- **Related Requirements:** [NFR-02](../requirements/non-functional.md#nfr-02--rust-for-memory-safety), [NFR-03](../requirements/non-functional.md#nfr-03--single-statically-linked-binary-arm--x86), [NFR-10](../requirements/non-functional.md#nfr-10--agent-binary-size-budget-from-nfr-03), [OC-01](../arc42/02-architecture-constraints.md#23-organizational-constraints), [OC-06](../arc42/02-architecture-constraints.md#23-organizational-constraints)
- **Related Use Cases:** all (UC-01, UC-02, UC-03)

## Context

The edge agent runs on regulated medical devices, autonomous robots, and HIL rigs. It executes signed code (scripts, binaries), writes raw block devices, and modifies bootloader state. A memory-safety class vulnerability in this binary translates directly into a class-1 medical-device safety incident and/or a robot autonomy incident.

The agent must also cross-compile to ARM and x86 with no dynamic dependencies beyond the Linux base userspace, and stay within a tight binary-size budget for OTA self-update efficiency.

## Decision

Implement the edge agent in **Rust**, statically linked against **musl libc**, distributed as a **single binary** per architecture. `unsafe` Rust is **banned outside reviewed FFI shims** and enforced by a CI lint (`#![deny(unsafe_code)]` in every crate except an explicit allowlist of FFI crates with documented rationale).

Targets:

- `x86_64-unknown-linux-musl`
- `aarch64-unknown-linux-musl`
- `armv7-unknown-linux-musleabihf` (deferred; v1.1)

## Consequences

### Positive

- **Memory safety by construction** — eliminates buffer overflow, use-after-free, double-free, and data-race CVEs from the device-side attack surface ([NFR-02](../requirements/non-functional.md#nfr-02--rust-for-memory-safety)).
- **Single static binary** — predictable deploys, trivial OTA self-update, no `apt`/`dnf` runtime dependency surprises ([NFR-03](../requirements/non-functional.md#nfr-03--single-statically-linked-binary-arm--x86)).
- **Tight footprint** — Rust + musl produces small binaries; size budget achievable ([NFR-10](../requirements/non-functional.md#nfr-10--agent-binary-size-budget-from-nfr-03)).
- **Strong async story** via Tokio for NATS + HTTP fetcher concurrency without thread-per-connection overhead.
- **Compile-time exhaustiveness** on the modular step `enum` keeps the execution engine honest as new step types are added.

### Negative

- **Smaller talent pool** than Go/Python for ops/maintenance staff. Mitigated by documented architecture, conservative use of advanced features, and pairing with the (Go) control-plane team.
- **Slower compile times** vs. Go; mitigated by sccache and CI parallelization.
- **glibc-only system libraries** (rare) cannot be linked directly; the agent must call out to system tools (`grub2-editenv`, `systemctl`) via subprocess in those cases — which we already plan to do.

### Neutral

- The control plane stays in Go ([OC-01](../arc42/02-architecture-constraints.md#23-organizational-constraints)).
- Watchdog process (`ota-agent-watchdog.service`) may also be Rust, or it may remain a tiny shell script — implementation phase decides.

## Alternatives Considered

### A. Go for the agent (single language for the whole platform)
- **Pros:** One language across the team; richer NATS/Protobuf tooling familiarity.
- **Cons:** Garbage collection pauses on a constrained device performing block I/O are an avoidable risk; static linking with cgo is awkward; binary size is larger; GC overhead in low-RAM devices is a concern; memory-safety guarantees are weaker than Rust's borrow checker (data races still possible).
- **Verdict:** Rejected. The agent's safety profile justifies the language asymmetry.

### B. C / C++
- **Pros:** Smallest binaries; pervasive on embedded.
- **Cons:** Memory safety burden falls entirely on review and tooling; explicitly contradicts the "secure-by-design medical device" mandate.
- **Verdict:** Rejected.

### C. Zig
- **Pros:** Excellent cross-compilation; modern.
- **Cons:** Pre-1.0; smaller ecosystem; no JWS/Ed25519 production-grade libraries comparable to Rust's `ring`/`ed25519-dalek`; harder to staff.
- **Verdict:** Rejected (revisit if Zig 1.0 lands and the ecosystem matures by v3).

## Compliance Notes

- Memory safety is a stated [NFR-02](../requirements/non-functional.md#nfr-02--rust-for-memory-safety) and helps demonstrate "state of the art" for IEC 62304 / ISO 81001-5-1 secure-by-design discussions.
- The `unsafe` allowlist is itself an audit artifact reviewed by the Security Officer.
