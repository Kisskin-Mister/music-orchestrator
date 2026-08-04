# Music Orchestrator — web

Production-фронтенд: Vite + React + TypeScript, TanStack Query, Zustand (плеер/очередь), Tailwind CSS v4.

## Запуск

```bash
npm install
cp .env.example .env
npm run dev
```

`.env`: `VITE_API_BASE_URL` (по умолчанию текущий origin) и `VITE_API_KEY` для protected-эндпоинтов.

## Дизайн-система

- `src/styles.css` — все токены: `--accent`/`--accent-raw` (клэмп через `oklch()`, см. `src/lib/theme.ts`), `--surface`/`--surface-2`/`--surface-3`, шрифты (`Unbounded` для заголовков, `JetBrains Mono` для лейблов — самостоятельно захостены в `public/fonts`, без обращения к Google Fonts в рантайме).
- Акцентный цвет пользователя перекрашивает всё приложение через Tailwind `@theme`-ремап шкалы `lime-*` на `var(--accent)` — в JSX используются обычные классы `bg-lime-300`/`text-lime-300`, отдельного «accent»-класса нет.
- Те же токены значений (не тот же механизм) используются в `../../mobile/lib/theme/tokens.dart` — при правке палитры держать оба файла в синхроне.

## Структура

- `src/features/search/SearchPage.tsx` — основной экран-контейнер (поиск, медиатека, плейлисты, загрузки, настройки, плеер); переключение вкладок через локальный стейт, не через роутинг (react-router-dom установлен, но пока не подключён — см. `docs/roadmap.ru.md`, п. 3.1).
- `src/api/` — типы и HTTP-клиент под контракт `/openapi.json`.
- `src/store/player.ts` — очередь и состояние плеера.
