//! Anti-rollback gate (FR-25, FR-26, ADR-0010).
//!
//! `RollbackStore` is a trait so that the MVP file-backed implementation can
//! be swapped for the TPM NV-index implementation (NFR-16) without changing
//! callers. The MVP `FileStore` writes JSON to `/var/lib/persistent/anti-rollback.json`.

use std::{
    fs,
    path::{Path, PathBuf},
};

use parking_lot::Mutex;
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("io: {0}")]
    Io(#[from] std::io::Error),
    #[error("decode: {0}")]
    Decode(#[from] serde_json::Error),
    #[error("invalid semver: {0}")]
    BadVersion(String),
    #[error("manifest rejected: below lower_limit_version (current {current}, lower_limit {lower_limit})")]
    BelowLowerLimit { current: String, lower_limit: String },
    #[error("manifest rejected: rollback (desired {desired} < max_seen {max_seen}); authorized downgrade not provided")]
    Rollback { desired: String, max_seen: String },
}

/// Outcome of `evaluate`: either `Accept` (proceed) or `Reject` (don't apply).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Decision {
    Accept,
    RejectBelowLowerLimit,
    RejectRollback,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize)]
pub struct PersistedState {
    pub max_seen_version: Option<String>,
    pub current_deployed_version: Option<String>,
}

pub trait RollbackStore: Send + Sync {
    fn load(&self) -> Result<PersistedState, Error>;
    fn record_success(&self, version: &str) -> Result<(), Error>;
}

/// Inputs from the manifest payload + any `downgrade`-capability key check.
pub struct EvalInput<'a> {
    pub desired_version: &'a str,
    pub lower_limit_version: &'a str,
    pub allow_downgrade: bool,
    pub downgrade_capable_key: bool,
    pub downgrade_justification: Option<&'a str>,
}

/// Evaluate the anti-rollback gate against the persisted state.
pub fn evaluate(state: &PersistedState, input: &EvalInput) -> Result<Decision, Error> {
    let current = state.current_deployed_version.clone().unwrap_or_else(|| "0.0.0".into());
    if cmp(&current, input.lower_limit_version)? == std::cmp::Ordering::Less {
        return Ok(Decision::RejectBelowLowerLimit);
    }
    let max_seen = state.max_seen_version.clone().unwrap_or_else(|| "0.0.0".into());
    if cmp(input.desired_version, &max_seen)? == std::cmp::Ordering::Less {
        // Authorised downgrade path requires ALL three signals (FR-25).
        let authorised = input.allow_downgrade
            && input.downgrade_capable_key
            && input.downgrade_justification.map_or(false, |s| !s.is_empty());
        if !authorised {
            return Ok(Decision::RejectRollback);
        }
    }
    Ok(Decision::Accept)
}

/// Minimal semver compare (major.minor.patch with optional `-pre` / `+build`).
/// Sufficient for MVP; replace with the `semver` crate post-MVP if needed.
fn cmp(a: &str, b: &str) -> Result<std::cmp::Ordering, Error> {
    let pa = parse(a)?;
    let pb = parse(b)?;
    Ok(pa.cmp(&pb))
}

fn parse(s: &str) -> Result<(u64, u64, u64), Error> {
    let core = s.split(['-', '+']).next().unwrap_or(s);
    let mut it = core.split('.');
    let parse_one = |x: Option<&str>| -> Result<u64, Error> {
        x.ok_or_else(|| Error::BadVersion(s.to_string()))?
            .parse()
            .map_err(|_| Error::BadVersion(s.to_string()))
    };
    Ok((parse_one(it.next())?, parse_one(it.next())?, parse_one(it.next())?))
}

/// File-backed store. Writes JSON atomically (tmp + rename).
pub struct FileStore {
    path: PathBuf,
    cache: Mutex<Option<PersistedState>>,
}

impl FileStore {
    pub fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into(), cache: Mutex::new(None) }
    }
}

impl RollbackStore for FileStore {
    fn load(&self) -> Result<PersistedState, Error> {
        if let Some(c) = self.cache.lock().clone() {
            return Ok(c);
        }
        let s = match fs::read(&self.path) {
            Ok(b) => serde_json::from_slice(&b)?,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => PersistedState::default(),
            Err(e) => return Err(e.into()),
        };
        *self.cache.lock() = Some(PersistedState {
            max_seen_version: s.max_seen_version.clone(),
            current_deployed_version: s.current_deployed_version.clone(),
        });
        Ok(s)
    }

    fn record_success(&self, version: &str) -> Result<(), Error> {
        let mut state = self.load()?;
        // max_seen never decreases.
        let bump = match state.max_seen_version.as_deref() {
            None => true,
            Some(prev) => cmp(version, prev)? == std::cmp::Ordering::Greater,
        };
        if bump {
            state.max_seen_version = Some(version.to_string());
        }
        state.current_deployed_version = Some(version.to_string());
        atomic_write(&self.path, &serde_json::to_vec_pretty(&state)?)?;
        *self.cache.lock() = Some(state);
        Ok(())
    }
}

fn atomic_write(target: &Path, bytes: &[u8]) -> Result<(), Error> {
    if let Some(p) = target.parent() {
        fs::create_dir_all(p)?;
    }
    let tmp = target.with_extension("tmp");
    fs::write(&tmp, bytes)?;
    fs::rename(tmp, target)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn tmpfile(suffix: &str) -> PathBuf {
        let p = std::env::temp_dir().join(format!(
            "ota-anti-rollback-{}-{}.json",
            std::process::id(),
            suffix
        ));
        let _ = std::fs::remove_file(&p);
        p
    }

    #[test]
    fn replay_attack_rejected() {
        let store = FileStore::new(tmpfile("replay"));
        store.record_success("2.1.0").unwrap();
        let s = store.load().unwrap();
        let d = evaluate(
            &s,
            &EvalInput {
                desired_version: "2.0.0",
                lower_limit_version: "1.0.0",
                allow_downgrade: false,
                downgrade_capable_key: false,
                downgrade_justification: None,
            },
        )
        .unwrap();
        assert_eq!(d, Decision::RejectRollback);
    }

    #[test]
    fn lower_limit_rejected() {
        let store = FileStore::new(tmpfile("lower"));
        store.record_success("1.9.0").unwrap();
        let s = store.load().unwrap();
        let d = evaluate(
            &s,
            &EvalInput {
                desired_version: "2.4.0",
                lower_limit_version: "2.0.0",
                allow_downgrade: false,
                downgrade_capable_key: false,
                downgrade_justification: None,
            },
        )
        .unwrap();
        assert_eq!(d, Decision::RejectBelowLowerLimit);
    }

    #[test]
    fn authorized_downgrade_accepted() {
        let store = FileStore::new(tmpfile("dg"));
        store.record_success("2.1.0").unwrap();
        let s = store.load().unwrap();
        let d = evaluate(
            &s,
            &EvalInput {
                desired_version: "2.0.0",
                lower_limit_version: "1.0.0",
                allow_downgrade: true,
                downgrade_capable_key: true,
                downgrade_justification: Some("emergency-rollback-INC-2026-04-23"),
            },
        )
        .unwrap();
        assert_eq!(d, Decision::Accept);
    }

    #[test]
    fn happy_path_accepts() {
        let store = FileStore::new(tmpfile("happy"));
        store.record_success("2.0.0").unwrap();
        let s = store.load().unwrap();
        let d = evaluate(
            &s,
            &EvalInput {
                desired_version: "2.4.0",
                lower_limit_version: "2.0.0",
                allow_downgrade: false,
                downgrade_capable_key: false,
                downgrade_justification: None,
            },
        )
        .unwrap();
        assert_eq!(d, Decision::Accept);
    }

    #[test]
    fn max_seen_never_decreases() {
        let p = tmpfile("monotonic");
        let store = FileStore::new(&p);
        store.record_success("2.4.0").unwrap();
        // Even if a downgrade is applied later, max_seen stays at 2.4.0.
        store.record_success("2.0.0").unwrap();
        let store2 = FileStore::new(&p);
        let s = store2.load().unwrap();
        assert_eq!(s.max_seen_version.as_deref(), Some("2.4.0"));
        assert_eq!(s.current_deployed_version.as_deref(), Some("2.0.0"));
    }
}
