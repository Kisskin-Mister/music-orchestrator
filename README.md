# Music Orchestrator

Go self-hosted music backend for search, playback contracts, local media downloads, favorites, playlists, jobs and frontend integration.

This repository is intentionally Go-first from the first public commit: one binary, no Python runtime, no hidden credentials, no committed `.env`.

## Status

Backend MVP v1 is ready for frontend work:

- Go `net/http` API.
- No runtime Go dependencies outside stdlib.
- JSON file store by default: `APP_STORE_PATH=./data/store.json`.
- Local/demo provider for safe contract tests.
- Optional YouTube/SoundCloud self-hosted extractor mode through `yt-dlp`.
- Playback resolver returns `local_stream`, `embed`, `extractor_stream` or `unavailable`.
- Download endpoint saves media under `APP_MEDIA_ROOT` and serves it via `/media/{filename}`.
- OpenAPI JSON is available at `/openapi.json`.
- Tests cover health, providers, search, playback, downloads, media, auth, favorites, playlists.

## Safety model

Public-safe mode is the default:

```env
APP_ENABLE_RISKY_EXTRACTORS=false
```

YouTube/SoundCloud stream/download mode is explicit opt-in for personal self-hosted use:

```env
APP_ENABLE_RISKY_EXTRACTORS=true
APP_YT_DLP_BINARY=yt-dlp
```

`yt-dlp` and `ffmpeg` are external tools. Users are responsible for source terms and local law. Do not run extractor mode as public SaaS.

## Requirements

For safe local/demo backend:

```bash
go >= 1.22
```

For YouTube/SoundCloud stream/download:

```bash
yt-dlp
ffmpeg
```

Install on Ubuntu/Raspberry Pi:

```bash
sudo apt-get update
sudo apt-get install -y golang-go ffmpeg
python3 -m pip install -U yt-dlp
```

## Quick start

```bash
cp .env.example .env
go test ./...
go run .
```

Health:

```bash
curl http://127.0.0.1:8080/health
```

Search local demo:

```bash
curl 'http://127.0.0.1:8080/v1/search?q=demo'
```

Enable YouTube/SoundCloud extractor mode in `.env` or shell:

```bash
export APP_ENABLE_RISKY_EXTRACTORS=true
export APP_YT_DLP_BINARY=yt-dlp
go run .
```

Search external providers:

```bash
curl 'http://127.0.0.1:8080/v1/search?q=lofi&providers=youtube_stream,soundcloud_stream&limit=5'
```

Resolve playback stream URL:

```bash
curl 'http://127.0.0.1:8080/v1/playback/youtube_stream:VIDEO_ID'
```

Download audio:

```bash
curl -X POST http://127.0.0.1:8080/v1/downloads \
  -H 'X-API-Key: change-me-local-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{"track_id":"youtube_stream:VIDEO_ID","format":"mp3"}'
```

## Configuration

See `.env.example`.

Important variables:

- `APP_ADDR=:8080`
- `APP_API_KEYS=change-me-local-dev-key`
- `APP_STORE_PATH=./data/store.json`
- `APP_MEDIA_ROOT=./data/media`
- `APP_PUBLIC_MEDIA_BASE_URL=`
- `APP_ENABLE_RISKY_EXTRACTORS=false`
- `APP_YT_DLP_BINARY=yt-dlp`
- `APP_EXTRACTOR_TIMEOUT_SECONDS=30`
- `APP_DOWNLOAD_TIMEOUT_SECONDS=600`

Optional future integrations:

- `APP_YOUTUBE_API_KEY`
- `APP_SOUNDCLOUD_CLIENT_ID`
- `APP_NAVIDROME_BASE_URL`
- `APP_NAVIDROME_USERNAME`
- `APP_NAVIDROME_TOKEN`

## API contract

Machine-readable:

```http
GET /openapi.json
```

Human docs:

- `docs/API.ru.md`
- `docs/API.en.md`
- `docs/FRONTEND-CONTRACT.md`

Core endpoints:

```text
GET  /health
GET  /openapi.json
GET  /v1/providers
GET  /v1/search
GET  /v1/tracks/{track_id}
GET  /v1/playback/{track_id}
POST /v1/downloads
GET  /media/{filename}
GET  /v1/favorites
POST /v1/favorites
DELETE /v1/favorites/{track_id}
GET  /v1/playlists
POST /v1/playlists
GET  /v1/playlists/{playlist_id}
POST /v1/playlists/{playlist_id}/tracks
GET  /v1/jobs
GET  /v1/jobs/{job_id}
```

## Recommended frontend stack

- Vite + React + TypeScript
- TanStack Query
- Zustand for player/queue state
- Tailwind CSS + shadcn/ui
- native `<audio>` first; Howler.js later if crossfade/advanced queue is needed
- OpenAPI TypeScript client generated from `/openapi.json`
- Playwright for smoke tests

## Docker

```bash
docker build -t music-orchestrator:local .
docker run --rm -p 8080:8080 \
  -e APP_ADDR=:8080 \
  -e APP_API_KEYS=change-me-local-dev-key \
  -v "$PWD/data:/app/data" \
  music-orchestrator:local
```

Extractor mode in Docker requires `yt-dlp` and `ffmpeg`; the provided Dockerfile installs both.

## Development checks

```bash
gofmt -w .
go test ./...
go run .
```
