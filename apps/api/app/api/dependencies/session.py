from collections.abc import Generator
from dataclasses import dataclass
from typing import Annotated, cast

from fastapi import Cookie, Depends, Request
from sqlalchemy.orm import Session

from app.application.errors import ApplicationError
from app.core.crypto import hmac_sha256
from app.core.settings import Settings
from app.db.session import SessionFactory, session_scope
from app.domain.models import User, UserSession
from app.infrastructure.repositories.sessions import SessionRepository


def get_settings(request: Request) -> Settings:
    return cast(Settings, request.app.state.settings)


def get_db_session(request: Request) -> Generator[Session, None, None]:
    factory: SessionFactory = request.app.state.session_factory
    yield from session_scope(factory)


@dataclass(frozen=True)
class CurrentUserSession:
    user: User
    session: UserSession


def require_user_session(
    session_token: Annotated[str | None, Cookie(alias="session")] = None,
    database_session: Session = Depends(get_db_session),
    settings: Settings = Depends(get_settings),
) -> CurrentUserSession:
    if session_token is None:
        raise ApplicationError(401, "SESSION_INVALID", "User session is required.")

    active_session = SessionRepository(database_session).get_active_user_session(
        hmac_sha256(session_token, settings.session_secret)
    )
    if active_session is None:
        raise ApplicationError(401, "SESSION_INVALID", "User session is invalid or expired.")

    user_session, user = active_session
    if user.status != "ACTIVE":
        raise ApplicationError(403, "ACCOUNT_DISABLED", "User account is disabled.")
    return CurrentUserSession(user=user, session=user_session)
