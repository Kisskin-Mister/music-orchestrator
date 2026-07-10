from __future__ import annotations

from .providers import ProviderAdapter, canonical_key
from .schemas import Policy, ProviderCapabilities, ProviderResult, RiskLevel, TrackRead


def merge_capabilities(results: list[ProviderResult]) -> ProviderCapabilities:
    return ProviderCapabilities(
        search_metadata=any(r.capabilities.search_metadata for r in results),
        raw_audio_stream=any(r.capabilities.raw_audio_stream for r in results),
        embed_playback=any(r.capabilities.embed_playback for r in results),
        official_stream_url=any(r.capabilities.official_stream_url for r in results),
        persistent_cache=any(r.capabilities.persistent_cache for r in results),
        offline_playback=any(r.capabilities.offline_playback for r in results),
        server_favorites=all(r.capabilities.server_favorites for r in results),
        server_playlists=all(r.capabilities.server_playlists for r in results),
        multiuser_safe=all(r.capabilities.multiuser_safe for r in results),
        public_deployment_safe=all(r.capabilities.public_deployment_safe for r in results),
    )


def merge_policy(results: list[ProviderResult]) -> Policy:
    notes: list[str] = []
    for result in results:
        notes.extend(result.policy.notes)
    return Policy(
        compliant_mode=all(r.policy.compliant_mode for r in results),
        cache_allowed=any(r.policy.cache_allowed for r in results),
        download_allowed=any(r.policy.download_allowed for r in results),
        requires_attribution=any(r.policy.requires_attribution for r in results),
        requires_external_credentials=any(r.policy.requires_external_credentials for r in results),
        notes=sorted(set(notes)),
    )


def merged_risk(results: list[ProviderResult]) -> RiskLevel:
    if any(r.risk_level == RiskLevel.RISKY for r in results):
        return RiskLevel.RISKY
    if any(r.risk_level == RiskLevel.CONSTRAINED for r in results):
        return RiskLevel.CONSTRAINED
    return RiskLevel.COMPLIANT


def to_track(results: list[ProviderResult]) -> TrackRead:
    primary = sorted(results, key=lambda r: (not r.capabilities.raw_audio_stream, r.provider_id))[0]
    provider_result_id = f"{primary.provider_id}:{primary.provider_track_id}"
    return TrackRead(
        id=provider_result_id,
        title=primary.title,
        artist=primary.artist,
        album=primary.album,
        duration_seconds=primary.duration_seconds,
        artwork_url=primary.artwork_url,
        canonical_key=canonical_key(primary.title, primary.artist),
        providers=sorted({r.provider_id for r in results}),
        provider_results=results,
        risk_level=merged_risk(results),
        capabilities=merge_capabilities(results),
        policy=merge_policy(results),
    )


def search_merged(adapters: dict[str, ProviderAdapter], provider_ids: list[str], query: str, limit: int) -> list[TrackRead]:
    groups: dict[str, list[ProviderResult]] = {}
    for provider_id in provider_ids:
        adapter = adapters.get(provider_id)
        if adapter is None:
            continue
        for result in adapter.search(query, limit):
            groups.setdefault(canonical_key(result.title, result.artist), []).append(result)
    tracks = [to_track(results) for results in groups.values()]
    tracks.sort(key=lambda t: (not t.capabilities.raw_audio_stream, t.title.lower()))
    return tracks[:limit]


def get_track(adapters: dict[str, ProviderAdapter], track_id: str) -> TrackRead | None:
    provider_id, sep, provider_track_id = track_id.partition(":")
    if not sep:
        return None
    adapter = adapters.get(provider_id)
    if adapter is None:
        return None
    result = adapter.get(provider_track_id)
    if result is None:
        return None
    return to_track([result])
