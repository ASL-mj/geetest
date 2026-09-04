import os
from datetime import UTC, datetime, timedelta

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import Engine, select, text
from sqlalchemy.orm import Session, sessionmaker

from app.core.crypto import hmac_sha256, normalize_cdk
from app.core.settings import Settings
from app.domain.models import ApiKey, Cdk, CdkBatch
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


def activate_user(
    client: TestClient, engine: Engine, settings: Settings, identity: str = "alice"
) -> str:
    code = f"CDK-{identity}-1234"
    with Session(engine) as session:
        batch = CdkBatch(name=f"batch-{identity}", default_quota=10, service_duration_days=30)
        session.add(batch)
        session.flush()
        session.add(
            Cdk(
                batch_id=batch.id,
                code_prefix=code[:8],
                code_hash=hmac_sha256(normalize_cdk(code), settings.cdk_pepper),
                status="UNACTIVATED",
                activation_deadline=datetime.now(UTC) + timedelta(days=1),
                quota_total=10,
                quota_used=0,
                quota_reserved=0,
                quota_remaining=10,
            )
        )
        session.commit()

    response = client.post(
        "/v1/auth/activate",
        json={"cdk": code},
    )
    assert response.status_code == 201
    default_api_key = response.json()["data"]["default_api_key"]
    assert isinstance(default_api_key, str)
    return default_api_key


def test_new_key_is_returned_once_and_stored_as_hash(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    activate_user(client, migrated_engine, settings)

    response = client.post("/v1/keys", json={"name": "worker-a"})

    assert response.status_code == 201
    secret = response.json()["data"]["secret"]
    assert secret.startswith("gtsk_live_")
    listing = client.get("/v1/keys")
    assert listing.status_code == 200
    assert secret not in listing.text
    with Session(migrated_engine) as session:
        saved_key = session.scalar(select(ApiKey).where(ApiKey.name == "worker-a"))
        assert saved_key is not None
        assert saved_key.key_hash == hmac_sha256(secret, settings.api_key_pepper)


def test_user_can_rename_disable_enable_and_soft_delete_own_key(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    activate_user(client, migrated_engine, settings)
    created = client.post("/v1/keys", json={"name": "worker-a"}).json()["data"]
    key_id = created["id"]

    disable_response = client.patch(
        f"/v1/keys/{key_id}", json={"name": "renamed", "status": "DISABLED"}
    )
    assert disable_response.status_code == 200
    assert client.patch(f"/v1/keys/{key_id}", json={"status": "ACTIVE"}).status_code == 200
    assert client.delete(f"/v1/keys/{key_id}").status_code == 200
    keys = client.get("/v1/keys").json()["data"]["items"]
    deleted = next(item for item in keys if item["id"] == key_id)
    assert deleted["status"] == "DELETED"
    assert "secret" not in deleted


def test_user_cannot_update_another_users_key(
    client: TestClient, migrated_engine: Engine, settings: Settings
) -> None:
    activate_user(client, migrated_engine, settings, "alice")
    key_id = client.post("/v1/keys", json={"name": "alice-key"}).json()["data"]["id"]
    second_client = TestClient(client.app, base_url="https://testserver")
    activate_user(second_client, migrated_engine, settings, "bob")

    response = second_client.patch(f"/v1/keys/{key_id}", json={"name": "stolen"})

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "API_KEY_NOT_FOUND"
