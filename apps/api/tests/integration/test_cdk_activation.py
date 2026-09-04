import hashlib
import os
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import Engine, select, text
from sqlalchemy.orm import Session, sessionmaker

from app.core.crypto import hmac_sha256, normalize_cdk
from app.core.settings import Settings
from app.domain.models import ApiKey, Cdk, CdkBatch, User, UserSession
from app.main import create_app

pytestmark = pytest.mark.skipif(
    os.environ.get("TEST_DATABASE_URL") is None,
    reason="TEST_DATABASE_URL must point to an empty PostgreSQL database",
)


@pytest.fixture
def settings() -> Settings:
    database_url = os.environ.get("TEST_DATABASE_URL")
    assert database_url is not None
    return Settings.model_validate(
        {
            "environment": "test",
            "database_url": database_url,
            "session_secret": "test-session-secret",
            "api_key_pepper": "test-api-key-pepper",
            "cdk_pepper": "test-cdk-pepper",
        }
    )


@pytest.fixture
def client(migrated_engine: Engine, settings: Settings) -> TestClient:
    factory = sessionmaker(bind=migrated_engine, expire_on_commit=False)
    return TestClient(create_app(settings=settings, session_factory=factory), base_url="https://testserver")


@pytest.fixture(autouse=True)
def clear_platform_data(migrated_engine: Engine) -> None:
    with migrated_engine.begin() as connection:
        connection.execute(text("TRUNCATE TABLE users, cdk_batches CASCADE"))


def seed_cdk(
    engine: Engine,
    settings: Settings,
    *,
    code: str = "CDK-ACTIVATION-1234",
    status: str = "UNACTIVATED",
    activation_deadline: datetime | None = None,
    quota_remaining: int = 10,
    bound_user_id: UUID | None = None,
) -> Cdk:
    with Session(engine) as session:
        batch = CdkBatch(
            name=f"batch-{hashlib.sha256(code.encode()).hexdigest()[:16]}",
            default_quota=quota_remaining,
            service_duration_days=30,
        )
        session.add(batch)
        session.flush()
        code_hash = hmac_sha256(normalize_cdk(code), settings.cdk_pepper)
        cdk = Cdk(
            batch_id=batch.id,
            code_prefix=code[:8],
            code_hash=code_hash,
            status=status,
            bound_user_id=bound_user_id,
            activation_deadline=activation_deadline,
            quota_total=quota_remaining,
            quota_used=0,
            quota_reserved=0,
            quota_remaining=quota_remaining,
        )
        session.add(cdk)
        session.commit()
        session.refresh(cdk)
        return cdk


def activation_payload(cdk: str, username: str = "alice") -> dict[str, str]:
    return {"cdk": cdk, "username": username, "password": "A-long-password-123"}


