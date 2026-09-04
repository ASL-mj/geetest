import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, inspect, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from alembic import command
from alembic.config import Config
from app.domain.models import ApiCall, ApiKey, Cdk, CdkBatch, QuotaLedger, User

DATABASE_URL = os.environ.get("TEST_DATABASE_URL")
API_ROOT = Path(__file__).resolve().parents[2]
pytestmark = pytest.mark.skipif(
    DATABASE_URL is None,
    reason="TEST_DATABASE_URL must point to an empty PostgreSQL database",
)


@pytest.fixture(scope="session")
def migrated_engine() -> Iterator[Engine]:
    assert DATABASE_URL is not None
    control_engine = create_engine(DATABASE_URL)
    with control_engine.begin() as connection:
        connection.execute(text("CREATE ROLE geetest_platform_app NOLOGIN"))
        connection.execute(
            text(
                "ALTER DEFAULT PRIVILEGES IN SCHEMA public "
                "GRANT UPDATE, DELETE ON TABLES TO geetest_platform_app"
            )
        )

    alembic_config = Config(str(API_ROOT / "alembic.ini"))
    alembic_config.set_main_option("sqlalchemy.url", DATABASE_URL)
    command.upgrade(alembic_config, "head")
    engine = create_engine(DATABASE_URL)

    yield engine

    engine.dispose()
    command.downgrade(alembic_config, "base")
    with control_engine.begin() as connection:
        connection.execute(
            text(
                "ALTER DEFAULT PRIVILEGES IN SCHEMA public "
                "REVOKE UPDATE, DELETE ON TABLES FROM geetest_platform_app"
            )
        )
        connection.execute(text("DROP OWNED BY geetest_platform_app"))
        connection.execute(text("DROP ROLE geetest_platform_app"))
    control_engine.dispose()


@pytest.fixture
def session(migrated_engine: Engine) -> Iterator[Session]:
    with Session(migrated_engine) as database_session:
        yield database_session
        database_session.rollback()


def create_user(session: Session) -> User:
    user = User(username=f"user_{uuid4().hex}", password_hash="argon2id$test")
    session.add(user)
    session.flush()
    return user


def create_cdk_batch(session: Session) -> CdkBatch:
    batch = CdkBatch(
        name=f"batch-{uuid4().hex}",
        default_quota=100,
        service_duration_days=30,
    )
    session.add(batch)
    session.flush()
    return batch


def create_bound_cdk(session: Session, user_id: UUID) -> Cdk:
    batch = create_cdk_batch(session)
    cdk = Cdk(
        batch_id=batch.id,
        code_prefix="CDK-TEST",
        code_hash=uuid4().bytes,
        status="ACTIVE",
        bound_user_id=user_id,
        activation_deadline=datetime.now(UTC) + timedelta(days=1),
        expires_at=datetime.now(UTC) + timedelta(days=30),
        quota_total=100,
        quota_used=0,
        quota_reserved=0,
        quota_remaining=100,
    )
    session.add(cdk)
    session.flush()
    return cdk


def create_api_key(session: Session, user_id: UUID) -> ApiKey:
    api_key = ApiKey(
        user_id=user_id,
        name="test-key",
        key_prefix="gtsk_live_test",
        key_last4="test",
        key_hash=uuid4().bytes,
        status="ACTIVE",
        total_calls=0,
    )
    session.add(api_key)
    session.flush()
    return api_key


def create_call(session: Session, user_id: UUID, cdk_id: UUID, api_key_id: UUID) -> ApiCall:
    call = ApiCall(
        request_id=f"req_{uuid4().hex}",
        operation="geetest.solve",
        idempotency_key_hash=uuid4().bytes,
        user_id=user_id,
        cdk_id=cdk_id,
        api_key_id=api_key_id,
        api_key_name_snapshot="test-key",
        api_key_prefix_snapshot="gtsk_live_test",
        captcha_id="captcha-test",
        risk_type="slide",
        status="RECEIVED",
        http_status=202,
        quota_reserved=False,
        quota_refunded=False,
        client_ip_masked="127.0.0.0/24",
        client_ip_hash=uuid4().bytes,
        user_agent="pytest",
    )
    session.add(call)
    session.flush()
    return call


def test_cdk_is_bound_to_at_most_one_user(session: Session) -> None:
    user = create_user(session)
    create_bound_cdk(session, user.id)

    with pytest.raises(IntegrityError):
        create_bound_cdk(session, user.id)


