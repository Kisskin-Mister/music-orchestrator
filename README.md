# Music Orchestrator

Backend-first compliant MVP for a personal/home music orchestration service.

RU: этот проект — безопасный backend-контракт для будущего Spotify/YouTube Music-подобного фронтенда под личный self-hosted сценарий. Он не скачивает и не проксирует YouTube/SoundCloud audio в compliant mode.

EN: this project is a safe backend contract for a future self-hosted Spotify/YouTube Music-like frontend. It does not download or proxy YouTube/SoundCloud audio in compliant mode.

## Scope / Объём MVP

- Python FastAPI backend only; no frontend in this phase.
- Typed OpenAPI REST API at `/openapi.json`.
- SQLite for local development; database URL is Postgres-ready.
- Provider capability/policy model for frontend decisions.
- Local/Navidrome-compatible provider boundary.
- YouTube official metadata/embed provider boundary, disabled until API key is configured.
- SoundCloud official constrained provider boundary, disabled until client id is configured.
- Favorites, playlists, abstract jobs, search merge/dedupe behavior.

## Non-goals / Не цели

- No production deployment in this phase.
- No public SaaS behavior.
- No yt-dlp implementation.
- No cookie extraction.
- No YouTube raw-audio streaming/proxy/cache/download.
- No SoundCloud persistent cache/offline storage.
- No source-policy evasion.

`APP_ENABLE_RISKY_EXTRACTORS=false` is the default and this MVP intentionally contains no risky extractor code.

## Quick start

```bash
cd /mnt/ssd/projects/music-orchestrator
python3 -m venv .venv
. .venv/bin/activate
pip install -e '.[dev]'
cp .env.example .env
pytest -q
uvicorn music_orchestrator.main:create_app --factory --host 127.0.0.1 --port 8080
```

Health:

```bash
curl http://127.0.0.1:8080/health
```

Search demo local seed data:

```bash
curl 'http://127.0.0.1:8080/v1/search?q=demo%20song'
```

Create favorite:

```bash
curl -X POST http://127.0.0.1:8080/v1/favorites \
  -H 'X-API-Key: change-me-local-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{"track_id":"local:seed-1"}'
```

## Docker local development

```bash
cp .env.example .env
docker compose up --build api
```

Optional Postgres profile exists for local development only:

```bash
docker compose --profile postgres up --build
```

Then set `APP_DATABASE_URL` in `.env` manually if you want to use Postgres.

## API docs

- English: `docs/API.en.md`
- Russian: `docs/API.ru.md`
- Machine schema: `GET /openapi.json`

## Provider model

Every provider exposes:

- `risk_level`
- `capabilities`
- `policy`
- attribution/docs notes

The frontend must not infer stream/cache/download behavior from provider name. It must read capability and policy fields.

## Development checks

```bash
pytest -q
ruff check .
python - <<'PY'
from fastapi.testclient import TestClient
from music_orchestrator.main import create_app
c = TestClient(create_app())
s = c.get('/openapi.json').json()
print(len(s['paths']))
PY
```

## Русское резюме

Лучший безопасный путь: сначала стабилизировать backend API и capability model, затем подключать реальные official credentials и только после этого делать фронтенд. В этом MVP внешние источники не подделываются: если YouTube/SoundCloud credentials не заданы, provider остаётся capability/policy stub, а локальный seed нужен только для тестирования API-контракта.

## License

MIT.
