from __future__ import annotations

from datetime import datetime, timezone
from typing import Any
from uuid import uuid4

from sqlalchemy import JSON, Column
from sqlmodel import Field, SQLModel


def utcnow() -> str:
    return datetime.now(timezone.utc).isoformat()


class Favorite(SQLModel, table=True):
    user_id: str = Field(primary_key=True)
    track_id: str = Field(primary_key=True)
    created_at: str = Field(default_factory=utcnow)


class Playlist(SQLModel, table=True):
    id: str = Field(default_factory=lambda: str(uuid4()), primary_key=True)
    user_id: str = Field(index=True)
    name: str
    description: str | None = None
    created_at: str = Field(default_factory=utcnow)
    updated_at: str = Field(default_factory=utcnow)


class PlaylistTrack(SQLModel, table=True):
    playlist_id: str = Field(primary_key=True)
    track_id: str = Field(primary_key=True)
    position: int
    added_at: str = Field(default_factory=utcnow)


class Job(SQLModel, table=True):
    id: str = Field(default_factory=lambda: str(uuid4()), primary_key=True)
    user_id: str = Field(index=True)
    type: str
    status: str = "queued"
    track_id: str | None = None
    payload: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSON))
    result: dict[str, Any] | None = Field(default=None, sa_column=Column(JSON))
    error: str | None = None
    created_at: str = Field(default_factory=utcnow)
    updated_at: str = Field(default_factory=utcnow)
