# Use Cases

This catalogue holds the user-goal level use cases that drive every requirement, every component, and every architectural decision in the platform.

## Index

| ID | Title | Primary Actor | Status |
|----|-------|---------------|--------|
| [UC-01](UC-01-ab-ota-medical.md) | Modular A/B OTA Update for Medical Devices | Release Manager | Baseline |
| [UC-02](UC-02-ros2-modular-deploy.md) | ROS2 Autonomous AI Validation (Modular Deployment) | AI / Robotics Engineer | Baseline |
| [UC-03](UC-03-cicd-hil-claiming.md) | CI/CD Pipeline HIL Device Claiming | CI/CD Pipeline (automated) | Baseline |

## Use Case Template

All use cases follow the same template:

1. **Brief** — one-paragraph elevator description.
2. **Stakeholders & Interests** — who cares and why.
3. **Preconditions** — what must be true before the trigger.
4. **Trigger** — the event that initiates the flow.
5. **Main Success Flow** — happy path, numbered steps.
6. **Alternative & Error Flows** — Alt-N for valid alternatives, Err-N for failures.
7. **Derived Requirements** — back-references to FR/NFR plus *new* requirements surfaced.
8. **Related Architecture** — links into arc42 sections and ADRs.
9. **Open Issues** — known unknowns.

New requirements derived from these walkthroughs are reflected in [`../requirements/functional.md`](../requirements/functional.md) and [`../requirements/non-functional.md`](../requirements/non-functional.md), and rolled up in the [traceability matrix](../requirements/traceability-matrix.md).
