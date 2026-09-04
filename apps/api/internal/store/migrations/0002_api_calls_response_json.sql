-- 0002_api_calls_response_json.sql
-- Store the normalized success response so idempotent replays can serve the
-- original result without a second solver call (V1 spec 7.3). Only
-- non-sensitive solve payloads are stored - the service key and API key
-- secrets never enter this column.

ALTER TABLE api_calls ADD COLUMN response_json jsonb
