from datetime import UTC, datetime

from sqlalchemy import select
from sqlalchemy.orm import Session

from app.domain.models import User, UserSession


class SessionRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def get_active_user_session(self, token_hash: bytes) -> tuple[UserSession, User] | None:
        statement = (
            select(UserSession, User)
            .join(User, UserSession.user_id == User.id)
            .where(
                UserSession.token_hash == token_hash,
                UserSession.revoked_at.is_(None),
                UserSession.expires_at > datetime.now(UTC),
            )
        )
        return self._session.execute(statement).tuples().one_or_none()
