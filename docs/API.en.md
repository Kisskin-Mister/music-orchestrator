# Music Orchestrator API (EN)

Base URL for local development: `http://localhost:8080`

This is a backend-first compliant MVP contract for a separate frontend. It does not claim working external music playback unless credentials and a real provider adapter are configured.

## Auth

Write endpoints require `X-API-Key`.

Example:

```bash
curl -H 'X-API-Key: change-me-local-dev-key' http://localhost:8080/v1/favorites
```

## Endpoints

### `GET /health`

Returns service status, compliant mode, risky extractor flag, and database type.

### `GET /v1/providers`

Returns provider capability/policy records:

- `local`: local/Navidrome-compatible library boundary; local files may stream/cache.
- `youtube_official`: YouTube Data API metadata + embed playback only; no raw audio/download/cache.
- `soundcloud_official`: constrained official API boundary; attribution required; no persistent cache/offline.

### `GET /v1/search?q=demo&providers=local,youtube_official,soundcloud_official&limit=20&offset=0`

Returns normalized merged tracks. Each track includes:

- `provider_results`
- `risk_level`
- merged `capabilities`
- merged `policy`

Example:

```bash
curl 'http://localhost:8080/v1/search?q=demo%20song'
```

### `GET /v1/tracks/{track_id}`

Returns one normalized track by id, e.g. `local:seed-1`.

### `GET /v1/playback/{track_id}`

Returns playback instructions. YouTube official returns `embed` with `embed_url`; local returns `local_stream`; unsupported constrained sources can return `unavailable`.

### `POST /v1/favorites`

```bash
curl -X POST http://localhost:8080/v1/favorites \
  -H 'X-API-Key: change-me-local-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{"track_id":"local:seed-1"}'
```

### `GET /v1/favorites`

Lists user favorites.

### `DELETE /v1/favorites/{track_id}`

Deletes a favorite.

### `POST /v1/playlists`

Creates a playlist.

### `GET /v1/playlists`

Lists playlists with `track_count`.

### `GET /v1/playlists/{playlist_id}`

Returns playlist tracks.

### `POST /v1/playlists/{playlist_id}/tracks`

Adds a track to a playlist.

### `POST /v1/jobs`

Creates an abstract background job record. Supported MVP types: `resolve`, `metadata_refresh`, `local_ingest`. The MVP stores queued jobs but does not run unsafe extractors.

### `GET /v1/jobs` and `GET /v1/jobs/{job_id}`

Lists and reads job records.

### `GET /openapi.json`

Machine-readable OpenAPI schema for frontend integration.

## Frontend notes

- Never assume a provider can stream or cache: read `capabilities` and `policy`.
- Show provider badges and risk labels in search results.
- For YouTube official playback, render the embed URL rather than trying to play raw audio.
- Do not show cache/download buttons unless `policy.cache_allowed` or `policy.download_allowed` is true.
