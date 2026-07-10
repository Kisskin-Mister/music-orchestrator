from __future__ import annotations

from collections.abc import Generator
from pathlib import Path

from sqlalchemy.pool import StaticPool
from sqlmodel import Session, SQLModel, create_engine

from .config import Settings


def make_engine(settings: Settings):
    if settings.database_url.startswith("sqlite:///"):
        db_path = settings.database_url.removeprefix("sqlite:///")
        if db_path and db_path != ":memory:":
            Path(db_path).parent.mkdir(parents=True, exist_ok=True)
    connect_args = {"check_same_thread": False} if settings.database_url.startswith("sqlite") else {}
    kwargs = {"connect_args": connect_args}
    if settings.database_url == "sqlite://":
        kwargs["poolclass"] = StaticPool
    return create_engine(settings.database_url, **kwargs)


def init_db(engine) -> None:
    SQLModel.metadata.create_all(engine)


def session_dependency(engine):
    def get_session() -> Generator[Session, None, None]:
        with Session(engine) as session:
            yield session

    return get_session
