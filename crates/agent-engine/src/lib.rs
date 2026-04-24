//! Config-driven execution engine (arc42 §5.4.3 / ADR-0008 / FR-01, FR-02).
//!
//! The engine walks `deployment_steps[]` in order, dispatching each step to
//! the registered `Primitive`. Halt-and-rollback semantics on the first
//! non-success result (FR-02). The rollback steps are walked best-effort:
//! a failure inside rollback is logged but does not re-trigger rollback.

#![forbid(unsafe_code)]

use std::sync::Arc;

use agent_primitives::{PrimitiveError, PrimitiveRegistry, StepContext, StepResult};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use tracing::{error, info, warn};

#[derive(Debug, Error)]
pub enum EngineError {
    #[error("step `{step_id}` failed (primitive {primitive}): {source}")]
    StepFailed {
        step_id: String,
        primitive: String,
        #[source]
        source: PrimitiveError,
    },
    #[error("step `{step_id}` returned non-success")]
    StepNonZero { step_id: String, result: StepResult },
    #[error("primitive `{0}` not registered")]
    UnknownPrimitive(String),
}

/// One executable step. This mirrors the manifest shape (subset).
#[derive(Debug, Deserialize, Serialize, Clone)]
pub struct Step {
    pub step_id: String,
    pub primitive: String,
    pub parameters: serde_json::Value,
    #[serde(default)]
    pub continue_on_error: bool,
}

/// Outcome — either Success(all step results) or Failure(triggered rollback).
pub enum RunOutcome {
    Success(Vec<StepResult>),
    Failed {
        completed: Vec<StepResult>,
        failure: EngineError,
        rollback: Vec<StepResult>,
    },
}

pub struct Engine {
    pub registry: Arc<PrimitiveRegistry>,
}

impl Engine {
    pub fn new(registry: Arc<PrimitiveRegistry>) -> Self {
        Self { registry }
    }

    /// Run a manifest's deployment steps, with optional rollback steps.
    pub async fn run(
        &self,
        deployment_steps: &[Step],
        rollback_steps: &[Step],
        ctx: &StepContext,
    ) -> RunOutcome {
        let mut completed = Vec::with_capacity(deployment_steps.len());
        for step in deployment_steps {
            match self.run_one(step, ctx).await {
                Ok(r) if r.success => {
                    info!(step_id = %step.step_id, primitive = %step.primitive, "step ok");
                    completed.push(r);
                }
                Ok(r) => {
                    if step.continue_on_error {
                        warn!(step_id = %step.step_id, "non-success but continue_on_error=true");
                        completed.push(r);
                        continue;
                    }
                    error!(step_id = %step.step_id, exit_code = r.exit_code, "step non-zero -> rollback");
                    let failure = EngineError::StepNonZero {
                        step_id: step.step_id.clone(),
                        result: r.clone(),
                    };
                    completed.push(r);
                    let rollback = self.rollback(rollback_steps, ctx).await;
                    return RunOutcome::Failed {
                        completed,
                        failure,
                        rollback,
                    };
                }
                Err(e) => {
                    error!(step_id = %step.step_id, error = %e, "step error -> rollback");
                    let failure = e;
                    let rollback = self.rollback(rollback_steps, ctx).await;
                    return RunOutcome::Failed {
                        completed,
                        failure,
                        rollback,
                    };
                }
            }
        }
        RunOutcome::Success(completed)
    }

    async fn run_one(&self, step: &Step, ctx: &StepContext) -> Result<StepResult, EngineError> {
        let p = self
            .registry
            .get(&step.primitive)
            .ok_or_else(|| EngineError::UnknownPrimitive(step.primitive.clone()))?;
        p.execute(&step.step_id, &step.parameters, ctx)
            .await
            .map_err(|e| EngineError::StepFailed {
                step_id: step.step_id.clone(),
                primitive: step.primitive.clone(),
                source: e,
            })
    }

