from datetime import datetime
from uuid import UUID, uuid4

from sqlalchemy import BigInteger, DateTime, ForeignKey, Index, String, UniqueConstraint, func
from sqlalchemy.orm import Mapped, mapped_column

from app.db.base import Base


class QuotaLedger(Base):
    __tablename__ = "quota_ledger"
    __table_args__ = (
        UniqueConstraint("api_call_id", "entry_type", name="uq_quota_ledger_call_entry_type"),
        Index("ix_quota_ledger_cdk_created_at", "cdk_id", "created_at"),
        Index("ix_quota_ledger_user_created_at", "user_id", "created_at"),
        Index("ix_quota_ledger_request_id", "request_id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True, default=uuid4)
    cdk_id: Mapped[UUID] = mapped_column(
        ForeignKey("cdks.id", ondelete="RESTRICT"), nullable=False
    )
    user_id: Mapped[UUID] = mapped_column(
        ForeignKey("users.id", ondelete="RESTRICT"), nullable=False
    )
    api_call_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("api_calls.id", ondelete="RESTRICT")
    )
    entry_type: Mapped[str] = mapped_column(String(32), nullable=False)
    available_before: Mapped[int] = mapped_column(BigInteger, nullable=False)
    delta_available: Mapped[int] = mapped_column(BigInteger, nullable=False)
    available_after: Mapped[int] = mapped_column(BigInteger, nullable=False)
    used_before: Mapped[int] = mapped_column(BigInteger, nullable=False)
    used_after: Mapped[int] = mapped_column(BigInteger, nullable=False)
    reserved_before: Mapped[int] = mapped_column(BigInteger, nullable=False)
    reserved_after: Mapped[int] = mapped_column(BigInteger, nullable=False)
    reason: Mapped[str] = mapped_column(String(256), nullable=False)
    request_id: Mapped[str] = mapped_column(String(64), nullable=False)
    actor_type: Mapped[str] = mapped_column(String(32), nullable=False)
    actor_id: Mapped[UUID | None] = mapped_column()
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )
