//! Primitive trait + reference primitives (Epic L + N).
//!
//! Per ADR-0008 and FR-24, the agent must contain NO hardcoded references to
//! GRUB, ext4, Btrfs, systemd-boot, U-Boot, or specific distributions — all
//! such intelligence is delivered as signed scripts through the
//! `SCRIPT_EXECUTION` primitive defined here.

#![forbid(unsafe_code)]

use std::{
    collections::HashMap,
    path::PathBuf,
    sync::Arc,
    time::Duration,
};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use thiserror::Error;
use tokio::{
    fs::{self, OpenOptions},
    io::AsyncWriteExt,
    process::Command,
};

#[derive(Debug, Error)]
pub enum PrimitiveError {
    #[error("io: {0}")]
    Io(#[from] std::io::Error),
    #[error("hash mismatch: want {want}, got {got}")]
    HashMismatch { want: String, got: String },
    #[error("unknown primitive `{0}`")]
    Unknown(String),
    #[error("invalid parameters for {primitive}: {detail}")]
    BadParameters { primitive: String, detail: String },
    #[error("step timed out after {0:?}")]
    Timeout(Duration),
    #[error("script exited with code {0}")]
    ScriptFailed(i32),
}

/// StepResult mirrors `otap.v1.StepResult` for now (kept independent so the
/// crate has no proto-bindgen dependency in MVP).
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct StepResult {
    pub step_id: String,
    pub primitive: String,
    pub success: bool,
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
    pub stdout_truncated: bool,
    pub stderr_truncated: bool,
    pub duration_ms: u64,
}

/// Per-step context (writable spool dir, cancellation, env to merge in).
pub struct StepContext {
    pub spool_dir: PathBuf,
    pub deployment_id: String,
    pub serial: String,
    /// HTTP client used by FILE_TRANSFER. The trait abstraction lets unit
    /// tests inject a fake.
    pub fetcher: Arc<dyn Fetcher>,
}

/// Contract for HTTPS GET (Epic L + M). The MVP implementation is a thin
/// wrapper around `reqwest`; tests inject a fake that serves `file://` paths
/// or in-memory bodies.
#[async_trait]
pub trait Fetcher: Send + Sync {
    async fn fetch(&self, url: &str, headers: &HashMap<String, String>) -> Result<Vec<u8>, PrimitiveError>;
}

/// Minimal in-memory fetcher used by tests.
pub struct MemFetcher(pub HashMap<String, Vec<u8>>);

#[async_trait]
impl Fetcher for MemFetcher {
    async fn fetch(&self, url: &str, _headers: &HashMap<String, String>) -> Result<Vec<u8>, PrimitiveError> {
        self.0
            .get(url)
            .cloned()
            .ok_or_else(|| PrimitiveError::BadParameters {
                primitive: "FILE_TRANSFER".into(),
                detail: format!("MemFetcher: unknown url {url}"),
            })
    }
}

/// Primitive — every step type implements this.
#[async_trait]
pub trait Primitive: Send + Sync {
    fn name(&self) -> &'static str;
    async fn execute(
        &self,
        step_id: &str,
        params: &serde_json::Value,
        ctx: &StepContext,
    ) -> Result<StepResult, PrimitiveError>;
}

/// Registry — a `PrimitiveRegistry` is the OS-portable substrate of the
/// config-driven engine.
#[derive(Default)]
pub struct PrimitiveRegistry {
    primitives: HashMap<&'static str, Arc<dyn Primitive>>,
}

impl PrimitiveRegistry {
    pub fn new() -> Self {
        Self::default()
    }
    pub fn register(&mut self, p: Arc<dyn Primitive>) {
        self.primitives.insert(p.name(), p);
    }
    pub fn get(&self, name: &str) -> Option<Arc<dyn Primitive>> {
        self.primitives.get(name).cloned()
    }
    pub fn known(&self) -> Vec<&'static str> {
        self.primitives.keys().copied().collect()
    }
}

// --- FILE_TRANSFER ---------------------------------------------------------

pub struct FileTransfer;

#[async_trait]
impl Primitive for FileTransfer {
    fn name(&self) -> &'static str {
        "FILE_TRANSFER"
    }

    async fn execute(
        &self,
        step_id: &str,
        params: &serde_json::Value,
        ctx: &StepContext,
    ) -> Result<StepResult, PrimitiveError> {
        let started = std::time::Instant::now();
        let url = params
            .get("url")
            .and_then(|v| v.as_str())
            .ok_or_else(|| PrimitiveError::BadParameters {
                primitive: self.name().into(),
                detail: "missing url".into(),
            })?;
        let want_hash = params
            .get("sha256")
            .and_then(|v| v.as_str())
            .ok_or_else(|| PrimitiveError::BadParameters {
                primitive: self.name().into(),
                detail: "missing sha256".into(),
            })?
            .to_lowercase();
        let dest = params
            .get("dest_path")
            .and_then(|v| v.as_str())
            .map(PathBuf::from)
            .unwrap_or_else(|| ctx.spool_dir.join(format!("{}.bin", step_id)));

        // Auth headers (bearer / api-key); pulled from process env (Epic M).
        let mut headers = HashMap::new();
        if let Ok(t) = std::env::var("OTA_ARTIFACTORY_BEARER") {
            if !t.is_empty() {
                headers.insert("Authorization".to_string(), format!("Bearer {t}"));
            }
        }
        if let Ok(k) = std::env::var("OTA_ARTIFACTORY_API_KEY") {
            if !k.is_empty() {
                headers.insert("X-JFrog-Art-Api".to_string(), k);
            }
        }

        let body = ctx.fetcher.fetch(url, &headers).await?;

        // Streaming SHA-256 (one-shot here since fetcher returns Vec<u8>; the
        // production HTTP fetcher will hash chunks as it streams).
        let mut hasher = Sha256::new();
        hasher.update(&body);
        let got = hex::encode(hasher.finalize());
        if got != want_hash {
            return Err(PrimitiveError::HashMismatch { want: want_hash, got });
        }

        if let Some(parent) = dest.parent() {
            fs::create_dir_all(parent).await?;
        }
        let mut f = OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(&dest)
            .await?;
        f.write_all(&body).await?;
        f.flush().await?;

        Ok(StepResult {
            step_id: step_id.into(),
            primitive: self.name().into(),
            success: true,
            exit_code: 0,
            stdout: format!("wrote {} bytes -> {}", body.len(), dest.display()),
            stderr: String::new(),
            stdout_truncated: false,
            stderr_truncated: false,
            duration_ms: started.elapsed().as_millis() as u64,
        })
    }
}

// --- SCRIPT_EXECUTION ------------------------------------------------------

pub struct ScriptExecution;

#[async_trait]
impl Primitive for ScriptExecution {
    fn name(&self) -> &'static str {
        "SCRIPT_EXECUTION"
    }

    async fn execute(
        &self,
        step_id: &str,
        params: &serde_json::Value,
        _ctx: &StepContext,
    ) -> Result<StepResult, PrimitiveError> {
        let started = std::time::Instant::now();
        let interpreter = params
            .get("interpreter")
            .and_then(|v| v.as_str())
            .unwrap_or("/bin/bash");
        let script_ref = params
            .get("script_ref")
            .and_then(|v| v.as_str())
            .ok_or_else(|| PrimitiveError::BadParameters {
                primitive: self.name().into(),
                detail: "missing script_ref".into(),
            })?;
        let script_sha256 = params
            .get("script_sha256")
            .and_then(|v| v.as_str())
            .ok_or_else(|| PrimitiveError::BadParameters {
                primitive: self.name().into(),
                detail: "missing script_sha256".into(),
            })?;
        // Ensure file exists and matches sha256.
        let body = fs::read(script_ref).await?;
        let mut h = Sha256::new();
        h.update(&body);
        let got = hex::encode(h.finalize());
        if got != script_sha256.to_lowercase() {
            return Err(PrimitiveError::HashMismatch {
                want: script_sha256.into(),
                got,
            });
        }

        // Build env. Per FR-30 only OTA_*-prefixed vars from manifest are passed.
        let mut cmd = Command::new(interpreter);
        cmd.arg(script_ref);
        cmd.env_clear();
        // Preserve a minimal PATH so the script can find /usr/bin tools.
        cmd.env(
            "PATH",
            std::env::var("PATH").unwrap_or_else(|_| "/usr/sbin:/usr/bin:/sbin:/bin".into()),
        );
        if let Some(env) = params.get("env").and_then(|v| v.as_object()) {
            for (k, v) in env {
                if !k.starts_with("OTA_") {
                    return Err(PrimitiveError::BadParameters {
                        primitive: self.name().into(),
                        detail: format!("env var `{k}` must start with OTA_ (FR-30 §3)"),
                    });
                }
                if let Some(s) = v.as_str() {
                    cmd.env(k, s);
                }
            }
        }
        let output = cmd.output().await?;
        let (stdout, stdout_trunc) = ringbuf(&output.stdout, MAX_STREAM_BYTES);
        let (stderr, stderr_trunc) = ringbuf(&output.stderr, MAX_STREAM_BYTES);
        let exit_code = output.status.code().unwrap_or(-1);
        let success = output.status.success();
        Ok(StepResult {
            step_id: step_id.into(),
            primitive: self.name().into(),
            success,
            exit_code,
            stdout,
            stderr,
            stdout_truncated: stdout_trunc,
            stderr_truncated: stderr_trunc,
            duration_ms: started.elapsed().as_millis() as u64,
        })
    }
}

// FR-21: 1 MiB ring per stream per step.
pub const MAX_STREAM_BYTES: usize = 1 << 20;

fn ringbuf(buf: &[u8], max: usize) -> (String, bool) {
    if buf.len() <= max {
        return (String::from_utf8_lossy(buf).into_owned(), false);
    }
    let tail = &buf[buf.len() - max..];
    (String::from_utf8_lossy(tail).into_owned(), true)
}

// --- REBOOT ---------------------------------------------------------------

/// Reboot writes a sentinel and returns success. The actual `systemctl reboot`
/// is invoked by the supervisor *after* it has flushed step results — that
/// keeps this primitive a pure intent. FR-30 §1 forbids any other primitive
/// from triggering a reboot.
pub struct Reboot {
    pub sentinel_path: PathBuf,
}

#[async_trait]
impl Primitive for Reboot {
    fn name(&self) -> &'static str {
        "REBOOT"
    }
    async fn execute(
        &self,
        step_id: &str,
        params: &serde_json::Value,
        _ctx: &StepContext,
    ) -> Result<StepResult, PrimitiveError> {
        let grace = params.get("grace_seconds").and_then(|v| v.as_u64()).unwrap_or(30);
        if let Some(p) = self.sentinel_path.parent() {
            fs::create_dir_all(p).await?;
        }
        fs::write(
            &self.sentinel_path,
            serde_json::to_vec_pretty(&serde_json::json!({
                "grace_seconds": grace,
                "step_id": step_id,
            }))
            .unwrap(),
        )
        .await?;
        Ok(StepResult {
            step_id: step_id.into(),
            primitive: self.name().into(),
            success: true,
            exit_code: 0,
            stdout: format!("reboot scheduled (grace={grace}s, sentinel={})", self.sentinel_path.display()),
            stderr: String::new(),
            stdout_truncated: false,
            stderr_truncated: false,
            duration_ms: 0,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::Arc;
    use tempfile::tempdir;

    fn ctx(spool: &std::path::Path, fetcher: Arc<dyn Fetcher>) -> StepContext {
        StepContext {
            spool_dir: spool.to_path_buf(),
            deployment_id: "d".into(),
            serial: "s".into(),
            fetcher,
        }
    }

    #[tokio::test]
    async fn file_transfer_writes_and_verifies_hash() {
        let dir = tempdir().unwrap();
        let body = b"hello world".to_vec();
        let mut hasher = Sha256::new();
        hasher.update(&body);
        let want = hex::encode(hasher.finalize());

        let mut store = HashMap::new();
        store.insert("https://example/wic".to_string(), body.clone());
        let fetcher = Arc::new(MemFetcher(store));

        let p = FileTransfer;
        let dest = dir.path().join("out.bin");
        let res = p
            .execute(
                "01",
                &serde_json::json!({
                    "url": "https://example/wic",
                    "sha256": want,
                    "dest_path": dest.to_string_lossy(),
                }),
                &ctx(dir.path(), fetcher),
            )
            .await
            .unwrap();
        assert!(res.success);
        let got = std::fs::read(&dest).unwrap();
        assert_eq!(got, body);
    }

    #[tokio::test]
    async fn file_transfer_rejects_bad_hash() {
        let dir = tempdir().unwrap();
        let mut store = HashMap::new();
        store.insert("u".to_string(), b"x".to_vec());
        let fetcher = Arc::new(MemFetcher(store));

        let p = FileTransfer;
        let dest = dir.path().join("o");
        let err = p
            .execute(
                "01",
                &serde_json::json!({
                    "url": "u",
                    "sha256": "deadbeef",
                    "dest_path": dest.to_string_lossy(),
                }),
                &ctx(dir.path(), fetcher),
            )
            .await
            .unwrap_err();
        assert!(matches!(err, PrimitiveError::HashMismatch { .. }));
    }

    #[tokio::test]
    async fn script_execution_runs_with_ota_env_only() {
        let dir = tempdir().unwrap();
        let script = dir.path().join("hello.sh");
        std::fs::write(&script, "#!/bin/bash\necho \"hi $OTA_NAME\"\n").unwrap();
        std::fs::set_permissions(&script, std::os::unix::fs::PermissionsExt::from_mode(0o755)).unwrap();
        let mut h = Sha256::new();
        h.update(std::fs::read(&script).unwrap());
        let want = hex::encode(h.finalize());

        let fetcher = Arc::new(MemFetcher(HashMap::new()));
        let p = ScriptExecution;
        let res = p
            .execute(
                "01",
                &serde_json::json!({
                    "interpreter": "/bin/bash",
                    "script_ref": script.to_string_lossy(),
                    "script_sha256": want,
                    "env": { "OTA_NAME": "world" }
                }),
                &ctx(dir.path(), fetcher),
            )
            .await
            .unwrap();
        assert!(res.success, "stdout={} stderr={}", res.stdout, res.stderr);
        assert!(res.stdout.contains("hi world"));
    }

    #[tokio::test]
    async fn script_execution_rejects_non_ota_env() {
        let dir = tempdir().unwrap();
        let script = dir.path().join("hello.sh");
        std::fs::write(&script, "#!/bin/bash\n:\n").unwrap();
        let mut h = Sha256::new();
        h.update(std::fs::read(&script).unwrap());
        let want = hex::encode(h.finalize());

        let fetcher = Arc::new(MemFetcher(HashMap::new()));
        let p = ScriptExecution;
        let err = p
            .execute(
                "01",
                &serde_json::json!({
                    "interpreter": "/bin/bash",
                    "script_ref": script.to_string_lossy(),
                    "script_sha256": want,
                    "env": { "PATH": "/usr/bin" }
                }),
                &ctx(dir.path(), fetcher),
            )
            .await
            .unwrap_err();
        assert!(matches!(err, PrimitiveError::BadParameters { .. }));
    }

    #[tokio::test]
    async fn reboot_writes_sentinel_only() {
        let dir = tempdir().unwrap();
        let sentinel = dir.path().join("reboot.json");
        let p = Reboot { sentinel_path: sentinel.clone() };
        let fetcher = Arc::new(MemFetcher(HashMap::new()));
        let res = p
            .execute("05", &serde_json::json!({"grace_seconds": 10}), &ctx(dir.path(), fetcher))
            .await
            .unwrap();
        assert!(res.success);
        assert!(sentinel.exists());
    }

    #[test]
    fn ringbuf_truncates() {
        let buf = vec![b'a'; MAX_STREAM_BYTES + 100];
        let (s, t) = ringbuf(&buf, MAX_STREAM_BYTES);
        assert!(t);
        assert_eq!(s.len(), MAX_STREAM_BYTES);
    }
}

// (Unix-only test path; no extra module required.)
