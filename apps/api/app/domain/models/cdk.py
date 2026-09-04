from datetime import datetime
from uuid import UUID, uuid4

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    LargeBinary,
    String,
    func,
)
from sqlalchemy.orm import Mapped, mapped_column

from app.db.base import Base


class CdkBatch(Base):
    __tablename__ = "cdk_batches"
    __table_args__ = (Index("ix_cdk_batches_created_at", "created_at"),)

    id: Mapped[UUID] = mapped_column(primary_key=True, default=uuid4)
    name: Mapped[str] = mapped_column(String(128), unique=True, nullable=False)
    description: Mapped[str | None] = mapped_column(String(2048))
    default_quota: Mapped[int] = mapped_column(BigInteger, nullable=False)
    activation_deadline: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    service_duration_days: Mapped[int | None] = mapped_column(nullable=True)
    created_by: Mapped[UUID | None] = mapped_column(
        ForeignKey("admin_users.id", ondelete="SET NULL")
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )


class Cdk(Base):
    __tablename__ = "cdks"
    __table_args__ = (
        CheckConstraint("quota_total >= 0", name="ck_cdks_quota_total_nonnegative"),
        CheckConstraint("quota_used >= 0", name="ck_cdks_quota_used_nonnegative"),
        CheckConstraint("quota_reserved >= 0", name="ck_cdks_quota_reserved_nonnegative"),
        CheckConstraint("quota_remaining >= 0", name="ck_cdks_quota_remaining_nonnegative"),
        Index("ix_cdks_batch_status", "batch_id", "status"),
        Index("ix_cdks_expires_at", "expires_at"),
        Index("ix_cdks_last_used_at", "last_used_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True, default=uuid4)
    batch_id: Mapped[UUID] = mapped_column(
        ForeignKey("cdk_batches.id", ondelete="RESTRICT"), nullable=False
    )
    code_prefix: Mapped[str] = mapped_column(String(32), nullable=False)
    code_hash: Mapped[bytes] = mapped_column(LargeBinary(64), unique=True, nullable=False)
    status: Mapped[str] = mapped_column(String(32), default="UNACTIVATED", nullable=False)
    bound_user_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("users.id", ondelete="RESTRICT"), unique=True
    )
    activation_deadline: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    expires_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    quota_total: Mapped[int] = mapped_column(BigInteger, nullable=False)
    quota_used: Mapped[int] = mapped_column(BigInteger, default=0, nullable=False)
    quota_reserved: Mapped[int] = mapped_column(BigInteger, default=0, nullable=False)
    quota_remaining: Mapped[int] = mapped_column(BigInteger, nullable=False)
    activated_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    last_used_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )
