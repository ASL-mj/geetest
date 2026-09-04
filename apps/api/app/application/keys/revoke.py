from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.orm import Session

from app.application.keys.update import get_user_api_key
from app.domain.models import ApiKey


def delete_api_key(session: Session, user_id: UUID, key_id: UUID) -> ApiKey:
    api_key = get_user_api_key(session, user_id, key_id)
    api_key.status = "DELETED"
    api_key.revoked_at = datetime.now(UTC)
    session.flush()
    return api_key
