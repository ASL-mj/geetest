ALTER TABLE api_keys ADD COLUMN secret_ciphertext bytea;
ALTER TABLE api_keys ADD COLUMN quota_limit BIGINT;
ALTER TABLE api_keys ADD COLUMN allowed_ips TEXT;
ALTER TABLE cdks ADD COLUMN code_ciphertext bytea;
ALTER TABLE cdks ADD COLUMN remark TEXT;
