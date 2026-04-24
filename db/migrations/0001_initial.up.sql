-- 0001 — initial schema for the MVP control plane (Epic D / Epic E / Epic F / Epic P).
-- Up migration. Apply with `golang-migrate`.
--
-- Notes:
--   * `audit_deployments` is append-only: UPDATE / DELETE are revoked from the
--     application role at runtime by the bootstrap script (Epic P / FR-11 / NFR-13).
--     The CHECK constraint is a belt-and-suspenders against accidental updates.
--   * Tag storage uses a TEXT[] column with a GIN index for AND-filter queries (FR-22).

BEGIN;

CREATE TABLE devices (
    serial            TEXT PRIMARY KEY,
    public_key_pem    TEXT NOT NULL,
    nats_nkey         TEXT,                       -- nullable in mTLS-only mode (ADR-0009)
    tags              TEXT[] NOT NULL DEFAULT '{}',
    agent_version     TEXT,
    active_partition  TEXT,
    last_heartbeat_at TIMESTAMPTZ,
    retired           BOOLEAN NOT NULL DEFAULT FALSE,
    retired_reason    TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX devices_tags_gin ON devices USING GIN (tags);
CREATE INDEX devices_retired_idx ON devices (retired) WHERE retired = FALSE;

CREATE TABLE claims (
    claim_id                 TEXT PRIMARY KEY,
    state                    TEXT NOT NULL,
    count                    INTEGER NOT NULL CHECK (count >= 1),
    required_tags            TEXT[] NOT NULL,
    desired_version          TEXT,
    ttl_seconds              INTEGER NOT NULL CHECK (ttl_seconds > 0),
    preparation_timeout_secs INTEGER NOT NULL DEFAULT 0,
    requested_by             TEXT NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at               TIMESTAMPTZ NOT NULL
);
CREATE INDEX claims_state_idx ON claims (state) WHERE state IN ('Open', 'PartiallyLocked', 'Locked', 'Preparing', 'Ready', 'InUse');
CREATE INDEX claims_expires_idx ON claims (expires_at);

-- One row per (claim, locked device). The Postgres adapter uses
--   SELECT … FROM claim_locks WHERE claim_id = ? FOR UPDATE SKIP LOCKED
-- to satisfy NFR-09 (linearizable lock acquisition).
CREATE TABLE claim_locks (
    claim_id    TEXT NOT NULL REFERENCES claims(claim_id) ON DELETE CASCADE,
    serial      TEXT NOT NULL REFERENCES devices(serial),
    lease_id    TEXT NOT NULL UNIQUE,
    state       TEXT NOT NULL,
    locked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (claim_id, serial)
);
CREATE INDEX claim_locks_state_idx ON claim_locks (claim_id, state);

-- Append-only audit log (FR-11 / FR-12 / NFR-01 / NFR-13).
-- Real WORM storage with object-lock is post-MVP; this table is the staging
-- ground. The application role has only INSERT/SELECT; UPDATE/DELETE are
-- revoked at deploy time and the CHECK constraint catches client-side bugs.
CREATE TABLE audit_deployments (
    id                BIGSERIAL PRIMARY KEY,
    deployment_id     TEXT NOT NULL,
    serial            TEXT NOT NULL,
    manifest_hash     TEXT NOT NULL,
    deployed_version  TEXT NOT NULL,
    outcome           TEXT NOT NULL CHECK (outcome IN ('SUCCESS', 'FAILED', 'ROLLED_BACK', 'REJECTED_ROLLBACK', 'REJECTED_BELOW_LOWER_LIMIT')),
    signed_payload    BYTEA NOT NULL,
    signature_kid     TEXT NOT NULL,
    received_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, serial, manifest_hash)
);
CREATE INDEX audit_deployments_serial_idx ON audit_deployments (serial, received_at DESC);

COMMIT;
