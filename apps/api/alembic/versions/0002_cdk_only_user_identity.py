"""remove end-user passwords and usernames

Revision ID: 0002_cdk_only_user_identity
Revises: 0001_platform_domain
Create Date: 2026-09-04 16:40:00
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "0002_cdk_only_user_identity"
down_revision: str | Sequence[str] | None = "0001_platform_domain"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint("users_username_key", "users", type_="unique")
    op.drop_column("users", "username")
    op.drop_column("users", "password_hash")


def downgrade() -> None:
    op.add_column("users", sa.Column("password_hash", sa.String(length=512), nullable=True))
    op.add_column("users", sa.Column("username", postgresql.CITEXT(), nullable=True))
    op.create_unique_constraint("users_username_key", "users", ["username"])
