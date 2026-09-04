from datetime import datetime
from uuid import UUID, uuid4

from sqlalchemy import (
    Boolean,
    DateTime,
    ForeignKey,
    Index,
    LargeBinary,
    SmallInteger,
    String,
    UniqueConstraint,
    func,
)
from sqlalchemy.dialects.postgresql import CIDR
from sqlalchemy.orm import Mapped, mapped_column

from app.db.base import Base


class ApiCall(Base):
    __tablename__ = "api_calls"
    __table_args__ = (
        UniqueConstraint(
            "user_id", "operation", "idempotency_key_hash", name="uq_api_calls_idempotency"
        ),
        Index("ix_api_calls_user_accepted_at", "user_id", "accepted_at"),
        Index("ix_api_calls_cdk_accepted_at", "cdk_id", "accepted_at"),
        Index("ix_api_calls_api_key_accepted_at", "api_key_id", "accepted_at"),
        Index("ix_api_calls_status_accepted_at", "status", "accepted_at"),
        Index("ix_api_calls_captcha_id", "captcha_id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True, default=uuid4)
    request_id: Mapped[str] = mapped_column(String(64), unique=True, nullable=False)
    operation: Mapped[str] = mapped_column(String(64), nullable=False)
    idempotency_key_hash: Mapped[bytes] = mapped_column(LargeBinary(64), nullable=False)
    user_id: Mapped[UUID] = mapped_column(
        ForeignKey("users.id", ondelete="RESTRICT"), nullable=False
    )
    cdk_id: Mapped[UUID] = mapped_column(
        ForeignKey("cdks.id", ondelete="RESTRICT"), nullable=False
    )
    api_key_id: Mapped[UUID] = mapped_column(
        ForeignKey("api_keys.id", ondelete="RESTRICT"), nullable=False
    )
    api_key_name_snapshot: Mapped[str] = mapped_column(String(128), nullable=False)
    api_key_prefix_snapshot: Mapped[str] = mapped_column(String(32), nullable=False)
    captcha_id: Mapped[str] = mapped_column(String(256), nullable=False)
    risk_type: Mapped[str] = mapped_column(String(32), nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    http_status: Mapped[int] = mapped_column(SmallInteger, nullable=False)
    error_code: Mapped[str | None] = mapped_column(String(64))
    error_summary: Mapped[str | None] = mapped_column(String(512))
    accepted_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    duration_ms: Mapped[int | None] = mapped_column()
    quota_reserved: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    quota_refunded: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    client_ip_masked: Mapped[str] = mapped_column(CIDR, nullable=False)
    client_ip_hash: Mapped[bytes] = mapped_column(LargeBinary(64), nullable=False)
    user_agent: Mapped[str] = mapped_column(String(512), nullable=False)