def test_activate_binds_cdk_creates_user_session_and_default_key(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    cdk = seed_cdk(migrated_engine, settings)

    response = client.post("/v1/auth/activate", json=activation_payload("CDK-ACTIVATION-1234"))

    assert response.status_code == 201
    body = response.json()
    assert body["success"] is True
    assert body["data"]["default_api_key"].startswith("gtsk_live_")
    session_token = response.cookies.get("session")
    assert session_token is not None
    set_cookie = response.headers["set-cookie"].lower()
    assert "httponly" in set_cookie
    assert "secure" in set_cookie
    assert "samesite=lax" in set_cookie

    with Session(migrated_engine) as session:
        persisted_cdk = session.get(Cdk, cdk.id)
        assert persisted_cdk is not None
        assert persisted_cdk.bound_user_id == UUID(body["data"]["user"]["id"])
        assert persisted_cdk.activated_at is not None
        assert persisted_cdk.expires_at is not None
        assert persisted_cdk.expires_at > datetime.now(UTC) + timedelta(days=29)
        user = session.scalar(select(User).where(User.username == "alice"))
        assert user is not None
        api_key = session.scalar(select(ApiKey).where(ApiKey.user_id == user.id))
        assert api_key is not None
        assert api_key.key_hash != body["data"]["default_api_key"].encode()
        user_session = session.scalar(select(UserSession).where(UserSession.user_id == user.id))
        assert user_session is not None
        assert user_session.token_hash != session_token.encode()


@pytest.mark.parametrize(
    ("status", "activation_deadline", "quota_remaining", "expected_status", "expected_code"),
    [
        ("DISABLED", None, 10, 403, "CDK_DISABLED"),
        (
            "UNACTIVATED",
            datetime.now(UTC) - timedelta(seconds=1),
            10,
            403,
            "CDK_ACTIVATION_EXPIRED",
        ),
        ("UNACTIVATED", None, 0, 402, "CDK_EXHAUSTED"),
    ],
)
def test_invalid_cdk_state_does_not_create_user_or_key(
    client: TestClient,
    migrated_engine: Engine,
    settings: Settings,
    status: str,
    activation_deadline: datetime | None,
    quota_remaining: int,
    expected_status: int,
    expected_code: str,
) -> None:
    seed_cdk(
        migrated_engine,
        settings,
        code=f"CDK-{expected_code}",
        status=status,
        activation_deadline=activation_deadline,
        quota_remaining=quota_remaining,
    )

    response = client.post(
        "/v1/auth/activate", json=activation_payload(f"CDK-{expected_code}", expected_code.lower())
    )

    assert response.status_code == expected_status
    assert response.json()["error"]["code"] == expected_code
    with Session(migrated_engine) as session:
        assert session.scalar(select(User).where(User.username == expected_code.lower())) is None
        assert session.scalar(select(ApiKey)) is None


def test_invalid_activation_payload_uses_standard_error_response(client: TestClient) -> None:
    response = client.post(
        "/v1/auth/activate",
        json={"cdk": "CDK-ACTIVATION-1234", "username": "alice", "password": "short"},
    )

    assert response.status_code == 422
    assert response.json()["success"] is False
    assert response.json()["request_id"].startswith("req_")
    assert response.json()["error"]["code"] == "INVALID_REQUEST"


def test_already_bound_cdk_does_not_create_a_second_user_or_key(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    with Session(migrated_engine) as session:
        user = User(username="existing", password_hash="not-used", status="ACTIVE")
        session.add(user)
        session.commit()
        session.refresh(user)
    seed_cdk(migrated_engine, settings, status="ACTIVE", bound_user_id=user.id)

    response = client.post(
        "/v1/auth/activate", json=activation_payload("CDK-ACTIVATION-1234", "second")
    )

    assert response.status_code == 409
    assert response.json()["error"]["code"] == "CDK_ALREADY_BOUND"
    with Session(migrated_engine) as session:
        assert session.scalar(select(User).where(User.username == "second")) is None
        assert session.scalar(select(ApiKey).where(ApiKey.user_id == user.id)) is None


def test_login_and_logout_revoke_only_the_current_session(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    seed_cdk(migrated_engine, settings)
    activation = client.post("/v1/auth/activate", json=activation_payload("CDK-ACTIVATION-1234"))
    assert activation.status_code == 201
    first_session = client.cookies.get("session")
    assert first_session is not None

    login = client.post(
        "/v1/auth/login", json={"username": "alice", "password": "A-long-password-123"}
    )

    assert login.status_code == 200
    second_session = client.cookies.get("session")
    assert second_session is not None
    assert second_session != first_session
    logout = client.post("/v1/auth/logout")
    assert logout.status_code == 200
    assert logout.json()["success"] is True

    with Session(migrated_engine) as session:
        persisted_sessions = list(
            session.scalars(select(UserSession).order_by(UserSession.created_at))
        )
        assert len(persisted_sessions) == 2
        assert persisted_sessions[0].revoked_at is None
        assert persisted_sessions[1].revoked_at is not None


def test_logout_requires_an_active_user_session(client: TestClient) -> None:
    response = client.post("/v1/auth/logout")

    assert response.status_code == 401
    assert response.json()["error"]["code"] == "SESSION_INVALID"


def test_logout_rejects_a_disabled_user(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    seed_cdk(migrated_engine, settings)
    activation = client.post("/v1/auth/activate", json=activation_payload("CDK-ACTIVATION-1234"))
    assert activation.status_code == 201

    with Session(migrated_engine) as session:
        user = session.scalar(select(User).where(User.username == "alice"))
        assert user is not None
        user.status = "DISABLED"
        session.commit()

    response = client.post("/v1/auth/logout")

    assert response.status_code == 403
    assert response.json()["error"]["code"] == "ACCOUNT_DISABLED"


def test_concurrent_activation_binds_a_cdk_once(
    migrated_engine: Engine, settings: Settings
) -> None:
    seed_cdk(migrated_engine, settings)
    factory = sessionmaker(bind=migrated_engine, expire_on_commit=False)
    app = create_app(settings=settings, session_factory=factory)

    def activate(username: str) -> int:
        with TestClient(app, base_url="https://testserver") as local_client:
            response = local_client.post(
                "/v1/auth/activate", json=activation_payload("CDK-ACTIVATION-1234", username)
            )
            return response.status_code

    with ThreadPoolExecutor(max_workers=2) as executor:
        statuses = list(executor.map(activate, ["alice", "bob"]))

    assert sorted(statuses) == [201, 409]
    with Session(migrated_engine) as session:
        assert session.scalar(select(User).where(User.username.in_(["alice", "bob"]))) is not None
        assert session.scalar(select(ApiKey)) is not None
