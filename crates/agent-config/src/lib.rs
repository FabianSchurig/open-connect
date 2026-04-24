//! Agent TOML configuration loader.

use serde::Deserialize;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("io: {0}")]
    Io(#[from] std::io::Error),
    #[error("toml: {0}")]
    Toml(#[from] toml::de::Error),
}

#[derive(Debug, Deserialize, Clone)]
pub struct AgentConfig {
    pub serial: String,
    pub control_plane_url: String,
    pub nats_url: Option<String>,
    pub trust_store_dir: String,
    pub spool_dir: String,
    pub anti_rollback_path: String,
    #[serde(default = "default_poll_interval")]
    pub poll_interval_seconds: u64,
    #[serde(default)]
    pub tags: Vec<String>,
}

fn default_poll_interval() -> u64 {
    30
}

impl AgentConfig {
    pub fn from_path(p: impl AsRef<std::path::Path>) -> Result<Self, ConfigError> {
        let body = std::fs::read_to_string(p)?;
        Ok(toml::from_str(&body)?)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn parses_minimal_config() {
        let toml = r#"
serial = "DEV-1"
control_plane_url = "https://control.example/"
trust_store_dir = "/etc/ota/trust-store"
spool_dir = "/var/lib/ota/spool"
anti_rollback_path = "/var/lib/persistent/anti-rollback.json"
tags = ["profile-a", "x86"]
"#;
        let c: AgentConfig = toml::from_str(toml).unwrap();
        assert_eq!(c.serial, "DEV-1");
        assert_eq!(c.poll_interval_seconds, 30);
        assert_eq!(c.tags, vec!["profile-a".to_string(), "x86".into()]);
    }
}
