# Music Orchestrator — продуктовый дизайн

## 1. Позиционирование

Личный self-hosted музыкальный командный центр для поиска, прослушивания и сохранения музыки из локальных и внешних источников. Это рабочий инструмент, а не публичный SaaS и не копия стримингового сервиса.

## 2. UX-стратегия

Главная единица интерфейса — track с opaque `track_id`, provider capabilities и policy. UI не угадывает поведение по имени provider. Play, Favorite, Add to playlist и Save offline остаются отдельными командами. Persistent player сохраняет контекст между маршрутами; inspector показывает выбранный track, queue или job.

## 3. Информационная архитектура

```text
App Shell
├── Search / Home
├── Library
│   ├── Local
│   └── Recent
├── Favorites
├── Playlists
│   └── Playlist Detail
├── Downloads
│   └── Job Detail
└── Settings
    ├── API Connection
    ├── Providers
    ├── Risk Mode
    └── Privacy
```

Now Playing существует как persistent bar и как отдельный полноэкранный surface.

## 4. Рекомендуемый stack

- Vite + React + TypeScript.
- Tailwind CSS и Radix/shadcn primitives.
- TanStack Query: API, cache, retries и polling jobs.
- Zustand: current track, playback contract, queue, volume и player errors.
- React Router или TanStack Router.
- OpenAPI-generated client из `/openapi.json`.
- Native `<audio>` для MVP; Howler.js — только при появлении crossfade/advanced queue.
- Zod на ручных boundary, которые не покрывает generated client.
- Vitest + React Testing Library; Playwright для smoke path search → play → favorite → download.

## 5. Визуальная система

Dark studio cockpit: near-black background, layered graphite surfaces, warm off-white copy, acid-lime только для active playback, cool periwinkle только для streaming provider signal, amber только для risky policy. Панели собраны границами и spacing; без тяжёлого glassmorphism и декоративных градиентов.

Ключевые токены находятся в `assets/app.css`: color, typography, spacing posture, radius, shadow, motion, z-index и player dimensions. Display — Space Grotesk/Avenir Next, body — Geist/SF Pro Text, mono — Geist Mono/JetBrains Mono.

## 6. Экранные контракты

- **Search/Home:** command search, filters, source health, result rows, risk notice, player, queue inspector.
- **Library:** локальные файлы, formats, storage placeholder, local playback semantics.
- **Favorites:** сохранённые track ID, quick play, remove, playlist action.
- **Playlists:** коллекции, detail, add action; reorder честно обозначен как следующий API contract.
- **Downloads:** running/succeeded/failed/blocked_by_policy, result media URL и error detail.
- **Settings:** base URL, API key, health test, providers, risky mode и analytics opt-in.
- **Now Playing:** artwork, waveform, contract badge, controls и явное Save offline.

## 7. Компоненты

`AppShell`, `Sidebar`, `TopSearchCommand`, `ProviderFilterBar`, `ProviderBadge`, `CapabilityBadge`, `TrackRow`, `ArtworkPlaceholder`, `PlayerBar`, `NowPlayingSheet`, `QueuePanel`, `FavoriteButton`, `DownloadButton`, `PlaylistPickerDialog`, `JobsPanel`, `JobStatusBadge`, `SettingsPanel`, `ApiConnectionCard`, `EmptyState`, `ErrorState`, `LoadingSkeleton`, `Toast`, `RiskModeBanner`, `AudioWaveformMini`, `EqualizerIndicator`.

## 8. API integration map

| UI | Запрос | Правило |
|---|---|---|
| Connection | `GET /health` | Без ключа, короткий timeout |
| Providers | `GET /v1/providers` | Рендерить `capabilities`, `policy`, `risk_level` |
| Search | `GET /v1/search` | Raw query не отправлять в analytics |
| Track inspector | `GET /v1/tracks/{track_id}` | `track_id` не разбирать |
| Play | `GET /v1/playback/{track_id}` | `stream_url` → audio, `embed_url` → iframe |
| Save offline | `POST /v1/downloads` | Только после явного действия |
| Favorites | `/v1/favorites` | Protected mutation + invalidation |
| Playlists | `/v1/playlists` | Protected queries/mutations |
| Jobs | `/v1/jobs` | Polling только для active jobs |

## 9. Playback state machine

```text
idle
  → resolving_contract
      → local_stream → playing ↔ paused
      → extractor_stream → playing ↔ paused → expired → refreshing
      → embed → embedded_playback
      → unavailable
  → error

playing → buffering → playing
playing → stream_error → refreshing | error
```

Быстро меняющийся progress остаётся внутри audio/player store, а не в TanStack Query.

## 10. Download/job UX

`POST /v1/downloads` создаёт отдельный job. `running` показывает текущую фазу без выдуманного процента. `succeeded` показывает `media_url` и «Играть локальный файл». `blocked_by_policy` объясняет policy без устрашающей подачи. `failed` раскрывает backend error и предлагает повторить.

## 11. Состояния

- Loading: waveform/skeleton с `aria-busy`.
- Empty search: нейтральное приглашение уточнить запрос или providers.
- No providers: переход в Settings.
- API offline: сохранять shell и mock fallback, показывать connection banner.
- Invalid key: public search остаётся доступным, protected actions объясняют ошибку.
- Stream expired: «Обновить stream» без автоматического download.
- Playback unavailable: disabled control + policy note.
- Download blocked: amber status + конкретная capability.
- Reduced motion: отключить meters, shimmer и movement transitions.

## 12. Responsive rules

- `≥1180`: rail + workspace + inspector + bottom player.
- `901–1179`: icon rail + workspace + compact inspector.
- `≤900`: mobile/tablet content, bottom navigation, mini-player, inspector как отдельный route/sheet.
- `≤580`: single-column sources, compact result actions, sticky primary action.

Проверочные ширины: 320, 375, 414, 768, 1024 и 1440 px. Горизонтальный overflow запрещён.

## 13. Accessibility

Keyboard-first navigation, `/` и `Cmd/Ctrl+K` для search, видимый focus ring, labels для icon controls, 44 px mobile targets, AA contrast, status text помимо цвета, `aria-live` для уведомлений и `prefers-reduced-motion`.

## 14. Privacy-first analytics

По умолчанию используется локальный dev logger. Внешний adapter подключается только после opt-in. События: `search_submitted`, `provider_filter_changed`, `playback_requested`, `playback_started`, `playback_failed`, `stream_url_expired`, `favorite_added`, `playlist_created`, `download_requested`, `download_blocked_by_policy`, `download_succeeded`, `api_health_failed`, `api_key_invalid`.

Разрешённые properties: provider IDs, result count, latency bucket, playback type, duration bucket и error code. Запрещены raw query, title, API key, raw stream URL и private media path.
