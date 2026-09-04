-- 0001_platform_domain.sql — create the platform domain tables.
-- Mirrors the V1 specification: UUID primary keys, timestamptz columns,
-- non-negative quota checks and the idempotency uniqueness contracts.

CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY,
    status        varchar(32) NOT NULL,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_users (
    id            uuid PRIMARY KEY,
    username      citext NOT NULL UNIQUE,
    password_hash varchar(512) NOT NULL,
    role          varchar(32) NOT NULL,
    status        varchar(32) NOT NULL,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_admin_users_role_status ON admin_users (role, status);

CREATE TABLE cdk_batches (
    id                    uuid PRIMARY KEY,
    name                  varchar(128) NOT NULL UNIQUE,
    description           varchar(2048),
    default_quota         bigint NOT NULL,
    activation_deadline   timestamptz,
    service_duration_days integer,
    created_by            uuid REFERENCES admin_users (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_cdk_batches_created_at ON cdk_batches (created_at);

CREATE TABLE cdks (
    id                  uuid PRIMARY KEY,
    batch_id            uuid NOT NULL REFERENCES cdk_batches (id) ON DELETE RESTRICT,
    code_prefix         varchar(32) NOT NULL,
    code_hash           bytea NOT NULL UNIQUE,
    status              varchar(32) NOT NULL,
    bound_user_id       uuid UNIQUE REFERENCES users (id) ON DELETE RESTRICT,
    activation_deadline timestamptz,
    expires_at          timestamptz,
    quota_total         bigint NOT NULL CHECK (quota_total >= 0),
    quota_used          bigint NOT NULL CHECK (quota_used >= 0),
    quota_reserved      bigint NOT NULL CHECK (quota_reserved >= 0),
    quota_remaining     bigint NOT NULL CHECK (quota_remaining >= 0),
    activated_at        timestamptz,
    last_used_at        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_cdks_batch_status ON cdks (batch_id, status);
CREATE INDEX ix_cdks_expires_at ON cdks (expires_at);
CREATE INDEX ix_cdks_last_used_at ON cdks (last_used_at);

CREATE TABLE api_keys (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    name         varchar(128) NOT NULL,
    key_prefix   varchar(32) NOT NULL,
    key_last4    varchar(4) NOT NULL,
    key_hash     bytea NOT NULL UNIQUE,
    status       varchar(32) NOT NULL,
    total_calls  bigint NOT NULL,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz
);
CREATE INDEX ix_api_keys_user_status ON api_keys (user_id, status);
CREATE INDEX ix_api_keys_last_used_at ON api_keys (last_used_at);

CREATE TABLE api_calls (
    id                      uuid PRIMARY KEY,
    request_id              varchar(64) NOT NULL UNIQUE,
    operation               varchar(64) NOT NULL,
    idempotency_key_hash    bytea NOT NULL,
    user_id                 uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    cdk_id                  uuid NOT NULL REFERENCES cdks (id) ON DELETE RESTRICT,
    api_key_id              uuid NOT NULL REFERENCES api_keys (id) ON DELETE RESTRICT,
    api_key_name_snapshot   varchar(128) NOT NULL,
    api_key_prefix_snapshot varchar(32) NOT NULL,
    captcha_id              varchar(256) NOT NULL,
    risk_type               varchar(32) NOT NULL,
    status                  varchar(32) NOT NULL,
    http_status             smallint NOT NULL,
    error_code              varchar(64),
    error_summary           varchar(512),
    accepted_at             timestamptz NOT NULL DEFAULT now(),
    completed_at            timestamptz,
    duration_ms             integer,
    quota_reserved          boolean NOT NULL,
    quota_refunded          boolean NOT NULL,
    client_ip_masked        cidr NOT NULL,
    client_ip_hash          bytea NOT NULL,
    user_agent              varchar(512) NOT NULL,
    CONSTRAINT uq_api_calls_idempotency UNIQUE (user_id, operation, idempotency_key_hash)
);
CREATE INDEX ix_api_calls_user_accepted_at ON api_calls (user_id, accepted_at);
CREATE INDEX ix_api_calls_cdk_accepted_at ON api_calls (cdk_id, accepted_at);
CREATE INDEX ix_api_calls_api_key_accepted_at ON api_calls (api_key_id, accepted_at);
CREATE INDEX ix_api_calls_status_accepted_at ON api_calls (status, accepted_at);
CREATE INDEX ix_api_calls_captcha_id ON api_calls (captcha_id);

CREATE TABLE quota_ledger (
    id               uuid PRIMARY KEY,
    cdk_id           uuid NOT NULL REFERENCES cdks (id) ON DELETE RESTRICT,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    api_call_id      uuid REFERENCES api_calls (id) ON DELETE RESTRICT,
    entry_type       varchar(32) NOT NULL,
    available_before bigint NOT NULL,
    delta_available  bigint NOT NULL,
    available_after  bigint NOT NULL,
    used_before      bigint NOT NULL,
    used_after       bigint NOT NULL,
    reserved_before  bigint NOT NULL,
    reserved_after   bigint NOT NULL,
    reason           varchar(256) NOT NULL,
    request_id       varchar(64) NOT NULL,
    actor_type       varchar(32) NOT NULL,
    actor_id         uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_quota_ledger_call_entry_type UNIQUE (api_call_id, entry_type)
);
CREATE INDEX ix_quota_ledger_cdk_created_at ON quota_ledger (cdk_id, created_at);
CREATE INDEX ix_quota_ledger_user_created_at ON quota_ledger (user_id, created_at);
CREATE INDEX ix_quota_ledger_request_id ON quota_ledger (request_id);

CREATE TABLE admin_audit_logs (
    id            uuid PRIMARY KEY,
    admin_user_id uuid NOT NULL REFERENCES admin_users (id) ON DELETE RESTRICT,
    action        varchar(128) NOT NULL,
    target_type   varchar(64) NOT NULL,
    target_id     uuid NOT NULL,
    before_json   jsonb,
    after_json    jsonb,
    reason        varchar(512) NOT NULL,
    ip_masked     varchar(64),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_admin_audit_logs_admin_created_at ON admin_audit_logs (admin_user_id, created_at);
CREATE INDEX ix_admin_audit_logs_target ON admin_audit_logs (target_type, target_id);

CREATE TABLE system_settings (
    setting_key varchar(128) PRIMARY KEY,
    value_json  jsonb NOT NULL,
    updated_by  uuid REFERENCES admin_users (id) ON DELETE SET NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_sessions (
    id           uuid PRIMARY KEY,
    admin_user_id uuid NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
