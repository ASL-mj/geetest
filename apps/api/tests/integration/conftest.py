import os
from collections.abc import Iterator
from pathlib import Path

import pytest
from sqlalchemy import Engine, create_engine, text

from alembic import command
from alembic.config import Config

TEST_DATABASE_URL = os.environ.get("TEST_DATABASE_URL")
API_ROOT = Path(__file__).resolve().parents[2]


@pytest.fixture(scope="session")
def migrated_engine() -> Iterator[Engine]:
    if TEST_DATABASE_URL is None:
        pytest.skip("TEST_DATABASE_URL must point to an empty PostgreSQL database")

    control_engine = create_engine(TEST_DATABASE_URL)
    with control_engine.begin() as connection:
        connection.execute(text("CREATE ROLE geetest_platform_app NOLOGIN"))
        connection.execute(
            text(
                "ALTER DEFAULT PRIVILEGES IN SCHEMA public "
                "GRANT UPDATE, DELETE ON TABLES TO geetest_platform_app"
            )
        )

    alembic_config = Config(str(API_ROOT / "alembic.ini"))
    alembic_config.set_main_option("sqlalchemy.url", TEST_DATABASE_URL)
    command.upgrade(alembic_config, "head")
    engine = create_engine(TEST_DATABASE_URL)

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
