from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session

from app.application.errors import ApplicationError
from app.domain.models import ApiKey


def get_user_api_key(session: Session, user_id: UUID, key_id: UUID) -> ApiKey:
    api_key = session.scalar(select(ApiKey).where(ApiKey.id == key_id, ApiKey.user_id == user_id))
    if api_key is None:
        raise ApplicationError(404, "API_KEY_NOT_FOUND", "API Key was not found.")
    return api_key


def update_api_key(
    session: Session,
    user_id: UUID,
    key_id: UUID,
    *,
    name: str | None,
    status: str | None,
) -> ApiKey:
    api_key = get_user_api_key(session, user_id, key_id)
    if api_key.status == "DELETED":
        raise ApplicationError(409, "API_KEY_DELETED", "Deleted API Keys cannot be changed.")
    if name is not None:
        api_key.name = name
    if status is not None:
        api_key.status = status
        api_key.revoked_at = datetime.now(UTC) if status == "DISABLED" else None
    session.flush()
    return api_key
