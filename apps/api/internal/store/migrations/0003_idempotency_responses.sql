-- 0003_idempotency_responses.sql
-- Move the idempotent replay payload out of api_calls. The call log must not
-- carry the full solver result (V1 spec 12), while replay needs the original
-- normalized body (V1 spec 7.3) - a dedicated table keeps both properties.

CREATE TABLE idempotency_responses (
    api_call_id uuid PRIMARY KEY REFERENCES api_calls (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    response_json jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_idempotency_responses_user ON idempotency_responses (user_id);

ALTER TABLE api_calls DROP COLUMN response_json;