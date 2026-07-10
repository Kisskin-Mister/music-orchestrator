from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from re import sub

from .config import Settings
from .schemas import Policy, ProviderCapabilities, ProviderRead, ProviderResult, RiskLevel


def canonical_key(title: str, artist: str | None) -> str:
    raw = f"{artist or ''}::{title}".lower()
    return sub(r"[^a-z0-9а-яё]+", " ", raw).strip()


@dataclass(frozen=True)
class TrackSeed:
    provider_track_id: str
    title: str
    artist: str | None = None
    album: str | None = None
    duration_seconds: int | None = None
    source_url: str | None = None
    artwork_url: str | None = None
    attribution: str | None = None


class ProviderAdapter(ABC):
    provider: ProviderRead

    @abstractmethod
    def search(self, query: str, limit: int) -> list[ProviderResult]: ...

    @abstractmethod
    def get(self, provider_track_id: str) -> ProviderResult | None: ...


class LocalProvider(ProviderAdapter):
    def __init__(self, settings: Settings):
        self.settings = settings
        self.provider = ProviderRead(
            id="local",
            name="Local / Navidrome-compatible library",
            kind="local",
            enabled=True,
            configured=bool(settings.navidrome_base_url) or True,
            risk_level=RiskLevel.COMPLIANT,
            capabilities=ProviderCapabilities(
                raw_audio_stream=True,
                persistent_cache=True,
                offline_playback=True,
                public_deployment_safe=True,
            ),
            policy=Policy(
                cache_allowed=True,
                download_allowed=True,
                notes=["Local files only. Navidrome/Subsonic integration is an adapter boundary for future real credentials."],
            ),
            docs_url="https://www.navidrome.org/docs/developers/subsonic-api/",
        )
        self._seeds = [
            TrackSeed("seed-1", "Demo Song", "Local Artist", "Home Library", 180, "/media/demo-song.opus"),
            TrackSeed("seed-1-alt", "Demo Song", "Local Artist", "Home Library", 181, "/media/demo-song-alt.opus"),
            TrackSeed("seed-2", "Another Local Track", "Local Artist", "Home Library", 210, "/media/another-track.opus"),
        ]

    def _to_result(self, seed: TrackSeed) -> ProviderResult:
        return ProviderResult(
            provider_id="local",
            provider_track_id=seed.provider_track_id,
            title=seed.title,
            artist=seed.artist,
            album=seed.album,
            duration_seconds=seed.duration_seconds,
            artwork_url=seed.artwork_url,
            source_url=seed.source_url,
            attribution="Local library",
            risk_level=self.provider.risk_level,
            capabilities=self.provider.capabilities,
            policy=self.provider.policy,
        )

    def search(self, query: str, limit: int) -> list[ProviderResult]:
        q = query.lower().strip()
        matches = [s for s in self._seeds if q in f"{s.title} {s.artist or ''} {s.album or ''}".lower()]
        return [self._to_result(seed) for seed in matches[:limit]]

    def get(self, provider_track_id: str) -> ProviderResult | None:
        return next((self._to_result(seed) for seed in self._seeds if seed.provider_track_id == provider_track_id), None)


class YouTubeOfficialProvider(ProviderAdapter):
    def __init__(self, settings: Settings):
        configured = bool(settings.youtube_api_key)
        self.provider = ProviderRead(
            id="youtube_official",
            name="YouTube official metadata/embed",
            kind="youtube",
            enabled=configured,
            configured=configured,
            risky_enabled=False,
            risk_level=RiskLevel.COMPLIANT,
            capabilities=ProviderCapabilities(
                raw_audio_stream=False,
                embed_playback=True,
                persistent_cache=False,
                offline_playback=False,
                public_deployment_safe=True,
            ),
            policy=Policy(
                cache_allowed=False,
                download_allowed=False,
                requires_external_credentials=True,
                notes=["Metadata/search requires YouTube Data API key. Playback is embed-only. No raw audio, yt-dlp, cache or bypass implementation."],
            ),
            docs_url="https://developers.google.com/youtube/v3",
        )

    def search(self, query: str, limit: int) -> list[ProviderResult]:
        return []

    def get(self, provider_track_id: str) -> ProviderResult | None:
        return ProviderResult(
            provider_id="youtube_official",
            provider_track_id=provider_track_id,
            title="YouTube embedded item",
            artist=None,
            source_url=f"https://www.youtube.com/watch?v={provider_track_id}",
            attribution="YouTube embed playback; metadata not resolved without API key in this MVP.",
            risk_level=self.provider.risk_level,
            capabilities=self.provider.capabilities,
            policy=self.provider.policy,
        )


class SoundCloudOfficialProvider(ProviderAdapter):
    def __init__(self, settings: Settings):
        configured = bool(settings.soundcloud_client_id)
        self.provider = ProviderRead(
            id="soundcloud_official",
            name="SoundCloud official constrained",
            kind="soundcloud",
            enabled=configured,
            configured=configured,
            risky_enabled=False,
            risk_level=RiskLevel.CONSTRAINED,
            capabilities=ProviderCapabilities(
                raw_audio_stream=False,
                official_stream_url=True,
                persistent_cache=False,
                offline_playback=False,
                public_deployment_safe=False,
            ),
            policy=Policy(
                cache_allowed=False,
                download_allowed=False,
                requires_attribution=True,
                requires_external_credentials=True,
                notes=["Official API only; no persistent cache/offline. Custom player must preserve attribution and SoundCloud terms."],
            ),
            docs_url="https://developers.soundcloud.com/docs/api/guide",
        )

    def search(self, query: str, limit: int) -> list[ProviderResult]:
        return []

    def get(self, provider_track_id: str) -> ProviderResult | None:
        return None


def build_adapters(settings: Settings) -> dict[str, ProviderAdapter]:
    adapters = [LocalProvider(settings), YouTubeOfficialProvider(settings), SoundCloudOfficialProvider(settings)]
    return {adapter.provider.id: adapter for adapter in adapters}
