# Frontend Contract

## Stack

Recommended stack for the first frontend:

- Vite + React + TypeScript
- TanStack Query for API calls/cache/retries
- Zustand for player queue/current track state
- Tailwind CSS + shadcn/ui
- native `<audio>` first; Howler.js later if advanced queue/crossfade is needed
- OpenAPI TypeScript client generated from `GET /openapi.json`
- Playwright smoke tests

## API base

Default backend:

```text
http://localhost:8080
```

Protected endpoints need:

```http
X-API-Key: <key>
```

## Required frontend screens

- Search page with provider filters.
- Player bar with play/pause and source badge.
- Track detail drawer with provider policy/capabilities.
- Favorites.
- Playlists.
- Downloads/jobs.
- Settings: API base URL, API key, risky mode warning.

## Provider handling

Never hardcode provider behavior. Use:

- `capabilities.raw_audio_stream`
- `capabilities.embed_playback`
- `capabilities.persistent_cache`
- `capabilities.public_deployment_safe`
- `policy.download_allowed`
- `policy.cache_allowed`
- `risk_level`

UI rules:

- show Play if `/v1/playback/{track_id}` returns `stream_url` or `embed_url`;
- show Download only when `policy.download_allowed=true`;
- show warning badge for `risk_level=risky`;
- treat every `track_id` as opaque.

## Playback handling

```ts
type Playback = {
  track_id: string
  provider_id: string
  playback_type: 'local_stream' | 'embed' | 'extractor_stream' | 'unavailable'
  stream_url?: string | null
  embed_url?: string | null
  expires_in_seconds?: number | null
  attribution?: string
}
```

- `stream_url`: use native `<audio src>`.
- `embed_url`: render iframe.
- `expires_in_seconds`: refresh before replaying stale URLs.
- `unavailable`: disable play and show policy note.

## Download handling

```ts
type DownloadRequest = {
  track_id: string
  format: 'mp3' | 'm4a' | 'opus' | 'wav' | 'flac' | string
}
```

`POST /v1/downloads` returns a job synchronously in the first MVP.

If `job.status === 'succeeded'`, use `job.result.media_url`.

## Error handling

```ts
type APIError = {
  error: {
    code: string
    message: string
    details?: unknown
  }
}
```

Display `error.message` directly for MVP.
