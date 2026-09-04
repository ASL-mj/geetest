from sqlalchemy.orm import Session

from app.domain.models import User


class UserRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def create(self) -> User:
        user = User(status="ACTIVE")
        self._session.add(user)
        self._session.flush()
        return user
