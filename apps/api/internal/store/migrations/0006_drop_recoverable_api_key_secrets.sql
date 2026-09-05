-- API Key plaintext is creation-time-only. Existing sealed copies are no
-- longer needed for authentication and must not remain recoverable.
ALTER TABLE api_keys DROP COLUMN secret_ciphertext;
