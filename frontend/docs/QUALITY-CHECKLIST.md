# Quality checklist

## P0 — пройдено

- Нет Tailwind indigo и агрессивного purple/blue hero gradient.
- Нет emoji-иконок, filler copy, lorem ipsum и выдуманных performance-метрик.
- Нет rounded cards с цветной левой границей.
- Display typography использует отдельный display stack и отрицательный tracking.
- Все основные действия имеют доступное имя; focus ring и reduced motion предусмотрены.
- `Play now` и `Save offline` представлены как разные действия.
- Risk policy и provider capabilities объясняются текстом, а не только цветом.
- Канонический `index.html` автономен; companion screens ссылаются на существующие локальные assets.
- Inline и shared JavaScript проходят syntax validation; HTML имеет закрывающие `body/html`.
- Responsive rules не задают фиксированную ширину контента и отключают desktop chrome на mobile.

## P1 — пройдено

- Accent ограничен активным playback и редкими сигналами состояния.
- Контент привязан к реальным backend seed tracks, endpoints и policy fields.
- Есть loading, offline, invalid-key, unavailable, expired-stream и blocked-policy контракты.
- Artwork placeholders локальные; внешние placeholder CDN не используются.
