# Music Orchestrator API (RU)

Локальный base URL: `http://localhost:8080`

Это backend-first compliant MVP-контракт для отдельного фронтенда. Он не заявляет рабочий внешний музыкальный источник без реальных credentials и реализованного adapter.

## Auth

Write endpoints требуют `X-API-Key`.

Пример:

```bash
curl -H 'X-API-Key: change-me-local-dev-key' http://localhost:8080/v1/favorites
```

## Endpoints

### `GET /health`

Статус сервиса, compliant mode, флаг risky extractors и тип БД.

### `GET /v1/providers`

Возвращает capability/policy по провайдерам:

- `local`: граница локальной/Navidrome-compatible библиотеки; локальные файлы можно stream/cache.
- `youtube_official`: metadata через YouTube Data API + embed playback; без raw audio/download/cache.
- `soundcloud_official`: constrained official API; нужна attribution; без persistent cache/offline.

### `GET /v1/search?q=demo&providers=local,youtube_official,soundcloud_official&limit=20&offset=0`

Возвращает нормализованные и смердженные треки. В каждом треке есть:

- `provider_results`
- `risk_level`
- merged `capabilities`
- merged `policy`

Пример:

```bash
curl 'http://localhost:8080/v1/search?q=demo%20song'
```

### `GET /v1/tracks/{track_id}`

Возвращает один нормализованный трек, например `local:seed-1`.

### `GET /v1/playback/{track_id}`

Возвращает инструкции playback. YouTube official отдаёт `embed` и `embed_url`; local отдаёт `local_stream`; неподдержанные constrained источники могут вернуть `unavailable`.

### `POST /v1/favorites`

```bash
curl -X POST http://localhost:8080/v1/favorites \
  -H 'X-API-Key: change-me-local-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{"track_id":"local:seed-1"}'
```

### `GET /v1/favorites`

Список favorites пользователя.

### `DELETE /v1/favorites/{track_id}`

Удаляет favorite.

### `POST /v1/playlists`

Создаёт playlist.

### `GET /v1/playlists`

Список playlists с `track_count`.

### `GET /v1/playlists/{playlist_id}`

Playlist со списком треков.

### `POST /v1/playlists/{playlist_id}/tracks`

Добавляет трек в playlist.

### `POST /v1/jobs`

Создаёт абстрактную background job. MVP-типы: `resolve`, `metadata_refresh`, `local_ingest`. MVP хранит queued jobs, но не запускает unsafe extractors.

### `GET /v1/jobs` и `GET /v1/jobs/{job_id}`

Список и чтение job records.

### `GET /openapi.json`

OpenAPI schema для интеграции фронтенда.

## Заметки для фронтенда

- Не предполагай, что провайдер умеет stream/cache: читай `capabilities` и `policy`.
- Показывай provider badges и risk labels в search results.
- Для YouTube official playback рендерь embed URL, а не пытайся играть raw audio.
- Не показывай кнопки cache/download, если `policy.cache_allowed` или `policy.download_allowed` не true.
