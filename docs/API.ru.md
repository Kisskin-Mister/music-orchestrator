# API контракт

Backend написан на Go и отдаёт machine-readable контракт:

```http
GET /openapi.json
```

## Авторизация

Protected endpoints требуют header:

```http
X-API-Key: <APP_API_KEYS>
```

Без ключа доступны:

```text
GET /health
GET /openapi.json
GET /v1/providers
GET /v1/search
GET /v1/tracks/{track_id}
GET /v1/playback/{track_id}
GET /media/{filename}
```

С ключом:

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

Фронт должен читать `capabilities` и `policy`, а не угадывать по имени.

Провайдеры:

- `local` — безопасный demo/local provider;
- `youtube_official` — embed-only контракт;
- `soundcloud_official` — future official integration;
- `youtube_stream` — opt-in `yt-dlp` extractor;
- `soundcloud_stream` — opt-in `yt-dlp` extractor.

## Search

```http
GET /v1/search?q=lofi&providers=youtube_stream,soundcloud_stream&limit=20&offset=0
```

Ответ:

```json
{
  "query": "lofi",
  "limit": 20,
  "offset": 0,
  "total": 1,
  "items": []
}
```

`track_id` opaque. Не парсить на фронте, кроме передачи обратно в API.

## Playback

```http
GET /v1/playback/{track_id}
```

`playback_type`:

- `local_stream` — локальная media URL;
- `embed` — iframe URL;
- `extractor_stream` — временный direct audio URL;
- `unavailable` — нельзя проиграть.

Фронт:

- если есть `stream_url` — `<audio src>`;
- если есть `embed_url` — iframe;
- если `expires_in_seconds` есть — обновлять playback URL перед повторным проигрыванием.

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

Ответ — `Job`:

- `status=succeeded` + `result.media_url` — файл готов;
- `status=failed` + `error` — extractor/ffmpeg/source error;
- `status=blocked_by_policy` — risky mode выключен или provider не поддерживает download.

## Media

```http
GET /media/{filename}
```

Файлы лежат в `APP_MEDIA_ROOT`.

## Ошибки

```json
{
  "error": {
    "code": "http_404",
    "message": "Track not found"
  }
}
```
