from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID

from sqlalchemy.orm import Session

from app.core.crypto import generate_opaque_token, hmac_sha256
from app.core.settings import Settings
from app.domain.models import UserSession


@dataclass(frozen=True)
class IssuedUserSession:
    token: str
    expires_at: datetime


def issue_user_session(session: Session, user_id: UUID, settings: Settings) -> IssuedUserSession:
    token = generate_opaque_token()
    expires_at = datetime.now(UTC) + timedelta(seconds=settings.session_ttl_seconds)
    session.add(
        UserSession(
            user_id=user_id,
            token_hash=hmac_sha256(token, settings.session_secret),
            expires_at=expires_at,
        )
    )
    return IssuedUserSession(token=token, expires_at=expires_at)
