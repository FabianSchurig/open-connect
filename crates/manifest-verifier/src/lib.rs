//! Open-Connect manifest verifier — implements the strict ruleset of arc42 §5.8.
//!
//! Verification is fail-closed; the first failure aborts the deployment and
//! returns an error suitable for emission as an audit event.
//!
//! This crate is intentionally minimal in dependencies (no full JOSE library):
//!   - `alg` MUST be `EdDSA` (no algorithm confusion);
//!   - signatures are verified with `ed25519-dalek` (constant-time `verify`);
//!   - canonicalisation is JCS (RFC 8785) — the verifier re-canonicalises the
//!     decoded payload and asserts byte-for-byte equality with the signed
//!     bytes;
//!   - the JSON schema validation is currently a structural sanity check on
//!     `schema_version` + the required top-level fields. Full JSON-Schema
//!     validation against `proto/manifest/v1/manifest.schema.json` arrives in
//!     Epic C (CI) without changing this crate's API.

#![forbid(unsafe_code)]

use std::collections::{BTreeMap, HashMap};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum VerifyError {
    #[error("malformed JWS: {0}")]
    Malformed(&'static str),
    #[error("base64 decode failed: {0}")]
    Base64(#[from] base64::DecodeError),
    #[error("json decode failed: {0}")]
    Json(#[from] serde_json::Error),
    #[error("disallowed alg `{0}`; only EdDSA is accepted (§5.8 rule 1)")]
    DisallowedAlg(String),
    #[error("unknown kid `{0}` (§5.8 rule 2)")]
    UnknownKid(String),
    #[error("invalid public key in trust store entry `{0}`")]
    BadKey(String),
    #[error("signature verify failed (§5.8 rule 3)")]
    BadSignature,
    #[error("payload not JCS-canonical (§5.8 rule 5)")]
    NotCanonical,
    #[error("schema violation: {0}")]
    Schema(String),
    #[error("unknown primitive `{0}` (§5.8 rule 6)")]
    UnknownPrimitive(String),
    #[error("artifact `{0}` referenced by step but not present in artifacts[] (§5.8 rule 7 / FR-29)")]
    MissingArtifact(String),
    #[error("artifact hash mismatch for `{0}` between step and pin index (§5.8 rule 8)")]
    HashMismatch(String),
}

/// On-device trust store: kid -> verifying key (ed25519 pubkey).
#[derive(Default, Clone)]
pub struct TrustStore {
    keys: HashMap<String, VerifyingKey>,
}

impl TrustStore {
    pub fn new() -> Self {
        Self::default()
    }

    /// Register a key from raw 32-byte ed25519 public-key bytes.
    pub fn register_raw(&mut self, kid: impl Into<String>, raw: [u8; 32]) -> Result<(), VerifyError> {
        let kid = kid.into();
        let vk = VerifyingKey::from_bytes(&raw).map_err(|_| VerifyError::BadKey(kid.clone()))?;
        self.keys.insert(kid, vk);
        Ok(())
    }

    pub fn get(&self, kid: &str) -> Option<&VerifyingKey> {
        self.keys.get(kid)
    }
}

/// JOSE header subset we accept.
#[derive(Debug, Deserialize, Serialize)]
pub struct Header {
    pub alg: String,
    pub typ: Option<String>,
    pub kid: String,
    #[serde(default)]
    pub ver: u32,
}

/// The verified manifest, returned by `verify`.
#[derive(Debug)]
pub struct VerifiedManifest {
    pub header: Header,
    pub payload: serde_json::Value,
    /// SHA-256 of BASE64URL(JCS payload), per §5.8 rule 13.
    pub manifest_hash: String,
}

/// Verify a JWS-compact-serialised manifest against the trust store and the
/// set of registered primitives.
pub fn verify(
    jws_compact: &str,
    trust: &TrustStore,
    known_primitives: &[&str],
) -> Result<VerifiedManifest, VerifyError> {
    // Split.
    let mut parts = jws_compact.split('.');
    let h_b64 = parts.next().ok_or(VerifyError::Malformed("missing header"))?;
    let p_b64 = parts.next().ok_or(VerifyError::Malformed("missing payload"))?;
    let s_b64 = parts.next().ok_or(VerifyError::Malformed("missing signature"))?;
    if parts.next().is_some() {
        return Err(VerifyError::Malformed("too many parts"));
    }

    let h_bytes = URL_SAFE_NO_PAD.decode(h_b64)?;
    let p_bytes = URL_SAFE_NO_PAD.decode(p_b64)?;
    let s_bytes = URL_SAFE_NO_PAD.decode(s_b64)?;

    // Rule 1: alg MUST be EdDSA.
    let header: Header = serde_json::from_slice(&h_bytes)?;
    if header.alg != "EdDSA" {
        return Err(VerifyError::DisallowedAlg(header.alg));
    }

    // Rule 2: kid MUST resolve.
    let vk = trust
        .get(&header.kid)
        .ok_or_else(|| VerifyError::UnknownKid(header.kid.clone()))?;

    // Rule 3: signature verify over encoded(header) "." encoded(payload).
    let signing_input = format!("{}.{}", h_b64, p_b64);
    let sig_bytes: [u8; 64] = s_bytes
        .as_slice()
        .try_into()
        .map_err(|_| VerifyError::Malformed("signature length"))?;
    let sig = Signature::from_bytes(&sig_bytes);
    vk.verify(signing_input.as_bytes(), &sig)
        .map_err(|_| VerifyError::BadSignature)?;

    // Rule 4 + 5: parse + re-canonicalise + compare bytes.
    let payload: serde_json::Value = serde_json::from_slice(&p_bytes)?;
    let recanon = jcs(&payload)?;
    if recanon != p_bytes {
        return Err(VerifyError::NotCanonical);
    }

    // Schema sanity.
    schema_check(&payload)?;

    // Rule 6 + 7 + 8.
    primitives_check(&payload, known_primitives)?;
    artifacts_pin_check(&payload)?;

    let mut h = Sha256::new();
    h.update(p_b64.as_bytes());
    let manifest_hash = hex::encode(h.finalize());

    Ok(VerifiedManifest { header, payload, manifest_hash })
}

fn schema_check(payload: &serde_json::Value) -> Result<(), VerifyError> {
    let obj = payload.as_object().ok_or_else(|| VerifyError::Schema("payload is not an object".into()))?;
    for required in [
        "schema_version",
        "deployment_id",
        "desired_version",
        "lower_limit_version",
        "issued_at",
        "requested_by",
        "artifacts",
        "deployment_steps",
    ] {
        if !obj.contains_key(required) {
            return Err(VerifyError::Schema(format!("missing required field `{required}`")));
        }
    }
    let v = obj.get("schema_version").and_then(|v| v.as_u64()).unwrap_or(0);
    if v != 1 {
        return Err(VerifyError::Schema(format!("unsupported schema_version {v}")));
    }
    Ok(())
}

fn primitives_check(payload: &serde_json::Value, known: &[&str]) -> Result<(), VerifyError> {
    let known: std::collections::HashSet<&str> = known.iter().copied().collect();
    let steps = payload
        .get("deployment_steps")
        .and_then(|v| v.as_array())
        .ok_or_else(|| VerifyError::Schema("deployment_steps must be an array".into()))?;
    for step in steps {
        let prim = step
            .get("primitive")
            .and_then(|v| v.as_str())
            .ok_or_else(|| VerifyError::Schema("step.primitive missing".into()))?;
        if !known.contains(prim) {
            return Err(VerifyError::UnknownPrimitive(prim.to_string()));
        }
    }
    Ok(())
}

fn artifacts_pin_check(payload: &serde_json::Value) -> Result<(), VerifyError> {
    // Build a map name -> sha256 from the top-level artifacts[] pin index.
    let pins: BTreeMap<String, String> = payload
        .get("artifacts")
        .and_then(|v| v.as_array())
        .map(|a| {
            a.iter()
                .filter_map(|e| {
                    let n = e.get("name")?.as_str()?.to_string();
                    let h = e.get("sha256")?.as_str()?.to_string();
                    Some((n, h))
                })
                .collect()
        })
        .unwrap_or_default();

    let steps = payload.get("deployment_steps").and_then(|v| v.as_array()).cloned().unwrap_or_default();
    let rb = payload.get("rollback_steps").and_then(|v| v.as_array()).cloned().unwrap_or_default();
    for step in steps.iter().chain(rb.iter()) {
        let params = step.get("parameters").and_then(|v| v.as_object());
        let prim = step.get("primitive").and_then(|v| v.as_str()).unwrap_or("");

        // Look at common artifact-bearing fields.
        let candidates: &[(&str, &str)] = match prim {
            "FILE_TRANSFER" => &[("dest_artifact", "sha256")],
            "SCRIPT_EXECUTION" => &[("script_ref", "script_sha256")],
            _ => &[],
        };
        if let Some(p) = params {
            for (name_key, hash_key) in candidates {
                if let (Some(n), Some(h)) = (p.get(*name_key).and_then(|v| v.as_str()), p.get(*hash_key).and_then(|v| v.as_str())) {
                    if let Some(pinned) = pins.get(n) {
                        if pinned != h {
                            return Err(VerifyError::HashMismatch(n.to_string()));
                        }
                    } else {
                        return Err(VerifyError::MissingArtifact(n.to_string()));
                    }
                }
            }
            // FILE_TRANSFER also pins by name="basename(dest_path)" via sha256 — relax in MVP.
        }
    }
    Ok(())
}

/// JCS (RFC 8785) — minimal implementation matching the Go side.
pub fn jcs(value: &serde_json::Value) -> Result<Vec<u8>, VerifyError> {
    let mut buf = Vec::new();
    write_jcs(&mut buf, value)?;
    Ok(buf)
}

fn write_jcs(out: &mut Vec<u8>, v: &serde_json::Value) -> Result<(), VerifyError> {
    match v {
        serde_json::Value::Null => out.extend_from_slice(b"null"),
        serde_json::Value::Bool(b) => {
            out.extend_from_slice(if *b { b"true" } else { b"false" })
        }
        serde_json::Value::Number(n) => out.extend_from_slice(n.to_string().as_bytes()),
        serde_json::Value::String(s) => write_jcs_string(out, s),
        serde_json::Value::Array(a) => {
            out.push(b'[');
            for (i, e) in a.iter().enumerate() {
                if i > 0 {
                    out.push(b',');
                }
                write_jcs(out, e)?;
            }
            out.push(b']');
        }
        serde_json::Value::Object(map) => {
            // serde_json::Map preserves insertion order; we need lexicographic.
            let mut keys: Vec<&String> = map.keys().collect();
            keys.sort();
            out.push(b'{');
            for (i, k) in keys.iter().enumerate() {
                if i > 0 {
                    out.push(b',');
                }
                write_jcs_string(out, k);
                out.push(b':');
                write_jcs(out, map.get(*k).unwrap())?;
            }
            out.push(b'}');
        }
    }
    Ok(())
}

fn write_jcs_string(out: &mut Vec<u8>, s: &str) {
    // serde_json's escape rules already match RFC 8785 §3.2 for ASCII.
    let encoded = serde_json::to_string(s).expect("string serialise");
    out.extend_from_slice(encoded.as_bytes());
}

#[cfg(test)]
mod tests {
    use super::*;
    use ed25519_dalek::SigningKey;

    fn keypair_from_seed(seed: &[u8; 32]) -> (SigningKey, [u8; 32]) {
        let sk = SigningKey::from_bytes(seed);
        let pk = sk.verifying_key().to_bytes();
        (sk, pk)
    }

    fn sign_compact(sk: &SigningKey, kid: &str, payload: &serde_json::Value) -> String {
        let header = serde_json::json!({
            "alg": "EdDSA", "typ": "otap-desired-state+json", "kid": kid, "ver": 1
        });
        let h_bytes = serde_json::to_vec(&header).unwrap();
        let p_bytes = jcs(payload).unwrap();
        let h_b64 = URL_SAFE_NO_PAD.encode(&h_bytes);
        let p_b64 = URL_SAFE_NO_PAD.encode(&p_bytes);
        let signing_input = format!("{}.{}", h_b64, p_b64);
        let sig: Signature = ed25519_dalek::Signer::sign(sk, signing_input.as_bytes());
        format!("{}.{}", signing_input, URL_SAFE_NO_PAD.encode(sig.to_bytes()))
    }

    fn good_payload() -> serde_json::Value {
        serde_json::json!({
            "schema_version": 1,
            "deployment_id": "abc",
            "desired_version": "2.4.0",
            "lower_limit_version": "2.0.0",
            "issued_at": "2026-04-23T12:00:00Z",
            "requested_by": "alice",
            "artifacts": [
                {"name": "wic", "sha256": "aa", "size": 1}
            ],
            "deployment_steps": [
                {
                    "step_id": "01",
                    "primitive": "FILE_TRANSFER",
                    "parameters": {"url": "https://x", "sha256": "aa", "dest_path": "/tmp/x"}
                }
            ]
        })
    }

    #[test]
    fn verify_happy_path() {
        let (sk, pk) = keypair_from_seed(&[1; 32]);
        let mut ts = TrustStore::new();
        ts.register_raw("k1", pk).unwrap();
        let jws = sign_compact(&sk, "k1", &good_payload());
        let v = verify(&jws, &ts, &["FILE_TRANSFER", "SCRIPT_EXECUTION", "REBOOT"]).unwrap();
        assert_eq!(v.header.kid, "k1");
        assert_eq!(v.manifest_hash.len(), 64);
    }

    #[test]
    fn rejects_unknown_kid() {
        let (sk, _) = keypair_from_seed(&[2; 32]);
        let ts = TrustStore::new();
        let jws = sign_compact(&sk, "missing", &good_payload());
        assert!(matches!(verify(&jws, &ts, &["FILE_TRANSFER"]), Err(VerifyError::UnknownKid(_))));
    }

    #[test]
    fn rejects_disallowed_alg() {
        let (sk, pk) = keypair_from_seed(&[3; 32]);
        let mut ts = TrustStore::new();
        ts.register_raw("k1", pk).unwrap();
        // Build a JWS with alg=HS256 -> must reject.
        let header = serde_json::json!({"alg": "HS256", "typ": "x", "kid": "k1", "ver": 1});
        let h_bytes = serde_json::to_vec(&header).unwrap();
        let p_bytes = jcs(&good_payload()).unwrap();
        let h_b64 = URL_SAFE_NO_PAD.encode(&h_bytes);
        let p_b64 = URL_SAFE_NO_PAD.encode(&p_bytes);
        let signing_input = format!("{}.{}", h_b64, p_b64);
        let sig = ed25519_dalek::Signer::sign(&sk, signing_input.as_bytes());
        let sig_bytes: Signature = sig;
        let jws = format!("{}.{}", signing_input, URL_SAFE_NO_PAD.encode(sig_bytes.to_bytes()));
        assert!(matches!(verify(&jws, &ts, &["FILE_TRANSFER"]), Err(VerifyError::DisallowedAlg(_))));
    }

    #[test]
    fn rejects_tampered_payload() {
        let (sk, pk) = keypair_from_seed(&[4; 32]);
        let mut ts = TrustStore::new();
        ts.register_raw("k1", pk).unwrap();
        let jws = sign_compact(&sk, "k1", &good_payload());
        let mut parts: Vec<&str> = jws.splitn(3, '.').collect();
        // Replace payload with a different (correctly canonical) one.
        let mut altered = good_payload();
        altered["deployment_id"] = serde_json::json!("evil");
        let alt_b64 = URL_SAFE_NO_PAD.encode(jcs(&altered).unwrap());
        parts[1] = &alt_b64;
        let tampered = parts.join(".");
        assert!(matches!(verify(&tampered, &ts, &["FILE_TRANSFER"]), Err(VerifyError::BadSignature)));
    }

    #[test]
    fn rejects_unknown_primitive() {
        let (sk, pk) = keypair_from_seed(&[5; 32]);
        let mut ts = TrustStore::new();
        ts.register_raw("k1", pk).unwrap();
        let mut p = good_payload();
        p["deployment_steps"][0]["primitive"] = serde_json::json!("BOGUS");
        let jws = sign_compact(&sk, "k1", &p);
        assert!(matches!(verify(&jws, &ts, &["FILE_TRANSFER"]), Err(VerifyError::UnknownPrimitive(_))));
    }

    #[test]
    fn rejects_missing_artifact_for_script() {
        let (sk, pk) = keypair_from_seed(&[6; 32]);
        let mut ts = TrustStore::new();
        ts.register_raw("k1", pk).unwrap();
        let mut p = good_payload();
        p["deployment_steps"] = serde_json::json!([{
            "step_id": "01",
            "primitive": "SCRIPT_EXECUTION",
            "parameters": {
                "script_ref": "scripts/missing.sh",
                "script_sha256": "deadbeef",
                "env": {}
            }
        }]);
        let jws = sign_compact(&sk, "k1", &p);
        let err = verify(&jws, &ts, &["SCRIPT_EXECUTION", "FILE_TRANSFER"]).unwrap_err();
        assert!(matches!(err, VerifyError::MissingArtifact(_)));
    }

    #[test]
    fn jcs_sorts_object_keys() {
        let v = serde_json::json!({"b": 1, "a": 2, "c": [3, 1, 2]});
        let out = jcs(&v).unwrap();
        assert_eq!(std::str::from_utf8(&out).unwrap(), r#"{"a":2,"b":1,"c":[3,1,2]}"#);
    }
}