    async fn rollback(&self, steps: &[Step], ctx: &StepContext) -> Vec<StepResult> {
        let mut out = Vec::with_capacity(steps.len());
        for s in steps {
            match self.run_one(s, ctx).await {
                Ok(r) => out.push(r),
                Err(e) => {
                    error!(step_id = %s.step_id, error = %e, "rollback step failed (best-effort)");
                }
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use agent_primitives::{MemFetcher, Primitive, PrimitiveError};
    use async_trait::async_trait;
    use std::collections::HashMap;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;
    use tempfile::tempdir;

    struct CountingOk(Arc<AtomicUsize>);
    #[async_trait]
    impl Primitive for CountingOk {
        fn name(&self) -> &'static str {
            "OK"
        }
        async fn execute(
            &self,
            step_id: &str,
            _: &serde_json::Value,
            _: &StepContext,
        ) -> Result<StepResult, PrimitiveError> {
            self.0.fetch_add(1, Ordering::SeqCst);
            Ok(StepResult {
                step_id: step_id.into(),
                primitive: "OK".into(),
                success: true,
                exit_code: 0,
                stdout: String::new(),
                stderr: String::new(),
                stdout_truncated: false,
                stderr_truncated: false,
                duration_ms: 0,
            })
        }
    }

    struct AlwaysFail;
    #[async_trait]
    impl Primitive for AlwaysFail {
        fn name(&self) -> &'static str {
            "FAIL"
        }
        async fn execute(
            &self,
            step_id: &str,
            _: &serde_json::Value,
            _: &StepContext,
        ) -> Result<StepResult, PrimitiveError> {
            Ok(StepResult {
                step_id: step_id.into(),
                primitive: "FAIL".into(),
                success: false,
                exit_code: 7,
                stdout: String::new(),
                stderr: "boom".into(),
                stdout_truncated: false,
                stderr_truncated: false,
                duration_ms: 0,
            })
        }
    }

    fn ctx() -> StepContext {
        StepContext {
            spool_dir: tempdir().unwrap().path().to_path_buf(),
            deployment_id: "d".into(),
            serial: "s".into(),
            fetcher: Arc::new(MemFetcher(HashMap::new())),
        }
    }

    #[tokio::test]
    async fn happy_path_runs_all_steps() {
        let counter = Arc::new(AtomicUsize::new(0));
        let mut reg = PrimitiveRegistry::new();
        reg.register(Arc::new(CountingOk(counter.clone())));
        let e = Engine::new(Arc::new(reg));
        let steps = vec![
            Step {
                step_id: "1".into(),
                primitive: "OK".into(),
                parameters: serde_json::json!({}),
                continue_on_error: false,
            },
            Step {
                step_id: "2".into(),
                primitive: "OK".into(),
                parameters: serde_json::json!({}),
                continue_on_error: false,
            },
        ];
        match e.run(&steps, &[], &ctx()).await {
            RunOutcome::Success(rs) => assert_eq!(rs.len(), 2),
            _ => panic!("expected success"),
        }
        assert_eq!(counter.load(Ordering::SeqCst), 2);
    }

    #[tokio::test]
    async fn failure_triggers_rollback_and_halts() {
        let counter = Arc::new(AtomicUsize::new(0));
        let mut reg = PrimitiveRegistry::new();
        reg.register(Arc::new(CountingOk(counter.clone())));
        reg.register(Arc::new(AlwaysFail));
        let e = Engine::new(Arc::new(reg));
        let steps = vec![
            Step {
                step_id: "1".into(),
                primitive: "OK".into(),
                parameters: serde_json::json!({}),
                continue_on_error: false,
            },
            Step {
                step_id: "2".into(),
                primitive: "FAIL".into(),
                parameters: serde_json::json!({}),
                continue_on_error: false,
            },
            Step {
                step_id: "3".into(),
                primitive: "OK".into(),
                parameters: serde_json::json!({}),
                continue_on_error: false,
            },
        ];
        let rb = vec![Step {
            step_id: "rb1".into(),
            primitive: "OK".into(),
            parameters: serde_json::json!({}),
            continue_on_error: false,
        }];
        match e.run(&steps, &rb, &ctx()).await {
            RunOutcome::Failed {
                completed,
                failure,
                rollback,
            } => {
                assert_eq!(completed.len(), 2); // step 1 + the failed step's report
                assert!(matches!(failure, EngineError::StepNonZero { .. }));
                assert_eq!(rollback.len(), 1);
                // Step 3 must NOT run.
                assert_eq!(counter.load(Ordering::SeqCst), 2 /* step 1 + rb1 */);
            }
            _ => panic!("expected failure"),
        }
    }

    #[tokio::test]
    async fn unknown_primitive_is_engine_error() {
        let reg = PrimitiveRegistry::new();
        let e = Engine::new(Arc::new(reg));
        let steps = vec![Step {
            step_id: "1".into(),
            primitive: "GHOST".into(),
            parameters: serde_json::json!({}),
            continue_on_error: false,
        }];
        match e.run(&steps, &[], &ctx()).await {
            RunOutcome::Failed { failure, .. } => {
                assert!(matches!(failure, EngineError::UnknownPrimitive(_)))
            }
            _ => panic!(),
        }
    }
}
