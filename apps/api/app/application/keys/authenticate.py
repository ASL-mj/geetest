from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session

from app.application.errors import ApplicationError
from app.core.crypto import hmac_sha256
from app.core.settings import Settings
from app.domain.models import ApiKey, Cdk, User


@dataclass(frozen=True)
class AuthenticatedCaller:
    user_id: UUID
    cdk_id: UUID
    api_key_id: UUID


def validate_effective_caller(api_key: ApiKey, user: User, cdk: Cdk) -> AuthenticatedCaller:
    if api_key.status != "ACTIVE":
        raise ApplicationError(403, "API_KEY_DISABLED", "API Key is disabled.")
    if user.status != "ACTIVE":
        raise ApplicationError(403, "ACCOUNT_DISABLED", "User account is disabled.")
    if cdk.status != "ACTIVE":
        raise ApplicationError(403, "SERVICE_UNAVAILABLE", "CDK service is unavailable.")
    if cdk.expires_at is not None and cdk.expires_at <= datetime.now(UTC):
        raise ApplicationError(403, "SERVICE_UNAVAILABLE", "CDK service has expired.")
    if cdk.quota_remaining <= 0:
        raise ApplicationError(402, "QUOTA_EXHAUSTED", "No remaining quota.")
    return AuthenticatedCaller(user_id=user.id, cdk_id=cdk.id, api_key_id=api_key.id)


def authenticate_api_key(session: Session, settings: Settings, secret: str) -> AuthenticatedCaller:
    api_key = session.scalar(
        select(ApiKey).where(ApiKey.key_hash == hmac_sha256(secret, settings.api_key_pepper))
    )
    if api_key is None:
        raise ApplicationError(401, "API_KEY_INVALID", "API Key is invalid.")
    user = session.get(User, api_key.user_id)
    cdk = session.scalar(select(Cdk).where(Cdk.bound_user_id == api_key.user_id))
    if user is None or cdk is None:
        raise ApplicationError(403, "SERVICE_UNAVAILABLE", "CDK service is unavailable.")
    return validate_effective_caller(api_key, user, cdk)
