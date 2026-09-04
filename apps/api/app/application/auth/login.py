from dataclasses import dataclass
from datetime import UTC, datetime

from sqlalchemy.orm import Session

from app.application.auth.passwords import verify_password
from app.application.auth.sessions import IssuedUserSession, issue_user_session
from app.application.errors import ApplicationError
from app.core.settings import Settings
from app.domain.models import User
from app.infrastructure.repositories.users import UserRepository


@dataclass(frozen=True)
class LoginResult:
    user: User
    session: IssuedUserSession


def login_user(
    session: Session, settings: Settings, *, username: str, password: str
) -> LoginResult:
    with session.begin():
        user = UserRepository(session).get_by_username(username)
        if user is None or not verify_password(password, user.password_hash):
            raise ApplicationError(401, "INVALID_CREDENTIALS", "Username or password is incorrect.")
        if user.status != "ACTIVE":
            raise ApplicationError(403, "ACCOUNT_DISABLED", "User account is disabled.")

        user.last_login_at = datetime.now(UTC)
        issued_session = issue_user_session(session, user.id, settings)

    return LoginResult(user=user, session=issued_session)
