//! NATS transport abstraction for the edge agent.
//!
//! The trait `NatsClient` lets the agent be tested deterministically against
//! an in-process implementation while the production code uses `async-nats`.
//! The real adapter lives in the agent-cli main crate (or a future
//! `agent-nats-async` impl crate) so that this crate stays free of TLS deps.

#![forbid(unsafe_code)]

use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use parking_lot::Mutex;
use thiserror::Error;
use tokio::sync::mpsc;

#[derive(Debug, Error)]
pub enum NatsError {
    #[error("transport: {0}")]
    Transport(String),
    #[error("no responder for subject `{0}`")]
    NoResponder(String),
}

#[derive(Debug, Clone)]
pub struct Message {
    pub subject: String,
    pub payload: Vec<u8>,
}

#[async_trait]
pub trait NatsClient: Send + Sync {
    async fn publish(&self, subject: &str, payload: &[u8]) -> Result<(), NatsError>;
    async fn request(&self, subject: &str, payload: &[u8]) -> Result<Vec<u8>, NatsError>;
    async fn subscribe(&self, subject: &str)
        -> Result<mpsc::UnboundedReceiver<Message>, NatsError>;
}

/// In-memory bus used by tests and demos.
#[derive(Default)]
pub struct MemBus {
    inner: Mutex<MemInner>,
}

#[derive(Default)]
struct MemInner {
    subs: HashMap<String, Vec<mpsc::UnboundedSender<Message>>>,
    responders: HashMap<String, Arc<Responder>>,
}

type Responder = dyn Fn(&[u8]) -> Vec<u8> + Send + Sync;

impl MemBus {
    pub fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// Register a synchronous responder for a subject.
    pub fn respond_with(
        &self,
        subject: &str,
        f: impl Fn(&[u8]) -> Vec<u8> + Send + Sync + 'static,
    ) {
        self.inner
            .lock()
            .responders
            .insert(subject.into(), Arc::new(f));
    }
}

#[async_trait]
impl NatsClient for MemBus {
    async fn publish(&self, subject: &str, payload: &[u8]) -> Result<(), NatsError> {
        let subs = {
            let inner = self.inner.lock();
            inner.subs.get(subject).cloned().unwrap_or_default()
        };
        for s in subs {
            let _ = s.send(Message {
                subject: subject.into(),
                payload: payload.to_vec(),
            });
        }
        Ok(())
    }
    async fn request(&self, subject: &str, payload: &[u8]) -> Result<Vec<u8>, NatsError> {
        let resp = {
            let inner = self.inner.lock();
            inner.responders.get(subject).cloned()
        };
        let f = resp.ok_or_else(|| NatsError::NoResponder(subject.into()))?;
        Ok(f(payload))
    }
    async fn subscribe(
        &self,
        subject: &str,
    ) -> Result<mpsc::UnboundedReceiver<Message>, NatsError> {
        let (tx, rx) = mpsc::unbounded_channel();
        self.inner
            .lock()
            .subs
            .entry(subject.into())
            .or_default()
            .push(tx);
        Ok(rx)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn publish_then_subscribe_delivers() {
        let bus = MemBus::new();
        let mut rx = bus.subscribe("device.A.heartbeat").await.unwrap();
        bus.publish("device.A.heartbeat", b"hi").await.unwrap();
        let m = rx.recv().await.unwrap();
        assert_eq!(m.payload, b"hi".to_vec());
    }

    #[tokio::test]
    async fn request_uses_responder() {
        let bus = MemBus::new();
        bus.respond_with("claim.lock.x", |p| {
            let mut out = b"ack:".to_vec();
            out.extend_from_slice(p);
            out
        });
        let resp = bus.request("claim.lock.x", b"DEV-1").await.unwrap();
        assert_eq!(resp, b"ack:DEV-1");
    }
}
