# Music Orchestrator — Гайд для фронтенда (v0.1.0 MVP)

## Содержание

1. [Видение продукта](#1-видение-продукта)
2. [Аутентификация](#2-аутентификация)
3. [Модели данных](#3-модели-данных)
4. [Система провайдеров](#4-система-провайдеров)
5. [Полный справочник API](#5-полный-справочник-api)
6. [User Flows](#6-user-flows)
7. [Обработка ошибок](#7-обработка-ошибок)
8. [Пагинация](#8-пагинация)
9. [Real-time и playback](#9-real-time-и-playback)
10. [CORS и конфигурация](#10-cors-и-конфигурация)

---

## 1. Видение продукта

**Music Orchestrator** — self-hosted сервис в стиле Spotify / YouTube Music, где **интернет = библиотека**. Пользователь ищет музыку через единый UI, а бэкенд оркестрирует несколько провайдеров (YouTube, SoundCloud, локальная библиотека/Navidrome), нормализует метаданные и отдаёт треки с учётом capabilities и policy каждого источника.

### Ключевые принципы

- **Единый поиск**: один запрос → результаты из всех подключённых провайдеров, merged и deduplicated.
- **Capabilities-first**: фронтенд **никогда не предполагает** возможности провайдера — всегда читает `capabilities` и `policy` из ответа.
- **Provider badges**: каждый трек показывает, откуда он пришёл и какие ограничения есть.
- **Playback-aware**: YouTube → embed iframe, Local → audio stream, SoundCloud → official stream URL.
- **Безопасный по умолчанию**: cache/download кнопки показываются **только** если `policy.cache_allowed` / `policy.download_allowed` == `true`.

### MVP scope

- Поиск по YouTube (metadata + embed), SoundCloud (metadata), Local (полный stream/cache).
- Избранное и плейлисты (серверные, привязаны к API key).
- Background jobs (resolve, metadata refresh, local ingest) — в MVP только хранят статус, не запускают unsafe extractors.
- Single-user model (один API key = один user_id `local-user`).

---

## 2. Аутентификация

Все **записывающие** (POST, DELETE) эндпоинты требуют заголовок `X-API-Key`.

```
X-API-Key: <your-api-key>
```

- По умолчанию dev key: `dev-local-key-change-me` (настраивается через `APP_API_KEYS` env).
- Можно указать несколько ключей через запятую в env.
- При невалидном/отсутствующем ключе → `401 Unauthorized`.

### Пример

```bash
curl -X POST http://localhost:8080/v1/favorites \
  -H 'X-API-Key: dev-local-key-change-me' \
  -H 'Content-Type: application/json' \
  -d '{"track_id": "local:seed-1"}'
```

### Для фронтенда

- GET-эндпоинты (search, providers, tracks, playback, health) **не требуют** API key.
- POST/DELETE (favorites, playlists, jobs) **требуют** API key.
- Фронтенд должен хранить ключ в localStorage/secure storage и передавать в заголовке.

---

## 3. Модели данных

### 3.1 TrackRead — Трек (результат поиска / получения)

| Поле | Тип | Описание |
|---|---|---|
| `id` | `string` | Уникальный ID трека, формат `{provider}:{provider_track_id}`, напр. `local:seed-1` |
| `title` | `string` | Название трека |
| `artist` | `string?` | Исполнитель |
| `album` | `string?` | Альбом |
| `duration_seconds` | `int?` | Длительность в секундах |
| `artwork_url` | `string?` | URL обложки |
| `canonical_key` | `string` | Нормализованный ключ для deduplication (lowercase `artist::title`) |
| `providers` | `string[]` | Список ID провайдеров, у которых есть этот трек |
| `provider_results` | `ProviderResult[]` | Детальные результаты от каждого провайдера |
| `risk_level` | `RiskLevel` | Объединённый уровень риска |
| `capabilities` | `Capabilities` | Объединённые capabilities |
| `policy` | `Policy` | Объединённая policy |

### 3.2 ProviderResult — Результат от одного провайдера

| Поле | Тип | Описание |
|---|---|---|
| `provider_id` | `string` | ID провайдера (`local`, `youtube_official`, `soundcloud_official`) |
| `provider_track_id` | `string` | ID трека внутри провайдера |
| `title` | `string` | Название |
| `artist` | `string?` | Исполнитель |
| `album` | `string?` | Альбом |
| `duration_seconds` | `int?` | Длительность |
| `artwork_url` | `string?` | Обложка |
| `source_url` | `string?` | Прямая ссылка на источник |
| `attribution` | `string?` | Текст attribution (обязателен для SoundCloud) |
| `risk_level` | `RiskLevel` | Уровень риска провайдера |
| `capabilities` | `Capabilities` | Возможности |
| `policy` | `Policy` | Ограничения |

### 3.3 PlaybackRead — Инструкции воспроизведения

| Поле | Тип | Описание |
|---|---|---|
| `track_id` | `string` | ID трека |
| `playback_type` | `PlaybackType` | Тип воспроизведения |
| `provider_id` | `string` | Какой провайдер используется |
| `stream_url` | `string?` | URL для прямого аудио-стрима (local, official_stream) |
| `embed_url` | `string?` | URL для iframe embed (YouTube) |
| `expires_in_seconds` | `int?` | TTL стрим-URL (для official_stream) |
| `attribution` | `string?` | Текст attribution |
| `capabilities` | `Capabilities` | |
| `policy` | `Policy` | |

#### PlaybackType enum

| Значение | Описание | Когда |
|---|---|---|
| `local_stream` | Прямой аудиофайл | Local/Navidrome |
| `embed` | iframe embed | YouTube |
| `official_stream` | Официальный стрим URL | SoundCloud |
| `unavailable` | Невоспроизводимый | Нет прав/возможностей |

### 3.4 Capabilities (ProviderCapabilities)

| Поле | Тип | Описание |
|---|---|---|
| `search_metadata` | `bool` | Может искать метаданные |
| `raw_audio_stream` | `bool` | Может отдать raw audio |
| `embed_playback` | `bool` | Поддерживает embed-воспроизведение |
| `official_stream_url` | `bool` | Отдаёт official stream URL |
| `persistent_cache` | `bool` | Разрешено кэшировать на диск |
| `offline_playback` | `bool` | Поддерживает offline |
| `server_favorites` | `bool` | Серверные избранные |
| `server_playlists` | `bool` | Серверные плейлисты |
| `multiuser_safe` | `bool` | Безопасно для multiuser |
| `public_deployment_safe` | `bool` | Безопасно для публичного деплоя |

### 3.5 Policy

| Поле | Тип | Описание |
|---|---|---|
| `compliant_mode` | `bool` | Работает в compliant режиме |
| `cache_allowed` | `bool` | **Можно ли кэшировать** (→ показывать кнопку "Скачать") |
| `download_allowed` | `bool` | **Можно ли скачивать** (→ показывать кнопку "Сохранить") |
| `requires_attribution` | `bool` | Нужно показывать attribution |
| `requires_external_credentials` | `bool` | Нужны внешние credentials |
| `notes` | `string[]` | Пояснения/предупреждения |

### 3.6 RiskLevel enum

| Значение | Описание | UI-цвет (рекомендация) |
|---|---|---|
| `compliant` | Полностью легально | 🟢 зелёный |
| `constrained` | Ограничения (attribution, no cache) | 🟡 жёлтый |
| `risky` | ToS risk, личное использование | 🔴 красный |

### 3.7 FavoriteRead

| Поле | Тип |
|---|---|
| `track_id` | `string` |
| `created_at` | `string` (ISO 8601) |

### 3.8 PlaylistSummary / PlaylistRead

**PlaylistSummary** (для списков):

| Поле | Тип |
|---|---|
| `id` | `string` (UUID) |
| `name` | `string` |
| `description` | `string?` |
| `track_count` | `int` |
| `created_at` | `string` |
| `updated_at` | `string` |

**PlaylistRead** (extends PlaylistSummary):

| Поле | Тип |
|---|---|
| `tracks` | `PlaylistTrackRead[]` |

**PlaylistTrackRead**:

| Поле | Тип |
|---|---|
| `track_id` | `string` |
| `position` | `int` |
| `added_at` | `string` |

### 3.9 JobRead

| Поле | Тип | Описание |
|---|---|---|
| `id` | `string` (UUID) | ID задачи |
| `type` | `string` | Тип: `resolve`, `metadata_refresh`, `local_ingest` |
| `status` | `JobStatus` | Статус |
| `track_id` | `string?` | Связанный трек |
| `payload` | `object` | Произвольные параметры |
| `result` | `object?` | Результат |
| `error` | `string?` | Текст ошибки |
| `created_at` | `string` | |
| `updated_at` | `string` | |

#### JobStatus enum: `queued` → `running` → `succeeded` / `failed` / `blocked_by_policy`

### 3.10 ProviderRead

| Поле | Тип |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `kind` | `"local"` \| `"youtube"` \| `"soundcloud"` |
| `enabled` | `bool` |
| `configured` | `bool` |
| `risky_enabled` | `bool` |
| `risk_level` | `RiskLevel` |
| `capabilities` | `Capabilities` |
| `policy` | `Policy` |
| `docs_url` | `string?` |

### 3.11 HealthRead

| Поле | Тип |
|---|---|
| `status` | `"ok"` |
| `mode` | `"compliant"` |
| `risky_extractors_enabled` | `bool` |
| `database` | `string` |

---

## 4. Система провайдеров

### Провайдеры в MVP

| Провайдер | ID | kind | Что может | Playback |
|---|---|---|---|---|
| Local / Navidrome | `local` | `local` | Поиск, stream, cache, download, offline | `local_stream` (прямой URL к файлу) |
| YouTube official | `youtube_official` | `youtube` | Metadata/search (нужен API key), embed playback | `embed` (iframe `youtube.com/embed/...`) |
| SoundCloud official | `soundcloud_official` | `soundcloud` | Metadata/search (нужен client_id), official stream | `official_stream` (временный URL) |

### Ключевое правило для UI

```
Перед показом любой кнопки воспроизведения / скачивания / кэша:
1. Проверить capabilities нужного типа
2. Проверить policy.cache_allowed / policy.download_allowed
3. Показывать provider badge и risk_level на каждом треке
```

### Track ID формат

```
{provider_id}:{provider_track_id}
```

Примеры: `local:seed-1`, `youtube_official:dQw4w9WgXcQ`, `soundcloud_official:12345`

---

## 5. Полный справочник API

**Base URL**: `http://localhost:8080`
**OpenAPI schema**: `GET /openapi.json`

---

### 5.1 `GET /health` — Проверка здоровья

**Auth**: не нужен

**Ответ 200**:
```json
{
  "status": "ok",
  "mode": "compliant",
  "risky_extractors_enabled": false,
  "database": "sqlite"
}
```

```bash
curl http://localhost:8080/health
```

---

### 5.2 `GET /v1/providers` — Список провайдеров

**Auth**: не нужен

**Ответ 200**:
```json
{
  "items": [
    {
      "id": "local",
      "name": "Local / Navidrome-compatible library",
      "kind": "local",
      "enabled": true,
      "configured": true,
      "risky_enabled": false,
      "risk_level": "compliant",
      "capabilities": {
        "search_metadata": true,
        "raw_audio_stream": true,
        "embed_playback": false,
        "official_stream_url": false,
        "persistent_cache": true,
        "offline_playback": true,
        "server_favorites": true,
        "server_playlists": true,
        "multiuser_safe": true,
        "public_deployment_safe": true
      },
      "policy": {
        "compliant_mode": true,
        "cache_allowed": true,
        "download_allowed": true,
        "requires_attribution": false,
        "requires_external_credentials": false,
        "notes": ["Local files only. ..."]
      },
      "docs_url": "https://www.navidrome.org/docs/developers/subsonic-api/"
    },
    {
      "id": "youtube_official",
      "name": "YouTube official metadata/embed",
      "kind": "youtube",
      "enabled": false,
      "configured": false,
      "risk_level": "compliant",
      "capabilities": {
        "raw_audio_stream": false,
        "embed_playback": true,
        "persistent_cache": false,
        "offline_playback": false,
        "public_deployment_safe": true
      },
      "policy": {
        "cache_allowed": false,
        "download_allowed": false,
        "requires_external_credentials": true,
        "notes": ["Metadata/search requires YouTube Data API key. ..."]
      },
      "docs_url": "https://developers.google.com/youtube/v3"
    }
  ]
}
```

```bash
curl http://localhost:8080/v1/providers
```

---

### 5.3 `GET /v1/search` — Поиск треков

**Auth**: не нужен

**Параметры запроса**:

| Параметр | Тип | По умолчанию | Описание |
|---|---|---|---|
| `q` | `string` | *обязательный* | Поисковый запрос (1–200 символов) |
| `providers` | `string` | `local,youtube_official,soundcloud_official` | Через запятую |
| `limit` | `int` | `20` | 1–50 |
| `offset` | `int` | `0` | ≥ 0 |

**Ответ 200**:
```json
{
  "query": "demo song",
  "limit": 20,
  "offset": 0,
  "total": 2,
  "items": [
    {
      "id": "local:seed-1",
      "title": "Demo Song",
      "artist": "Local Artist",
      "album": "Home Library",
      "duration_seconds": 180,
      "artwork_url": null,
      "canonical_key": "local artist::demo song",
      "providers": ["local"],
      "provider_results": [
        {
          "provider_id": "local",
          "provider_track_id": "seed-1",
          "title": "Demo Song",
          "artist": "Local Artist",
          "album": "Home Library",
          "duration_seconds": 180,
          "artwork_url": null,
          "source_url": "/media/demo-song.opus",
          "attribution": "Local library",
          "risk_level": "compliant",
          "capabilities": { "...": "..." },
          "policy": { "...": "..." }
        }
      ],
      "risk_level": "compliant",
      "capabilities": {
        "raw_audio_stream": true,
        "persistent_cache": true,
        "cache_allowed": true,
        "download_allowed": true
      },
      "policy": {
        "compliant_mode": true,
        "cache_allowed": true,
        "download_allowed": true,
        "requires_attribution": false,
        "requires_external_credentials": false,
        "notes": ["Local files only. ..."]
      }
    }
  ]
}
```

```bash
curl 'http://localhost:8080/v1/search?q=demo%20song&limit=10'
curl 'http://localhost:8080/v1/search?q=rock&providers=local&limit=5&offset=0'
```

---

### 5.4 `GET /v1/tracks/{track_id}` — Получить трек по ID

**Auth**: не нужен

**Path params**: `track_id` — формат `{provider}:{id}`, напр. `local:seed-1`

**Ответ 200**: объект `TrackRead` (см. §3.1)
**Ответ 404**: если трек не найден

```bash
curl http://localhost:8080/v1/tracks/local:seed-1
```

---

### 5.5 `GET /v1/playback/{track_id}` — Получить инструкции воспроизведения

**Auth**: не нужен

**Path params**: `track_id`

**Ответ 200**: объект `PlaybackRead` (см. §3.3)

В зависимости от провайдера вернётся один из типов:

| playback_type | stream_url | embed_url | Пример UI |
|---|---|---|---|
| `local_stream` | ✅ прямой URL файла | — | `<audio src="...">` |
| `embed` | — | ✅ `youtube.com/embed/...` | `<iframe src="...">` |
| `official_stream` | ✅ временный URL | — | `<audio src="...">` (с учётом expires) |
| `unavailable` | — | — | "Невоспроизводимо" |

```bash
curl http://localhost:8080/v1/playback/local:seed-1
```

Пример ответа для YouTube:
```json
{
  "track_id": "youtube_official:abc123",
  "playback_type": "embed",
  "provider_id": "youtube_official",
  "stream_url": null,
  "embed_url": "https://www.youtube.com/embed/abc123",
  "expires_in_seconds": null,
  "attribution": "YouTube embed playback; metadata not resolved without API key in this MVP.",
  "capabilities": { "embed_playback": true, "raw_audio_stream": false, "..." : "..." },
  "policy": { "cache_allowed": false, "download_allowed": false, "..." : "..." }
}
```

---

### 5.6 `POST /v1/favorites` — Добавить в избранное

**Auth**: `X-API-Key` ✅

**Request body**:
```json
{
  "track_id": "local:seed-1"
}
```
`track_id`: 3–300 символов.

**Ответ 201**:
```json
{
  "track_id": "local:seed-1",
  "created_at": "2026-07-10T12:00:00+00:00"
}
```

**Идемпотентно**: повторный POST того же track_id → 200 с тем же объектом (не создаёт дубликат).

```bash
curl -X POST http://localhost:8080/v1/favorites \
  -H 'X-API-Key: dev-local-key-change-me' \
  -H 'Content-Type: application/json' \
  -d '{"track_id": "local:seed-1"}'
```

---

### 5.7 `GET /v1/favorites` — Список избранных

**Auth**: `X-API-Key` ✅

**Query params**: `limit` (1–100, default 50), `offset` (default 0)

**Ответ 200**:
```json
{
  "limit": 50,
  "offset": 0,
  "total": 1,
  "items": [
    { "track_id": "local:seed-1", "created_at": "2026-07-10T12:00:00+00:00" }
  ]
}
```

```bash
curl http://localhost:8080/v1/favorites -H 'X-API-Key: dev-local-key-change-me'
```

---

### 5.8 `DELETE /v1/favorites/{track_id}` — Удалить из избранного

**Auth**: `X-API-Key` ✅

**Ответ**: `204 No Content` (идемпотентно — если не было в избранном, тоже 204)

```bash
curl -X DELETE http://localhost:8080/v1/favorites/local:seed-1 \
  -H 'X-API-Key: dev-local-key-change-me'
```

---

### 5.9 `POST /v1/playlists` — Создать плейлист

**Auth**: `X-API-Key` ✅

**Request body**:
```json
{
  "name": "My Playlist",
  "description": "Optional description"
}
```
`name`: 1–120 символов. `description`: до 500 символов, опционально.

**Ответ 201**:
```json
{
  "id": "uuid-...",
  "name": "My Playlist",
  "description": "Optional description",
  "track_count": 0,
  "created_at": "2026-07-10T12:00:00+00:00",
  "updated_at": "2026-07-10T12:00:00+00:00",
  "tracks": []
}
```

```bash
curl -X POST http://localhost:8080/v1/playlists \
  -H 'X-API-Key: dev-local-key-change-me' \
  -H 'Content-Type: application/json' \
  -d '{"name": "Chill Vibes", "description": "Relaxing music"}'
```

---

### 5.10 `GET /v1/playlists` — Список плейлистов

**Auth**: `X-API-Key` ✅

**Query params**: `limit` (1–100, default 50), `offset` (default 0)

**Ответ 200**:
```json
{
  "limit": 50,
  "offset": 0,
  "total": 1,
  "items": [
    {
      "id": "uuid-...",
      "name": "Chill Vibes",
      "description": "Relaxing music",
      "track_count": 3,
      "created_at": "2026-07-10T12:00:00+00:00",
      "updated_at": "2026-07-10T12:30:00+00:00"
    }
  ]
}
```

```bash
curl http://localhost:8080/v1/playlists -H 'X-API-Key: dev-local-key-change-me'
```

---

### 5.11 `GET /v1/playlists/{playlist_id}` — Плейлист с треками

**Auth**: `X-API-Key` ✅

**Ответ 200**:
```json
{
  "id": "uuid-...",
  "name": "Chill Vibes",
  "description": "Relaxing music",
  "track_count": 2,
  "created_at": "2026-07-10T12:00:00+00:00",
  "updated_at": "2026-07-10T12:30:00+00:00",
  "tracks": [
    { "track_id": "local:seed-1", "position": 1, "added_at": "2026-07-10T12:10:00+00:00" },
    { "track_id": "local:seed-2", "position": 2, "added_at": "2026-07-10T12:20:00+00:00" }
  ]
}
```

**Ответ 404**: если плейлист не найден или принадлежит другому пользователю.

```bash
curl http://localhost:8080/v1/playlists/uuid-... -H 'X-API-Key: dev-local-key-change-me'
```

---

### 5.12 `POST /v1/playlists/{playlist_id}/tracks` — Добавить трек в плейлист

**Auth**: `X-API-Key` ✅

**Request body**:
```json
{
  "track_id": "local:seed-2"
}
```

**Ответ 201**: возвращает обновлённый `PlaylistRead` (с обновлённым списком треков).

**Идемпотентно**: если трек уже в плейлите, не создаёт дубликат.

```bash
curl -X POST http://localhost:8080/v1/playlists/uuid-.../tracks \
  -H 'X-API-Key: dev-local-key-change-me' \
  -H 'Content-Type: application/json' \
  -d '{"track_id": "local:seed-2"}'
```

---

### 5.13 `POST /v1/jobs` — Создать фоновую задачу

**Auth**: `X-API-Key` ✅

**Request body**:
```json
{
  "type": "resolve",
  "track_id": "youtube_official:abc123",
  "payload": {}
}
```

`type`: `"resolve"` | `"metadata_refresh"` | `"local_ingest"`

**Ответ 202**:
```json
{
  "id": "uuid-...",
  "type": "resolve",
  "status": "queued",
  "track_id": "youtube_official:abc123",
  "payload": {},
  "result": null,
  "error": null,
  "created_at": "2026-07-10T12:00:00+00:00",
  "updated_at": "2026-07-10T12:00:00+00:00"
}
```

> **MVP note**: задачи создаются и сохраняются со статусом `queued`, но не запускают реальных unsafe extractors.

```bash
curl -X POST http://localhost:8080/v1/jobs \
  -H 'X-API-Key: dev-local-key-change-me' \
  -H 'Content-Type: application/json' \
  -d '{"type": "metadata_refresh", "track_id": "local:seed-1"}'
```

---

### 5.14 `GET /v1/jobs` — Список задач

**Auth**: `X-API-Key` ✅

**Query params**: `limit` (1–100, default 50), `offset` (default 0)

**Ответ 200**: `JobList` с массивом `JobRead`

```bash
curl http://localhost:8080/v1/jobs -H 'X-API-Key: dev-local-key-change-me'
```

---

### 5.15 `GET /v1/jobs/{job_id}` — Получить задачу

**Auth**: `X-API-Key` ✅

**Ответ 200**: `JobRead`
**Ответ 404**: если задача не найдена

```bash
curl http://localhost:8080/v1/jobs/uuid-... -H 'X-API-Key: dev-local-key-change-me'
```

---

## 6. User Flows

### 6.1 Поиск → Прослушивание → Избранное → Плейлист

```
1. Пользователь вводит запрос в поисковую строку
2. GET /v1/search?q={query} → список треков
3. Клик на трек → GET /v1/playback/{track_id}
4. В зависимости от playback_type:
   - local_stream → <audio src="{stream_url}">
   - embed → <iframe src="{embed_url}">
   - official_stream → <audio src="{stream_url}"> (с отслеживанием expires)
   - unavailable → показать "Невоспроизводимо"
5. Кнопка ❤️ → POST /v1/favorites {track_id}
6. Кнопка "+ в плейлист" → POST /v1/playlists/{id}/tracks {track_id}
```

### 6.2 Просмотр библиотеки → Прослушивание

```
1. GET /v1/favorites → список избранных треков
2. Для каждого track_id → GET /v1/tracks/{track_id} (метаданные)
3. Клик → GET /v1/playback/{track_id} → воспроизведение
4. Аналогично для плейлистов: GET /v1/playlists → GET /v1/playlists/{id}
```

### 6.3 Скачивание/кэширование трека

```
1. Проверить policy.cache_allowed == true и policy.download_allowed == true
2. Только тогда показывать кнопку "Скачать"
3. GET /v1/playback/{track_id} → получить stream_url
4. Фронтенд: <a href="{stream_url}" download> или fetch + blob
5. (Опционально) POST /v1/jobs { type: "local_ingest", track_id, payload: { source_url } }
```

> ⚠️ Кнопки скачивания **скрыты** для YouTube и SoundCloud (policy.download_allowed == false).

### 6.4 Управление плейлистами

```
1. Создать: POST /v1/playlists { name, description? }
2. Список: GET /v1/playlists
3. Детали: GET /v1/playlists/{id} → список треков с позициями
4. Добавить трек: POST /v1/playlists/{id}/tracks { track_id }
5. (MVP: нет удаления трека из плейлиста, нет переупорядочивания)
```

---

## 7. Обработка ошибок

### Формат ошибки

Все ошибки возвращают JSON в едином формате:

```json
{
  "error": {
    "code": "unauthorized",
    "message": "Invalid or missing API key",
    "details": null
  }
}
```

### Коды ошибок

| HTTP статус | code | Когда |
|---|---|---|
| `401` | `unauthorized` | Невалидный или отсутствующий X-API-Key |
| `404` | `http_error` | Трек/плейлист/задача не найдена |
| `422` | `validation_error` | Невалидные параметры (Pydantic validation) |

### Для фронтенда

- Показывать toast/banner с `error.message` пользователю.
- При 401 — редирект на экран ввода API key.
- При 422 — подсветить невалидное поле.

---

## 8. Пагинация

Все list-эндпоинты используют единую схему:

**Запрос**:
- `limit` — количество элементов (default зависит от эндпоинта)
- `offset` — смещение

**Ответ**:
```json
{
  "limit": 20,
  "offset": 0,
  "total": 150,
  "items": [...]
}
```

**Для фронта**: `total` показывает общее количество; для "Load More" увеличивать `offset += limit`. Для постраничной навигации: `page = offset / limit + 1`, `totalPages = ceil(total / limit)`.

**Эндпоинты с пагинацией**:
- `GET /v1/search` (limit 1–50)
- `GET /v1/favorites` (limit 1–100)
- `GET /v1/playlists` (limit 1–100)
- `GET /v1/jobs` (limit 1–100)

---

## 9. Real-time и playback

### Текущий MVP

В MVP **нет** WebSocket или SSE. Playback статус отслеживается на стороне фронтенда:
- `<audio>` / `<iframe>` элементы + JS events (`play`, `pause`, `ended`, `timeupdate`).
- Для jobs: поллинг `GET /v1/jobs/{job_id}` с интервалом 2–5 секунд.

### Планируется

- WebSocket/SSE для real-time job status updates.
- Server-sent playback events для multi-device sync.

---

## 10. CORS и конфигурация

### CORS

Сервер настроен на:
- Origins: `http://localhost:5173`, `http://localhost:3000` (Vite dev, CRA)
- Methods: `GET`, `POST`, `DELETE`
- Headers: `X-API-Key`, `Content-Type`
- Credentials: отключены

### ENV-переменные (для справки)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `APP_API_KEYS` | `dev-local-key-change-me` | API ключи через запятую |
| `APP_CORS_ORIGINS` | `http://localhost:5173,...` | Разрешённые origins |
| `APP_DATABASE_URL` | `sqlite:///./data/music-orchestrator.db` | БД |
| `APP_YOUTUBE_API_KEY` | пусто | YouTube Data API key |
| `APP_SOUNDCLOUD_CLIENT_ID` | пусто | SoundCloud client_id |
| `APP_ENABLE_RISKY_EXTRACTORS` | `false` | Включить risky extractors |

---

## Приложение: Quick Reference — все эндпоинты

| # | Method | Path | Auth | Описание |
|---|---|---|---|---|
| 1 | GET | `/health` | — | Статус сервиса |
| 2 | GET | `/v1/providers` | — | Список провайдеров |
| 3 | GET | `/v1/search?q=...` | — | Поиск треков |
| 4 | GET | `/v1/tracks/{track_id}` | — | Трек по ID |
| 5 | GET | `/v1/playback/{track_id}` | — | Инструкции воспроизведения |
| 6 | POST | `/v1/favorites` | ✅ | Добавить в избранное |
| 7 | GET | `/v1/favorites` | ✅ | Список избранных |
| 8 | DELETE | `/v1/favorites/{track_id}` | ✅ | Удалить из избранного |
| 9 | POST | `/v1/playlists` | ✅ | Создать плейлист |
| 10 | GET | `/v1/playlists` | ✅ | Список плейлистов |
| 11 | GET | `/v1/playlists/{playlist_id}` | ✅ | Плейлист с треками |
| 12 | POST | `/v1/playlists/{playlist_id}/tracks` | ✅ | Добавить трек в плейлист |
| 13 | POST | `/v1/jobs` | ✅ | Создать задачу |
| 14 | GET | `/v1/jobs` | ✅ | Список задач |
| 15 | GET | `/v1/jobs/{job_id}` | ✅ | Задача по ID |
| 16 | GET | `/openapi.json` | — | OpenAPI схема |
