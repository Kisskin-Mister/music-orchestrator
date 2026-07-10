from __future__ import annotations

from contextlib import asynccontextmanager
from typing import Annotated

from fastapi import Depends, FastAPI, HTTPException, Query, Response
from fastapi.middleware.cors import CORSMiddleware
from sqlmodel import Session, col, delete, func, select

from .auth import http_exception_handler, require_api_key
from .config import Settings, get_settings
from .db import init_db, make_engine, session_dependency
from .models import Favorite, Job, Playlist, PlaylistTrack, utcnow
from .providers import build_adapters
from .schemas import (
    FavoriteCreate,
    FavoriteList,
    FavoriteRead,
    HealthRead,
    JobCreate,
    JobList,
    JobRead,
    JobStatus,
    PlaybackRead,
    PlaybackType,
    PlaylistCreate,
    PlaylistList,
    PlaylistRead,
    PlaylistSummary,
    PlaylistTrackCreate,
    PlaylistTrackRead,
    ProviderList,
    SearchResponse,
    TrackRead,
)
from .services import get_track, search_merged


def _count(session: Session, model, *where) -> int:
    statement = select(func.count()).select_from(model)
    for clause in where:
        statement = statement.where(clause)
    return int(session.exec(statement).one())


def _job_read(job: Job) -> JobRead:
    return JobRead(
        id=job.id,
        type=job.type,
        status=JobStatus(job.status),
        track_id=job.track_id,
        payload=job.payload,
        result=job.result,
        error=job.error,
        created_at=job.created_at,
        updated_at=job.updated_at,
    )


