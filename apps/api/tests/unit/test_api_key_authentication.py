from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest

from app.application.errors import ApplicationError
from app.application.keys.authenticate import validate_effective_caller
from app.domain.models import ApiKey, Cdk, User


def active_entities() -> tuple[ApiKey, User, Cdk]:
    user = User(id=uuid4(), status="ACTIVE")
    cdk = Cdk(
        id=uuid4(),
        batch_id=uuid4(),
        code_prefix="CDK",
        code_hash=b"h" * 32,
        status="ACTIVE",
        bound_user_id=user.id,
        quota_total=10,
        quota_used=0,
        quota_reserved=0,
        quota_remaining=10,
        expires_at=datetime.now(UTC) + timedelta(days=1),
    )
    api_key = ApiKey(
        id=uuid4(),
        user_id=user.id,
        name="worker",
        key_prefix="cf_live_abc",
        key_last4="abcd",
        key_hash=b"k" * 32,
        status="ACTIVE",
        total_calls=0,
    )
    return api_key, user, cdk


@pytest.mark.parametrize(
    ("target", "value", "status_code", "code"),
    [
        ("api_key", "DISABLED", 403, "API_KEY_DISABLED"),
        ("api_key", "DELETED", 403, "API_KEY_DISABLED"),
        ("user", "DISABLED", 403, "ACCOUNT_DISABLED"),
        ("cdk", "DISABLED", 403, "SERVICE_UNAVAILABLE"),
    ],
)
def test_effective_authorization_rejects_disabled_entities(
    target: str, value: str, status_code: int, code: str
) -> None:
    api_key, user, cdk = active_entities()
    setattr({"api_key": api_key, "user": user, "cdk": cdk}[target], "status", value)

    with pytest.raises(ApplicationError) as error:
        validate_effective_caller(api_key, user, cdk)

    assert error.value.status_code == status_code
    assert error.value.code == code


def test_effective_authorization_rejects_expired_and_exhausted_cdks() -> None:
    api_key, user, cdk = active_entities()
    cdk.expires_at = datetime.now(UTC) - timedelta(seconds=1)

    with pytest.raises(ApplicationError, match="expired") as expired:
        validate_effective_caller(api_key, user, cdk)
    assert expired.value.code == "SERVICE_UNAVAILABLE"

    cdk.expires_at = datetime.now(UTC) + timedelta(days=1)
    cdk.quota_remaining = 0
    with pytest.raises(ApplicationError) as exhausted:
        validate_effective_caller(api_key, user, cdk)
    assert exhausted.value.status_code == 402
    assert exhausted.value.code == "QUOTA_EXHAUSTED"
