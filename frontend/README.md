# Music Orchestrator Frontend

Dark-studio интерфейс для личного self-hosted поиска, playback и архивации музыки поверх Go API. Playback и explicit download — разные действия, не смешиваются.

## Что внутри

- `frontend/` — **канонический production-фронтенд**: Vite + React + TypeScript. Полный функционал (поиск, медиатека, плейлисты, загрузки, настройки, плеер) плюс десктопная раскладка с sidebar-рейлом и докованным плеером на широких экранах. См. `frontend/frontend/README.md`.
- `index.html`, `library.html`, `favorites.html`, `playlists.html`, `downloads.html`, `settings.html`, `now-playing.html` — исходный статический HTML/CSS-прототип, из которого выросла дизайн-система React-приложения (токены цвета/радиуса, responsive shell). Оставлен как референс дизайна, не разрабатывается дальше.
- `assets/` — токены и SVG-иконки прототипа.
- `docs/PRODUCT-DESIGN.md` — UX, IA, state machine, API map, analytics и accessibility.
- `../mobile/` — Flutter-клиент (iOS/Android/macOS) с той же дизайн-системой, тот же Go backend.
- `../docs/roadmap.ru.md` — план дальнейших апгрейдов.

## Запуск production frontend

```bash
cd frontend
cp ../.env.example .env
npm install
npm run dev
```

Backend по умолчанию ожидается на `http://127.0.0.1:8080`. Protected endpoints получают `X-API-Key` из `VITE_API_KEY`.

## Запуск дизайн-прототипа (референс)

```bash
python3 -m http.server 4173
```

## Подключение к Go backend

1. В backend-репозитории запустите `go run .`.
2. Проверьте `GET http://127.0.0.1:8080/health`.
3. Откройте frontend на `http://127.0.0.1:5173`.
4. При изменении контракта заново сгенерируйте типы из `/openapi.json`.

```bash
npx openapi-typescript http://127.0.0.1:8080/openapi.json -o src/api/schema.d.ts
```

## Risk mode

YouTube/SoundCloud extractor mode — явный self-hosted opt-in. `Play now` запрашивает временный stream URL и не сохраняет файл. Локальная копия создаётся только через `POST /v1/downloads` после действия «Сохранить offline».