def _playlist_summary(session: Session, playlist: Playlist) -> PlaylistSummary:
    track_count = _count(session, PlaylistTrack, PlaylistTrack.playlist_id == playlist.id)
    return PlaylistSummary(
        id=playlist.id,
        name=playlist.name,
        description=playlist.description,
        track_count=track_count,
        created_at=playlist.created_at,
        updated_at=playlist.updated_at,
    )


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or get_settings()
    engine = make_engine(settings)
    adapters = build_adapters(settings)

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        init_db(engine)
        yield

    app = FastAPI(
        title="Music Orchestrator API",
        version="0.1.0",
        description=(
            "Backend-first compliant Music Orchestrator MVP. "
            "External providers are capability-gated; risky extractors are disabled by default."
        ),
        lifespan=lifespan,
    )
    app.add_exception_handler(HTTPException, http_exception_handler)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.cors_origin_list,
        allow_credentials=False,
        allow_methods=["GET", "POST", "DELETE"],
        allow_headers=["X-API-Key", "Content-Type"],
    )

    get_session = session_dependency(engine)

    @app.get("/health", response_model=HealthRead, tags=["system"])
    def health() -> HealthRead:
        return HealthRead(
            status="ok",
            mode="compliant",
            risky_extractors_enabled=settings.enable_risky_extractors,
            database=settings.database_url.split(":", 1)[0],
        )

    @app.get("/v1/providers", response_model=ProviderList, tags=["providers"])
    def providers() -> ProviderList:
        return ProviderList(items=[adapter.provider for adapter in adapters.values()])

    @app.get("/v1/search", response_model=SearchResponse, tags=["catalog"])
    def search(
        q: Annotated[str, Query(min_length=1, max_length=200)],
        providers: Annotated[str, Query(description="Comma-separated provider ids")] = "local,youtube_official,soundcloud_official",
        limit: Annotated[int, Query(ge=1, le=50)] = 20,
        offset: Annotated[int, Query(ge=0)] = 0,
    ) -> SearchResponse:
        requested = [item.strip() for item in providers.split(",") if item.strip()]
        items = search_merged(adapters, requested, q, limit + offset)
        return SearchResponse(query=q, limit=limit, offset=offset, total=len(items), items=items[offset : offset + limit])

    @app.get("/v1/tracks/{track_id}", response_model=TrackRead, tags=["catalog"])
    def track(track_id: str) -> TrackRead:
        item = get_track(adapters, track_id)
        if item is None:
            raise HTTPException(status_code=404, detail="Track not found")
        return item

    @app.get("/v1/playback/{track_id}", response_model=PlaybackRead, tags=["playback"])
    def playback(track_id: str) -> PlaybackRead:
        item = get_track(adapters, track_id)
        if item is None:
            raise HTTPException(status_code=404, detail="Track not found")
        result = item.provider_results[0]
        if result.provider_id == "local" and result.source_url:
            return PlaybackRead(
                track_id=track_id,
                provider_id=result.provider_id,
                playback_type=PlaybackType.LOCAL_STREAM,
                stream_url=result.source_url,
                attribution=result.attribution,
                capabilities=result.capabilities,
                policy=result.policy,
            )
        if result.provider_id == "youtube_official":
            return PlaybackRead(
                track_id=track_id,
                provider_id=result.provider_id,
                playback_type=PlaybackType.EMBED,
                embed_url=f"https://www.youtube.com/embed/{result.provider_track_id}",
                attribution=result.attribution,
                capabilities=result.capabilities,
                policy=result.policy,
            )
        if result.capabilities.official_stream_url and result.source_url:
            return PlaybackRead(
                track_id=track_id,
                provider_id=result.provider_id,
                playback_type=PlaybackType.OFFICIAL_STREAM,
                stream_url=result.source_url,
                expires_in_seconds=3600,
                attribution=result.attribution,
                capabilities=result.capabilities,
                policy=result.policy,
            )
        return PlaybackRead(
            track_id=track_id,
            provider_id=result.provider_id,
            playback_type=PlaybackType.UNAVAILABLE,
            attribution=result.attribution,
            capabilities=result.capabilities,
            policy=result.policy,
        )

    @app.post("/v1/favorites", response_model=FavoriteRead, status_code=201, tags=["library"])
    def create_favorite(payload: FavoriteCreate, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> FavoriteRead:
        favorite = session.get(Favorite, (user_id, payload.track_id))
        if favorite is None:
            favorite = Favorite(user_id=user_id, track_id=payload.track_id)
            session.add(favorite)
            session.commit()
            session.refresh(favorite)
        return FavoriteRead(track_id=favorite.track_id, created_at=favorite.created_at)

    @app.get("/v1/favorites", response_model=FavoriteList, tags=["library"])
    def list_favorites(
        user_id: str = Depends(require_api_key),
        session: Session = Depends(get_session),
        limit: Annotated[int, Query(ge=1, le=100)] = 50,
        offset: Annotated[int, Query(ge=0)] = 0,
    ) -> FavoriteList:
        total = _count(session, Favorite, Favorite.user_id == user_id)
        rows = session.exec(
            select(Favorite).where(Favorite.user_id == user_id).order_by(col(Favorite.created_at).desc()).offset(offset).limit(limit)
        ).all()
        return FavoriteList(limit=limit, offset=offset, total=total, items=[FavoriteRead(track_id=r.track_id, created_at=r.created_at) for r in rows])

    @app.delete("/v1/favorites/{track_id}", status_code=204, tags=["library"])
    def delete_favorite(track_id: str, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> Response:
        session.exec(delete(Favorite).where(Favorite.user_id == user_id, Favorite.track_id == track_id))
        session.commit()
        return Response(status_code=204)

    @app.post("/v1/playlists", response_model=PlaylistRead, status_code=201, tags=["library"])
    def create_playlist(payload: PlaylistCreate, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> PlaylistRead:
        playlist = Playlist(user_id=user_id, name=payload.name, description=payload.description)
        session.add(playlist)
        session.commit()
        session.refresh(playlist)
        summary = _playlist_summary(session, playlist)
        return PlaylistRead(**summary.model_dump(), tracks=[])

    @app.get("/v1/playlists", response_model=PlaylistList, tags=["library"])
    def list_playlists(
        user_id: str = Depends(require_api_key),
        session: Session = Depends(get_session),
        limit: Annotated[int, Query(ge=1, le=100)] = 50,
        offset: Annotated[int, Query(ge=0)] = 0,
    ) -> PlaylistList:
        total = _count(session, Playlist, Playlist.user_id == user_id)
        rows = session.exec(
            select(Playlist).where(Playlist.user_id == user_id).order_by(col(Playlist.created_at).desc()).offset(offset).limit(limit)
        ).all()
        return PlaylistList(limit=limit, offset=offset, total=total, items=[_playlist_summary(session, row) for row in rows])

    @app.get("/v1/playlists/{playlist_id}", response_model=PlaylistRead, tags=["library"])
    def get_playlist(playlist_id: str, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> PlaylistRead:
        playlist = session.get(Playlist, playlist_id)
        if playlist is None or playlist.user_id != user_id:
            raise HTTPException(status_code=404, detail="Playlist not found")
        tracks = session.exec(
            select(PlaylistTrack).where(PlaylistTrack.playlist_id == playlist.id).order_by(PlaylistTrack.position)
        ).all()
        summary = _playlist_summary(session, playlist)
        return PlaylistRead(
            **summary.model_dump(),
            tracks=[PlaylistTrackRead(track_id=t.track_id, position=t.position, added_at=t.added_at) for t in tracks],
        )

    @app.post("/v1/playlists/{playlist_id}/tracks", response_model=PlaylistRead, status_code=201, tags=["library"])
    def add_playlist_track(
        playlist_id: str,
        payload: PlaylistTrackCreate,
        user_id: str = Depends(require_api_key),
        session: Session = Depends(get_session),
    ) -> PlaylistRead:
        playlist = session.get(Playlist, playlist_id)
        if playlist is None or playlist.user_id != user_id:
            raise HTTPException(status_code=404, detail="Playlist not found")
        existing = session.get(PlaylistTrack, (playlist_id, payload.track_id))
        if existing is None:
            position = _count(session, PlaylistTrack, PlaylistTrack.playlist_id == playlist_id) + 1
            session.add(PlaylistTrack(playlist_id=playlist_id, track_id=payload.track_id, position=position))
            playlist.updated_at = utcnow()
            session.add(playlist)
            session.commit()
        return get_playlist(playlist_id, user_id, session)

    @app.post("/v1/jobs", response_model=JobRead, status_code=202, tags=["jobs"])
    def create_job(payload: JobCreate, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> JobRead:
        job = Job(user_id=user_id, type=payload.type, track_id=payload.track_id, payload=payload.payload)
        session.add(job)
        session.commit()
        session.refresh(job)
        return _job_read(job)

    @app.get("/v1/jobs", response_model=JobList, tags=["jobs"])
    def list_jobs(
        user_id: str = Depends(require_api_key),
        session: Session = Depends(get_session),
        limit: Annotated[int, Query(ge=1, le=100)] = 50,
        offset: Annotated[int, Query(ge=0)] = 0,
    ) -> JobList:
        total = _count(session, Job, Job.user_id == user_id)
        rows = session.exec(
            select(Job).where(Job.user_id == user_id).order_by(col(Job.created_at).desc()).offset(offset).limit(limit)
        ).all()
        return JobList(limit=limit, offset=offset, total=total, items=[_job_read(row) for row in rows])

    @app.get("/v1/jobs/{job_id}", response_model=JobRead, tags=["jobs"])
    def get_job(job_id: str, user_id: str = Depends(require_api_key), session: Session = Depends(get_session)) -> JobRead:
        job = session.get(Job, job_id)
        if job is None or job.user_id != user_id:
            raise HTTPException(status_code=404, detail="Job not found")
        return _job_read(job)

    return app


def main() -> None:
    import uvicorn

    uvicorn.run("music_orchestrator.main:create_app", factory=True, host="0.0.0.0", port=8080)