def test_api_key_hash_is_unique(session: Session) -> None:
    user = create_user(session)
    key_hash = uuid4().bytes
    session.add_all(
        [
            ApiKey(
                user_id=user.id,
                name="first",
                key_prefix="gtsk_live_first",
                key_last4="1111",
                key_hash=key_hash,
                status="ACTIVE",
                total_calls=0,
            ),
            ApiKey(
                user_id=user.id,
                name="second",
                key_prefix="gtsk_live_second",
                key_last4="2222",
                key_hash=key_hash,
                status="ACTIVE",
                total_calls=0,
            ),
        ]
    )

    with pytest.raises(IntegrityError):
        session.flush()


def test_api_call_request_and_idempotency_keys_are_unique(session: Session) -> None:
    user = create_user(session)
    cdk = create_bound_cdk(session, user.id)
    api_key = create_api_key(session, user.id)
    call = create_call(session, user.id, cdk.id, api_key.id)
    session.add(
        ApiCall(
            request_id=call.request_id,
            operation="geetest.solve",
            idempotency_key_hash=uuid4().bytes,
            user_id=user.id,
            cdk_id=cdk.id,
            api_key_id=api_key.id,
            api_key_name_snapshot="test-key",
            api_key_prefix_snapshot="gtsk_live_test",
            captcha_id="captcha-another",
            risk_type="slide",
            status="RECEIVED",
            http_status=202,
            quota_reserved=False,
            quota_refunded=False,
            client_ip_masked="127.0.0.0/24",
            client_ip_hash=uuid4().bytes,
            user_agent="pytest",
        )
    )

    with pytest.raises(IntegrityError):
        session.flush()


def test_api_call_idempotency_scope_is_unique(session: Session) -> None:
    user = create_user(session)
    cdk = create_bound_cdk(session, user.id)
    api_key = create_api_key(session, user.id)
    call = create_call(session, user.id, cdk.id, api_key.id)
    session.add(
        ApiCall(
            request_id=f"req_{uuid4().hex}",
            operation=call.operation,
            idempotency_key_hash=call.idempotency_key_hash,
            user_id=user.id,
            cdk_id=cdk.id,
            api_key_id=api_key.id,
            api_key_name_snapshot="test-key",
            api_key_prefix_snapshot="gtsk_live_test",
            captcha_id="captcha-another",
            risk_type="slide",
            status="RECEIVED",
            http_status=202,
            quota_reserved=False,
            quota_refunded=False,
            client_ip_masked="127.0.0.0/24",
            client_ip_hash=uuid4().bytes,
            user_agent="pytest",
        )
    )

    with pytest.raises(IntegrityError):
        session.flush()


def test_quota_ledger_entry_type_is_unique_per_call(session: Session) -> None:
    user = create_user(session)
    cdk = create_bound_cdk(session, user.id)
    api_key = create_api_key(session, user.id)
    call = create_call(session, user.id, cdk.id, api_key.id)
    ledger_values = {
        "cdk_id": cdk.id,
        "user_id": user.id,
        "api_call_id": call.id,
        "entry_type": "RESERVE",
        "available_before": 100,
        "delta_available": -1,
        "available_after": 99,
        "used_before": 0,
        "used_after": 0,
        "reserved_before": 0,
        "reserved_after": 1,
        "reason": "geetest.solve",
        "request_id": call.request_id,
        "actor_type": "system",
    }
    session.add_all([QuotaLedger(**ledger_values), QuotaLedger(**ledger_values)])

    with pytest.raises(IntegrityError):
        session.flush()


def test_cdk_quota_fields_cannot_be_negative(session: Session) -> None:
    user = create_user(session)
    batch = create_cdk_batch(session)
    session.add(
        Cdk(
            batch_id=batch.id,
            code_prefix="CDK-TEST",
            code_hash=uuid4().bytes,
            status="ACTIVE",
            bound_user_id=user.id,
            quota_total=0,
            quota_used=0,
            quota_reserved=0,
            quota_remaining=-1,
        )
    )

    with pytest.raises(IntegrityError):
        session.flush()


def test_critical_indexes_exist(migrated_engine: Engine) -> None:
    indexes = {index["name"] for index in inspect(migrated_engine).get_indexes("api_calls")}

    assert "ix_api_calls_user_accepted_at" in indexes
    assert "ix_api_calls_cdk_accepted_at" in indexes
    assert "ix_api_calls_api_key_accepted_at" in indexes
    assert "ix_api_calls_status_accepted_at" in indexes


def test_quota_ledger_is_append_only_for_the_application_role(migrated_engine: Engine) -> None:
    with migrated_engine.connect() as connection:
        can_update = connection.scalar(
            text("SELECT has_table_privilege('geetest_platform_app', 'quota_ledger', 'UPDATE')")
        )
        can_delete = connection.scalar(
            text("SELECT has_table_privilege('geetest_platform_app', 'quota_ledger', 'DELETE')")
        )

    assert can_update is False
    assert can_delete is False
