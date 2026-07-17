# API contract

The backend is written in Go and exposes the machine-readable contract at:

```http
GET /openapi.json
```

## Authentication

Protected endpoints require:

```http
X-API-Key: <APP_API_KEYS>
```

Public endpoints:

```text
GET /health
GET /openapi.json
GET /v1/providers
GET /v1/search
GET /v1/tracks/{track_id}
GET /v1/playback/{track_id}
GET /media/{filename}
```

Protected endpoints:

```text
POST /v1/downloads
GET/POST/DELETE /v1/favorites...
GET/POST /v1/playlists...
GET /v1/jobs...
```

## Providers

```http
GET /v1/providers
```

Frontend must read `capabilities` and `policy` instead of inferring behavior from provider names.

Providers:

- `local` — safe demo/local provider;
- `youtube_official` — embed-only contract;
- `soundcloud_official` — future official integration;
- `youtube_stream` — opt-in `yt-dlp` extractor;
- `soundcloud_stream` — opt-in `yt-dlp` extractor.

## Search

```http
GET /v1/search?q=lofi&providers=youtube_stream,soundcloud_stream&limit=20&offset=0
```

Track ids are opaque. Pass them back to API as-is.

## Playback

```http
GET /v1/playback/{track_id}
```

`playback_type`:

- `local_stream`
- `embed`
- `extractor_stream`
- `unavailable`

Use `stream_url` for `<audio>`, `embed_url` for iframe and refresh expiring stream URLs when `expires_in_seconds` is present.

## Downloads

```http
POST /v1/downloads
X-API-Key: change-me-local-dev-key
Content-Type: application/json

{
  "track_id": "youtube_stream:VIDEO_ID",
  "format": "mp3"
}
```

Response is a `Job`:

- `status=succeeded` + `result.media_url` means file is ready;
- `status=failed` + `error` means extractor/ffmpeg/source failure;
- `status=blocked_by_policy` means risky mode is disabled or provider does not support downloads.

## Media

```http
GET /media/{filename}
```

Files are stored in `APP_MEDIA_ROOT`.

## Errors

```json
{
  "error": {
    "code": "http_404",
    "message": "Track not found"
  }
}
```
