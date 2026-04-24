//! Open-Connect edge agent (MVP scaffolding).
//!
//! This binary wires the supervisor loop. In MVP it boots the in-memory NATS
//! bus and primitive registry so it builds without the full TLS/async-nats
//! stack; the production async-nats adapter ships in a follow-up PR.

use std::sync::Arc;

use agent_core::{transition, AgentState, Event};
use agent_engine::Engine;
use agent_primitives::{FileTransfer, MemFetcher, PrimitiveRegistry, Reboot, ScriptExecution};
use anyhow::Result;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let mut reg = PrimitiveRegistry::new();
    reg.register(Arc::new(FileTransfer));
    reg.register(Arc::new(ScriptExecution));
    reg.register(Arc::new(Reboot {
        sentinel_path: "/var/lib/ota/reboot.json".into(),
    }));
    let _engine = Engine::new(Arc::new(reg));
    let _fetcher: Arc<dyn agent_primitives::Fetcher> = Arc::new(MemFetcher(Default::default()));

    // Demonstrate state-machine wiring builds & runs.
    let s = transition(AgentState::Idle, Event::PollTick).expect("Idle->Polling");
    tracing::info!(state = ?s, "agent booted (MVP scaffold)");
    Ok(())
}
