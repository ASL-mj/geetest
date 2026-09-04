from collections.abc import Generator

from sqlalchemy import Engine, create_engine
from sqlalchemy.orm import Session, sessionmaker

from app.core.settings import Settings

SessionFactory = sessionmaker[Session]


def sync_database_url(database_url: str) -> str:
    return database_url.replace("+asyncpg", "+psycopg")


def create_session_factory(settings: Settings) -> tuple[Engine, SessionFactory]:
    engine = create_engine(sync_database_url(settings.database_url), pool_pre_ping=True)
    return engine, sessionmaker(bind=engine, expire_on_commit=False)


def session_scope(factory: SessionFactory) -> Generator[Session]:
    with factory() as session:
        yield session
