-- 0004_admin_adjustments.sql
-- Operator quota adjustments may target CDKs that are not yet bound to a
-- user, so the ledger owner reference becomes nullable for those entries.

ALTER TABLE quota_ledger ALTER COLUMN user_id DROP NOT NULL;