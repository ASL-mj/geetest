"""create the platform domain tables

Revision ID: 0001_platform_domain
Revises:
Create Date: 2026-09-04 14:00:00
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "0001_platform_domain"
down_revision: str | Sequence[str] | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

metadata = sa.MetaData()

users = sa.Table(
    "users",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("username", postgresql.CITEXT(), nullable=False, unique=True),
    sa.Column("password_hash", sa.String(512), nullable=False),
    sa.Column("status", sa.String(32), nullable=False),
    sa.Column("last_login_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.Column(
        "updated_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)

admin_users = sa.Table(
    "admin_users",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("username", postgresql.CITEXT(), nullable=False, unique=True),
    sa.Column("password_hash", sa.String(512), nullable=False),
    sa.Column("role", sa.String(32), nullable=False),
    sa.Column("status", sa.String(32), nullable=False),
    sa.Column("last_login_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.Column(
        "updated_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)
sa.Index("ix_admin_users_role_status", admin_users.c.role, admin_users.c.status)

cdk_batches = sa.Table(
    "cdk_batches",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("name", sa.String(128), nullable=False, unique=True),
    sa.Column("description", sa.String(2048)),
    sa.Column("default_quota", sa.BigInteger(), nullable=False),
    sa.Column("activation_deadline", sa.DateTime(timezone=True)),
    sa.Column("service_duration_days", sa.Integer()),
    sa.Column("created_by", sa.Uuid(), sa.ForeignKey("admin_users.id", ondelete="SET NULL")),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)
sa.Index("ix_cdk_batches_created_at", cdk_batches.c.created_at)

cdks = sa.Table(
    "cdks",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column(
        "batch_id", sa.Uuid(), sa.ForeignKey("cdk_batches.id", ondelete="RESTRICT"), nullable=False
    ),
    sa.Column("code_prefix", sa.String(32), nullable=False),
    sa.Column("code_hash", sa.LargeBinary(64), nullable=False, unique=True),
    sa.Column("status", sa.String(32), nullable=False),
    sa.Column(
        "bound_user_id", sa.Uuid(), sa.ForeignKey("users.id", ondelete="RESTRICT"), unique=True
    ),
    sa.Column("activation_deadline", sa.DateTime(timezone=True)),
    sa.Column("expires_at", sa.DateTime(timezone=True)),
    sa.Column("quota_total", sa.BigInteger(), nullable=False),
    sa.Column("quota_used", sa.BigInteger(), nullable=False),
    sa.Column("quota_reserved", sa.BigInteger(), nullable=False),
    sa.Column("quota_remaining", sa.BigInteger(), nullable=False),
    sa.Column("activated_at", sa.DateTime(timezone=True)),
    sa.Column("last_used_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.Column(
        "updated_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.CheckConstraint("quota_total >= 0", name="ck_cdks_quota_total_nonnegative"),
    sa.CheckConstraint("quota_used >= 0", name="ck_cdks_quota_used_nonnegative"),
    sa.CheckConstraint("quota_reserved >= 0", name="ck_cdks_quota_reserved_nonnegative"),
    sa.CheckConstraint("quota_remaining >= 0", name="ck_cdks_quota_remaining_nonnegative"),
)
sa.Index("ix_cdks_batch_status", cdks.c.batch_id, cdks.c.status)
sa.Index("ix_cdks_expires_at", cdks.c.expires_at)
sa.Index("ix_cdks_last_used_at", cdks.c.last_used_at)

api_keys = sa.Table(
    "api_keys",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("user_id", sa.Uuid(), sa.ForeignKey("users.id", ondelete="RESTRICT"), nullable=False),
    sa.Column("name", sa.String(128), nullable=False),
    sa.Column("key_prefix", sa.String(32), nullable=False),
    sa.Column("key_last4", sa.String(4), nullable=False),
    sa.Column("key_hash", sa.LargeBinary(64), nullable=False, unique=True),
    sa.Column("status", sa.String(32), nullable=False),
    sa.Column("total_calls", sa.BigInteger(), nullable=False),
    sa.Column("last_used_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.Column("revoked_at", sa.DateTime(timezone=True)),
)
sa.Index("ix_api_keys_user_status", api_keys.c.user_id, api_keys.c.status)
sa.Index("ix_api_keys_last_used_at", api_keys.c.last_used_at)

api_calls = sa.Table(
    "api_calls",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("request_id", sa.String(64), nullable=False, unique=True),
    sa.Column("operation", sa.String(64), nullable=False),
    sa.Column("idempotency_key_hash", sa.LargeBinary(64), nullable=False),
    sa.Column("user_id", sa.Uuid(), sa.ForeignKey("users.id", ondelete="RESTRICT"), nullable=False),
    sa.Column("cdk_id", sa.Uuid(), sa.ForeignKey("cdks.id", ondelete="RESTRICT"), nullable=False),
    sa.Column(
        "api_key_id", sa.Uuid(), sa.ForeignKey("api_keys.id", ondelete="RESTRICT"), nullable=False
    ),
    sa.Column("api_key_name_snapshot", sa.String(128), nullable=False),
    sa.Column("api_key_prefix_snapshot", sa.String(32), nullable=False),
    sa.Column("captcha_id", sa.String(256), nullable=False),
    sa.Column("risk_type", sa.String(32), nullable=False),
    sa.Column("status", sa.String(32), nullable=False),
    sa.Column("http_status", sa.SmallInteger(), nullable=False),
    sa.Column("error_code", sa.String(64)),
    sa.Column("error_summary", sa.String(512)),
    sa.Column(
        "accepted_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.Column("completed_at", sa.DateTime(timezone=True)),
    sa.Column("duration_ms", sa.Integer()),
    sa.Column("quota_reserved", sa.Boolean(), nullable=False),
    sa.Column("quota_refunded", sa.Boolean(), nullable=False),
    sa.Column("client_ip_masked", postgresql.CIDR(), nullable=False),
    sa.Column("client_ip_hash", sa.LargeBinary(64), nullable=False),
    sa.Column("user_agent", sa.String(512), nullable=False),
    sa.UniqueConstraint(
        "user_id", "operation", "idempotency_key_hash", name="uq_api_calls_idempotency"
    ),
)
sa.Index("ix_api_calls_user_accepted_at", api_calls.c.user_id, api_calls.c.accepted_at)
sa.Index("ix_api_calls_cdk_accepted_at", api_calls.c.cdk_id, api_calls.c.accepted_at)
sa.Index("ix_api_calls_api_key_accepted_at", api_calls.c.api_key_id, api_calls.c.accepted_at)
sa.Index("ix_api_calls_status_accepted_at", api_calls.c.status, api_calls.c.accepted_at)
sa.Index("ix_api_calls_captcha_id", api_calls.c.captcha_id)

quota_ledger = sa.Table(
    "quota_ledger",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("cdk_id", sa.Uuid(), sa.ForeignKey("cdks.id", ondelete="RESTRICT"), nullable=False),
    sa.Column("user_id", sa.Uuid(), sa.ForeignKey("users.id", ondelete="RESTRICT"), nullable=False),
    sa.Column("api_call_id", sa.Uuid(), sa.ForeignKey("api_calls.id", ondelete="RESTRICT")),
    sa.Column("entry_type", sa.String(32), nullable=False),
    sa.Column("available_before", sa.BigInteger(), nullable=False),
    sa.Column("delta_available", sa.BigInteger(), nullable=False),
    sa.Column("available_after", sa.BigInteger(), nullable=False),
    sa.Column("used_before", sa.BigInteger(), nullable=False),
    sa.Column("used_after", sa.BigInteger(), nullable=False),
    sa.Column("reserved_before", sa.BigInteger(), nullable=False),
    sa.Column("reserved_after", sa.BigInteger(), nullable=False),
    sa.Column("reason", sa.String(256), nullable=False),
    sa.Column("request_id", sa.String(64), nullable=False),
    sa.Column("actor_type", sa.String(32), nullable=False),
    sa.Column("actor_id", sa.Uuid()),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
    sa.UniqueConstraint("api_call_id", "entry_type", name="uq_quota_ledger_call_entry_type"),
)
sa.Index("ix_quota_ledger_cdk_created_at", quota_ledger.c.cdk_id, quota_ledger.c.created_at)
sa.Index("ix_quota_ledger_user_created_at", quota_ledger.c.user_id, quota_ledger.c.created_at)
sa.Index("ix_quota_ledger_request_id", quota_ledger.c.request_id)

admin_audit_logs = sa.Table(
    "admin_audit_logs",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column(
        "admin_user_id",
        sa.Uuid(),
        sa.ForeignKey("admin_users.id", ondelete="RESTRICT"),
        nullable=False,
    ),
    sa.Column("action", sa.String(128), nullable=False),
    sa.Column("target_type", sa.String(64), nullable=False),
    sa.Column("target_id", sa.Uuid(), nullable=False),
    sa.Column("before_json", postgresql.JSONB()),
    sa.Column("after_json", postgresql.JSONB()),
    sa.Column("reason", sa.String(512), nullable=False),
    sa.Column("ip_masked", sa.String(64)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)
sa.Index(
    "ix_admin_audit_logs_admin_created_at",
    admin_audit_logs.c.admin_user_id,
    admin_audit_logs.c.created_at,
)
sa.Index("ix_admin_audit_logs_target", admin_audit_logs.c.target_type, admin_audit_logs.c.target_id)

system_settings = sa.Table(
    "system_settings",
    metadata,
    sa.Column("setting_key", sa.String(128), primary_key=True),
    sa.Column("value_json", postgresql.JSONB(), nullable=False),
    sa.Column("updated_by", sa.Uuid(), sa.ForeignKey("admin_users.id", ondelete="SET NULL")),
    sa.Column(
        "updated_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)
user_sessions = sa.Table(
    "user_sessions",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column("user_id", sa.Uuid(), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
    sa.Column("token_hash", sa.LargeBinary(64), nullable=False, unique=True),
    sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
    sa.Column("revoked_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)
admin_sessions = sa.Table(
    "admin_sessions",
    metadata,
    sa.Column("id", sa.Uuid(), primary_key=True),
    sa.Column(
        "admin_user_id",
        sa.Uuid(),
        sa.ForeignKey("admin_users.id", ondelete="CASCADE"),
        nullable=False,
    ),
    sa.Column("token_hash", sa.LargeBinary(64), nullable=False, unique=True),
    sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
    sa.Column("revoked_at", sa.DateTime(timezone=True)),
    sa.Column(
        "created_at", sa.DateTime(timezone=True), server_default=sa.text("now()"), nullable=False
    ),
)


def upgrade() -> None:
    op.execute("CREATE EXTENSION IF NOT EXISTS citext")
    metadata.create_all(bind=op.get_bind(), checkfirst=False)
    op.execute(
        """
        DO $$
        BEGIN
            IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'geetest_platform_app') THEN
                REVOKE UPDATE, DELETE ON TABLE quota_ledger FROM geetest_platform_app;
            END IF;
        END
        $$;
        """
    )


def downgrade() -> None:
    metadata.drop_all(bind=op.get_bind(), checkfirst=False)
    op.execute("DROP EXTENSION IF EXISTS citext")
