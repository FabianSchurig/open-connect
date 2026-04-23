//! Edge-agent state machine (arc42 §5.7.1).
//!
//! Subset implemented for MVP; transitions are exercised by table-driven tests
//! so that adding a state in the future requires updating both the impl and
//! the test table.

#![forbid(unsafe_code)]

use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum AgentState {
    Idle,
    Polling,
    Verifying,
    Executing,
    Rebooting,
    Validating,
    RollingBack,
    Preparing,
    Ready,
    InUse,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Event {
    PollTick,
    DesiredStateReceived,
    VerifyOk,
    VerifyFail,
    StepsOk,
    StepsFail,
    RebootRequested,
    RebootCompleted,
    HealthOk,
    HealthFail,
    ClaimGranted,
    PreparationOk,
    PreparationFail,
    InUseStarted,
    Released,
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum TransitionError {
    #[error("illegal transition {from:?} on event {event:?}")]
    Illegal { from: AgentState, event: Event },
}

pub fn transition(from: AgentState, event: Event) -> Result<AgentState, TransitionError> {
    use AgentState::*;
    use Event::*;
    let next = match (from, event) {
        (Idle, PollTick)             => Polling,
        (Polling, DesiredStateReceived) => Verifying,
        (Polling, PollTick)          => Polling,
        (Verifying, VerifyOk)        => Executing,
        (Verifying, VerifyFail)      => Idle,
        (Executing, StepsOk)         => Validating,
        (Executing, StepsFail)       => RollingBack,
        (Executing, RebootRequested) => Rebooting,
        (Rebooting, RebootCompleted) => Validating,
        (Validating, HealthOk)       => Idle,
        (Validating, HealthFail)     => RollingBack,
        (RollingBack, StepsOk)       => Idle,
        (RollingBack, StepsFail)     => Idle, // give up; agent stays alive
        (Idle, ClaimGranted)         => Preparing,
        (Preparing, PreparationOk)   => Ready,
        (Preparing, PreparationFail) => Idle,
        (Ready, InUseStarted)        => InUse,
        (Ready, Released)            => Idle,
        (InUse, Released)            => Idle,
        _ => return Err(TransitionError::Illegal { from, event }),
    };
    Ok(next)
}

#[cfg(test)]
mod tests {
    use super::*;
    use AgentState::*;
    use Event::*;

    #[test]
    fn happy_update_path() {
        let trail = [
            (Idle, PollTick, Polling),
            (Polling, DesiredStateReceived, Verifying),
            (Verifying, VerifyOk, Executing),
            (Executing, RebootRequested, Rebooting),
            (Rebooting, RebootCompleted, Validating),
            (Validating, HealthOk, Idle),
        ];
        let mut s = Idle;
        for (from, ev, want) in trail {
            assert_eq!(s, from);
            s = transition(s, ev).unwrap();
            assert_eq!(s, want);
        }
    }

    #[test]
    fn claim_lifecycle() {
        let mut s = Idle;
        s = transition(s, ClaimGranted).unwrap();
        assert_eq!(s, Preparing);
        s = transition(s, PreparationOk).unwrap();
        assert_eq!(s, Ready);
        s = transition(s, InUseStarted).unwrap();
        assert_eq!(s, InUse);
        s = transition(s, Released).unwrap();
        assert_eq!(s, Idle);
    }

    #[test]
    fn rollback_after_validate_fail() {
        let mut s = Validating;
        s = transition(s, HealthFail).unwrap();
        assert_eq!(s, RollingBack);
        s = transition(s, StepsOk).unwrap();
        assert_eq!(s, Idle);
    }

    #[test]
    fn illegal_transitions_are_rejected() {
        for (from, event) in [
            (Idle, VerifyOk),
            (Idle, RebootCompleted),
            (Ready, PollTick),
            (InUse, ClaimGranted),
        ] {
            assert!(transition(from, event).is_err(), "expected illegal: {from:?} on {event:?}");
        }
    }
}
