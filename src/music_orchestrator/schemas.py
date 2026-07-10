from __future__ import annotations

from enum import StrEnum
from typing import Literal

from pydantic import BaseModel, Field


class RiskLevel(StrEnum):
    COMPLIANT = "compliant"
    CONSTRAINED = "constrained"
    RISKY = "risky"


class PlaybackType(StrEnum):
    LOCAL_STREAM = "local_stream"
    EMBED = "embed"
    OFFICIAL_STREAM = "official_stream"
    UNAVAILABLE = "unavailable"


class JobStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    BLOCKED_BY_POLICY = "blocked_by_policy"


class ProviderCapabilities(BaseModel):
    search_metadata: bool = True
    raw_audio_stream: bool = False
    embed_playback: bool = False
    official_stream_url: bool = False
    persistent_cache: bool = False
    offline_playback: bool = False
    server_favorites: bool = True
    server_playlists: bool = True
    multiuser_safe: bool = True
    public_deployment_safe: bool = True


class Policy(BaseModel):
    compliant_mode: bool = True
    cache_allowed: bool = False
    download_allowed: bool = False
    requires_attribution: bool = False
    requires_external_credentials: bool = False
    notes: list[str] = Field(default_factory=list)


class ProviderRead(BaseModel):
    id: str
    name: str
    kind: Literal["local", "youtube", "soundcloud"]
    enabled: bool
    configured: bool
    risky_enabled: bool = False
    risk_level: RiskLevel
    capabilities: ProviderCapabilities
    policy: Policy
    docs_url: str | None = None


class ProviderList(BaseModel):
    items: list[ProviderRead]


class ProviderResult(BaseModel):
    provider_id: str
    provider_track_id: str
    title: str
    artist: str | None = None
    album: str | None = None
    duration_seconds: int | None = None
    artwork_url: str | None = None
    source_url: str | None = None
    attribution: str | None = None
    risk_level: RiskLevel
    capabilities: ProviderCapabilities
    policy: Policy


class TrackRead(BaseModel):
    id: str
    title: str
    artist: str | None = None
    album: str | None = None
    duration_seconds: int | None = None
    artwork_url: str | None = None
    canonical_key: str
    providers: list[str]
    provider_results: list[ProviderResult]
    risk_level: RiskLevel
    capabilities: ProviderCapabilities
    policy: Policy


class SearchResponse(BaseModel):
    query: str
    limit: int
    offset: int
    total: int
    items: list[TrackRead]


class PlaybackRead(BaseModel):
    track_id: str
    playback_type: PlaybackType
    provider_id: str
    stream_url: str | None = None
    embed_url: str | None = None
    expires_in_seconds: int | None = None
    attribution: str | None = None
    capabilities: ProviderCapabilities
    policy: Policy


class FavoriteCreate(BaseModel):
    track_id: str = Field(min_length=3, max_length=300)


class FavoriteRead(BaseModel):
    track_id: str
    created_at: str


class FavoriteList(BaseModel):
    limit: int
    offset: int
    total: int
    items: list[FavoriteRead]


class PlaylistCreate(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    description: str | None = Field(default=None, max_length=500)


class PlaylistTrackCreate(BaseModel):
    track_id: str = Field(min_length=3, max_length=300)


class PlaylistSummary(BaseModel):
    id: str
    name: str
    description: str | None = None
    track_count: int
    created_at: str
    updated_at: str


class PlaylistTrackRead(BaseModel):
    track_id: str
    position: int
    added_at: str


class PlaylistRead(PlaylistSummary):
    tracks: list[PlaylistTrackRead] = Field(default_factory=list)


class PlaylistList(BaseModel):
    limit: int
    offset: int
    total: int
    items: list[PlaylistSummary]


class JobCreate(BaseModel):
    type: Literal["resolve", "metadata_refresh", "local_ingest"]
    track_id: str | None = Field(default=None, max_length=300)
    payload: dict = Field(default_factory=dict)


class JobRead(BaseModel):
    id: str
    type: str
    status: JobStatus
    track_id: str | None = None
    payload: dict
    result: dict | None = None
    error: str | None = None
    created_at: str
    updated_at: str


class JobList(BaseModel):
    limit: int
    offset: int
    total: int
    items: list[JobRead]


class HealthRead(BaseModel):
    status: Literal["ok"]
    mode: Literal["compliant"]
    risky_extractors_enabled: bool
    database: str


class ErrorBody(BaseModel):
    code: str
    message: str
    details: dict | None = None


class ErrorResponse(BaseModel):
    error: ErrorBody
