# Music Orchestrator Frontend

Премиальный dark-studio интерфейс для личного self-hosted поиска, playback и архивации музыки. Прототип спроектирован поверх существующего Go API и не смешивает online playback с явным download.

## Что внутри

- `index.html` — канонический интерактивный Search/Home экран, автономный HTML.
- `library.html`, `favorites.html`, `playlists.html`, `downloads.html`, `settings.html` — отдельные продуктовые маршруты.
- `now-playing.html` — полноэкранный player для desktop и mobile.
- `assets/` — общие токены, responsive shell, interaction helpers и SVG-иконки.
- `docs/PRODUCT-DESIGN.md` — UX, IA, state machine, API map, analytics и accessibility.
- `frontend/` — Vite + React + TypeScript scaffold для production-реализации.

## Запуск дизайн-прототипа

Откройте `index.html` через любой static server. Например:

```bash
python3 -m http.server 4173
```

После этого откройте `http://127.0.0.1:4173`.

## Запуск React frontend

```bash
cd frontend
cp ../.env.example .env
npm install
npm run dev
```

Backend по умолчанию ожидается на `http://127.0.0.1:8080`. Protected endpoints получают `X-API-Key` из `VITE_API_KEY`.

## Подключение к Go backend

1. В backend-репозитории запустите `go run .`.
2. Проверьте `GET http://127.0.0.1:8080/health`.
3. Откройте frontend на `http://127.0.0.1:5173`.
4. При изменении контракта заново сгенерируйте типы из `/openapi.json`.

```bash
npx openapi-typescript http://127.0.0.1:8080/openapi.json -o src/api/schema.d.ts
```

## Mock fallback

`VITE_MOCK_FALLBACK=true` сохраняет дизайн-preview, когда backend выключен. Mock-данные используются только после сетевой ошибки; API key, raw query, stream URL и приватные media paths не журналируются.

## Risk mode

YouTube/SoundCloud extractor mode — явный self-hosted opt-in. `Play now` запрашивает временный stream URL и не сохраняет файл. Локальная копия создаётся только через `POST /v1/downloads` после действия «Сохранить offline».
