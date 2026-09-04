from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.application.auth.sessions import IssuedUserSession, issue_user_session
from app.application.errors import ApplicationError
from app.application.keys.create import create_api_key
from app.core.crypto import hmac_sha256, normalize_cdk
from app.core.settings import Settings
from app.domain.models import User
from app.infrastructure.repositories.cdks import CdkRepository
from app.infrastructure.repositories.users import UserRepository


@dataclass(frozen=True)
class ActivationResult:
    user: User
    cdk_prefix: str
    default_api_key: str | None
    session: IssuedUserSession
    newly_activated: bool


def activate_cdk(
    session: Session,
    settings: Settings,
    *,
    cdk_code: str,
) -> ActivationResult:
    normalized_cdk = normalize_cdk(cdk_code)
    if not normalized_cdk:
        raise ApplicationError(422, "INVALID_CDK", "CDK format is invalid.")

    now = datetime.now(UTC)
    try:
        with session.begin():
            locked_cdk = CdkRepository(session).lock_by_code_hash(
                hmac_sha256(normalized_cdk, settings.cdk_pepper)
            )
            if locked_cdk is None:
                raise ApplicationError(404, "CDK_NOT_FOUND", "CDK was not found.")

            cdk, batch = locked_cdk
            if cdk.status == "DISABLED":
                raise ApplicationError(403, "CDK_DISABLED", "CDK is disabled.")
            if cdk.activation_deadline is not None and cdk.activation_deadline <= now:
                raise ApplicationError(
                    403, "CDK_ACTIVATION_EXPIRED", "CDK activation deadline has passed."
                )
            if cdk.expires_at is not None and cdk.expires_at <= now:
                raise ApplicationError(403, "CDK_EXPIRED", "CDK service has expired.")
            if cdk.quota_remaining <= 0:
                raise ApplicationError(402, "CDK_EXHAUSTED", "CDK has no remaining quota.")
            if cdk.status == "ACTIVE" and cdk.bound_user_id is not None:
                user = session.get(User, cdk.bound_user_id)
                if user is None:
                    raise ApplicationError(409, "CDK_BINDING_INVALID", "CDK binding is invalid.")
                if user.status != "ACTIVE":
                    raise ApplicationError(403, "ACCOUNT_DISABLED", "User account is disabled.")
                user.last_login_at = now
                issued_session = issue_user_session(session, user.id, settings)
                return ActivationResult(
                    user=user,
                    cdk_prefix=cdk.code_prefix,
                    default_api_key=None,
                    session=issued_session,
                    newly_activated=False,
                )
            if cdk.status != "UNACTIVATED" or cdk.bound_user_id is not None:
                raise ApplicationError(409, "CDK_UNAVAILABLE", "CDK cannot be activated.")

            user = UserRepository(session).create()
            cdk.bound_user_id = user.id
            cdk.status = "ACTIVE"
            cdk.activated_at = now
            cdk.expires_at = (
                now + timedelta(days=batch.service_duration_days)
                if batch.service_duration_days is not None
                else None
            )
            user.last_login_at = now

            default_api_key = create_api_key(
                session, settings, user_id=user.id, name="default"
            ).secret
            issued_session = issue_user_session(session, user.id, settings)
    except IntegrityError as error:
        raise ApplicationError(409, "CDK_UNAVAILABLE", "CDK cannot be activated.") from error

    return ActivationResult(
        user=user,
        cdk_prefix=cdk.code_prefix,
        default_api_key=default_api_key,
        session=issued_session,
        newly_activated=True,
    )
