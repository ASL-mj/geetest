from dataclasses import dataclass
from uuid import UUID

from sqlalchemy.orm import Session

from app.core.crypto import generate_opaque_token, hmac_sha256
from app.core.settings import Settings
from app.domain.models import ApiKey


@dataclass(frozen=True)
class CreatedApiKey:
    record: ApiKey
    secret: str


def create_api_key(
    session: Session, settings: Settings, *, user_id: UUID, name: str
) -> CreatedApiKey:
    secret = f"gtsk_live_{generate_opaque_token()}"
    record = ApiKey(
        user_id=user_id,
        name=name,
        key_prefix=secret[:16],
        key_last4=secret[-4:],
        key_hash=hmac_sha256(secret, settings.api_key_pepper),
        status="ACTIVE",
        total_calls=0,
    )
    session.add(record)
    session.flush()
    return CreatedApiKey(record=record, secret=secret)
