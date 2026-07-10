# Music Orchestrator — Вайрфреймы и описание UI

## Содержание

1. [Общая архитектура UI](#1-общая-архитектура-ui)
2. [Навигация](#2-навигация)
3. [Страница: Home (Главная)](#3-страница-home-главная)
4. [Страница: Search (Поиск)](#4-страница-search-поиск)
5. [Страница: Library (Библиотека)](#5-страница-library-библиотека)
6. [Страница: Playlists (Плейлисты)](#6-страница-playlists-плейлисты)
7. [Страница: Playlist Detail (Детали плейлиста)](#7-страница-playlist-detail)
8. [Страница: Player (Плеер)](#8-страница-player-плеер)
9. [Страница: Settings (Настройки)](#9-страница-settings-настройки)
10. [Компоненты](#10-компоненты)
11. [Mobile-first layout](#11-mobile-first-layout)
12. [Цветовая схема и стили](#12-цветовая-схема-и-стили)

---

## 1. Общая архитектура UI

### Layout (мобильный, portrait)

```
┌─────────────────────────────┐
│         Header Bar          │  ← logo, search icon, settings
├─────────────────────────────┤
│                             │
│         Content Area        │  ← текущая страница
│         (scrollable)        │
│                             │
│                             │
├─────────────────────────────┤
│    Mini Player (sticky)     │  ← artwork, title, play/pause
├─────────────────────────────┤
│     Bottom Navigation       │  ← Home, Search, Library, Playlists
└─────────────────────────────┘
```

### Layout (desktop, ≥768px)

```
┌────────┬────────────────────────────────────────┐
│        │          Header Bar (search)            │
│  Side  ├────────────────────────────────────────┤
│  bar   │                                        │
│        │           Content Area                 │
│  Nav   │           (scrollable)                 │
│        │                                        │
│        ├────────────────────────────────────────┤
│        │       Mini Player (fixed bottom)       │
└────────┴────────────────────────────────────────┘
```

---

## 2. Навигация

### Bottom nav / Sidebar nav

| Иконка | Label | Страница | Описание |
|---|---|---|---|
| 🏠 |Главная| Home | Рекомендации, недавно прослушанное |
| 🔍 |Поиск| Search | Поисковая строка + результаты |
| 📚 |Библиотека| Library | Избранное + все плейлисты |
| 🎵 |Плейлисты| Playlists | Управление плейлистами |
| ⚙️ |Настройки| Settings | API key, провайдеры, theme |

### Навигационные потоки

```
Home ──→ Search (по клику на search bar)
      ──→ Track detail ──→ Player
      ──→ Playlist detail

Search ──→ Track detail ──→ Player
       ──→ Add to favorites / playlist (bottom sheet)

Library ──→ Favorites list ──→ Player
        ──→ Playlist detail ──→ Player

Playlists ──→ Create playlist
          ──→ Playlist detail ──→ Player

Settings ──→ API key input
         ──→ Provider status
```

---

## 3. Страница: Home (Главная)

### Описание

Стартовый экран. Показывает приветствие, быстрый доступ к поиску, недавно прослушанным трекам и рекомендациям.

### Компоненты

1. **Hero / Search CTA** — большая карточка "Найди любую музыку" с полем ввода → переход на Search.

2. **Recently Played** (горизонтальный scroll)
   - Карточки: обложка (64×64), название, исполнитель
   - Клик → воспроизведение
   - Если нет истории → скрыть секцию

3. **Your Favorites** (горизонтальный scroll)
   - Первые 5–10 избранных треков
   - "Показать все" → Library → Favorites

4. **Your Playlists** (горизонтальный scroll)
   - Карточки: название, количество треков
   - Клик → Playlist detail

5. **Provider Status** (info-блок)
   - Список подключённых провайдеров с бейджами
   - Green dot = configured, grey = not configured

### Wireframe

```
┌─────────────────────────────────────┐
│ 🎵 Music Orchestrator       ⚙️     │
├─────────────────────────────────────┤
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 🔍  Найди любую музыку...  │    │
│  └─────────────────────────────┘    │
│                                     │
│  Недавно прослушанное          →   │
│  ┌──────┐ ┌──────┐ ┌──────┐       │
│  │ 🎵   │ │ 🎵   │ │ 🎵   │       │
│  │ Song │ │ Song │ │ Song │       │
│  └──────┘ └──────┘ └──────┘       │
│                                     │
│  Избранное                      →  │
│  ┌──────┐ ┌──────┐ ┌──────┐       │
│  │ ❤️   │ │ ❤️   │ │ ❤️   │       │
│  │ Song │ │ Song │ │ Song │       │
│  └──────┘ └──────┘ └──────┘       │
│                                     │
├─────────────────────────────────────┤
│  [artwork] Song Title    ▶ ⏭      │
├─────────────────────────────────────┤
│  🏠     🔍     📚     🎵     ⚙️   │
└─────────────────────────────────────┘
```

---

## 4. Страница: Search (Поиск)

### Описание

Основной экран для поиска музыки. Показывает результаты из всех провайдеров.

### Компоненты

1. **Search Bar** (sticky top)
   - Поле ввода с debounce 300мс
   - Кнопка очистки (×)
   - Иконка фильтра провайдеров (dropdown с чекбоксами)

2. **Provider Filter** (опциональный dropdown/chips)
   - Чекбоксы: Local, YouTube, SoundCloud
   - По умолчанию все включены

3. **Results List**
   - Каждый трек — карточка:
     - Обложка (48×48 или placeholder)
     - Название, Исполнитель, Альбом
     - Длительность (формат mm:ss)
     - Provider badges (цветные иконки: 🟢 local, 🔴 YouTube, 🟡 SoundCloud)
     - Risk badge (🟢 compliant, 🟡 constrained, 🔴 risky)
     - Кнопки: ▶ Play, ❤️ Favorite, + Playlist
   - Infinite scroll или "Загрузить ещё"

4. **Empty State** — "Введите запрос для поиска"

5. **No Results** — "Ничего не найдено по запросу ..."

### Wireframe

```
┌─────────────────────────────────────┐
│ 🔍  Enter song name...         ✕   │
│ [Local ✓] [YouTube ✓] [SoundCloud ✓]│
├─────────────────────────────────────┤
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 🎵  Demo Song               │    │
│  │     Local Artist • 3:00     │    │
│  │     🟢local  🟢compliant    │    │
│  │            ▶  ❤️  +         │    │
│  └─────────────────────────────┘    │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 🎵  Another Track           │    │
│  │     YouTube Artist • 4:22   │    │
│  │     🔴youtube  🟢compliant  │    │
│  │            ▶  ❤️  +         │    │
│  └─────────────────────────────┘    │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 🎵  SoundCloud Track        │    │
│  │     SC Artist • 2:45        │    │
│  │     🟡soundcloud 🟡constrained│   │
│  │     ⚠️ attribution required  │    │
│  │            ▶  ❤️  +         │    │
│  └─────────────────────────────┘    │
│                                     │
│        [Загрузить ещё]              │
│                                     │
├─────────────────────────────────────┤
│  [artwork] Song Title    ▶ ⏭      │
├─────────────────────────────────────┤
│  🏠     🔍     📚     🎵     ⚙️   │
└─────────────────────────────────────┘
```

### Track Card — детальная структура

```
┌──────────────────────────────────────┐
│ ┌──────┐  Song Title                 │
│ │      │  Artist Name                │
│ │ art  │  Album • 3:45               │
│ │ work │  [🟢local] [🟢compliant]    │
│ └──────┘                             │
│                         ▶   ❤️   ⋮  │
└──────────────────────────────────────┘
```

Кнопка ⋮ (more) → Bottom sheet:
- Добавить в плейлист
- Перейти к источнику
- Информация о треке

---

## 5. Страница: Library (Библиотека)

### Описание

Единый экран библиотеки: избранное + плейлисты. Вкладки или секции.

### Компоненты

1. **Tab Bar**: `Избранное` | `Плейлисты`

2. **Вкладка "Избранное"**
   - Список треков (аналогичен search results, но без provider filter)
   - Каждый трек: обложка, название, исполнитель, длительность, ▶, ⋮
   - Кнопка ⋮ → "Удалить из избранного", "Добавить в плейлист"
   - Empty state: "Нет избранных треков. Ищите музыку и добавляйте ❤️"

3. **Вкладка "Плейлисты"**
   - Список плейлистов (карточки)
   - Каждая: название, описание, количество треков
   - FAB (floating action button): "+ Новый плейлист"
   - Empty state: "Нет плейлистов. Создайте первый 🎵"

### Wireframe

```
┌─────────────────────────────────────┐
│  📚 Библиотека                      │
├─────────────────────────────────────┤
│  [Избранное]  [Плейлисты]           │
├─────────────────────────────────────┤
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 🎵  Favorite Song 1         │    │
│  │     Artist • 3:00     ▶ ⋮  │    │
│  └─────────────────────────────┘    │
│  ┌─────────────────────────────┐    │
│  │ 🎵  Favorite Song 2         │    │
│  │     Artist • 4:15     ▶ ⋮  │    │
│  └─────────────────────────────┘    │
│                                     │
│                    ┌────┐           │
│                    │ +  │  ← FAB    │
│                    └────┘           │
├─────────────────────────────────────┤
│  [artwork] Song Title    ▶ ⏭      │
├─────────────────────────────────────┤
│  🏠     🔍     📚     🎵     ⚙️   │
└─────────────────────────────────────┘
```

---

## 6. Страница: Playlists (Плейлисты)

### Описание

Отдельный экран управления плейлистами (альтернативно может быть вкладкой в Library).

### Компоненты

1. **Playlist Cards** (grid или list)
   - Каждая: название, описание (обрезать до 2 строк), "X треков", дата создания
   - Клик → Playlist Detail

2. **Create Playlist Button** (FAB или header button)
   - Открывает модальное окно:
     - Название (обязательно, 1–120 символов)
     - Описание (опционально, до 500 символов)
     - Кнопки: Отмена, Создать

---

## 7. Страница: Playlist Detail

### Описание

Детальный вид плейлиста с треками.

### Компоненты

1. **Playlist Header**
   - Название, описание
   - Количество треков, общая длительность (вычислить из track.duration_seconds)
   - Кнопка ▶ Все (воспроизвести весь плейлист)

2. **Track List**
   - Нумерованный список треков
   - Каждый: #, обложка, название, исполнитель, длительность, ▶, ⋮
   - Порядок по `position`

3. **Empty State**: "Плейлист пуст. Добавьте треки из поиска."

### Wireframe

```
┌─────────────────────────────────────┐
│  ←  Chill Vibes                     │
│      Relaxing music                 │
│      12 треков • 45 мин    [▶ Все] │
├─────────────────────────────────────┤
│                                     │
│  1  🎵  Song One            3:00 ⋮ │
│  2  🎵  Song Two            4:15 ⋮ │
│  3  🎵  Song Three          2:30 ⋮ │
│  4  🎵  Song Four           5:00 ⋮ │
│  ...                                │
│                                     │
├─────────────────────────────────────┤
│  [artwork] Song Title    ▶ ⏭      │
├─────────────────────────────────────┤
│  🏠     🔍     📚     🎵     ⚙️   │
└─────────────────────────────────────┘
```

---

## 8. Страница: Player (Плеер)

### Описание

Полноэкранный плеер. Открывается по свайпу вверх от Mini Player или клику на него.

### Компоненты

1. **Artwork** — большое изображение обложки (по центру, адаптивное)

2. **Track Info** — Название (bold), Исполнитель, Альбом

3. **Provider Badge** — иконка провайдера + risk level

4. **Progress Bar**
   - Полоса прогресса (seekable для local_stream)
   - Текущее время / Общая длительность

5. **Playback Controls**
   - ⏮ Previous (для плейлиста)
   - ▶/⏸ Play/Pause
   - ⏭ Next (для плейлиста)
   - 🔀 Shuffle (опционально)
   - 🔁 Repeat (опционально)

6. **Action Buttons**
   - ❤️ Favorite toggle
   - ➕ Add to playlist (bottom sheet)
   - ⬇️ Download (только если policy.download_allowed)

7. **Attribution** (если policy.requires_attribution)
   - Показать attribution text

### Playback Rendering

В зависимости от `playback_type`:

| playback_type | Что рендерить |
|---|---|
| `local_stream` | `<audio src="{stream_url}">` — стандартный HTML5 audio |
| `embed` | `<iframe src="{embed_url}">` — YouTube embed player |
| `official_stream` | `<audio src="{stream_url}">` — с отслеживанием `expires_in_seconds` |
| `unavailable` | Блок с сообщением "Невоспроизводимо" |

### Mini Player (sticky bottom, виден на всех страницах)

```
┌─────────────────────────────────────┐
│ [🎵 art]  Song Title       ▶  ⏭   │
│           Artist Name               │
│ ════════════════░░░░░░░░░░░░░░░░░  │ ← progress bar
└─────────────────────────────────────┘
```

### Full Player (по свайпу вверх)

```
┌─────────────────────────────────────┐
│         ─── (drag handle) ───       │
│                                     │
│        ┌───────────────────┐        │
│        │                   │        │
│        │                   │        │
│        │    ARTWORK        │        │
│        │    (square)       │        │
│        │                   │        │
│        │                   │        │
│        └───────────────────┘        │
│                                     │
│        Song Title                   │
│        Artist Name                  │
│        🟢 local • compliant         │
│                                     │
│   1:23 ━━━━━━━━━━░━━━━━━━ 3:00     │
│                                     │
│        ⏮    ▶/⏸    ⏭              │
│        🔀               🔁          │
│                                     │
│    ❤️       ➕        ⬇️             │
│                                     │
│    Source: Local library             │
│                                     │
└─────────────────────────────────────┘
```

---

## 9. Страница: Settings (Настройки)

### Описание

Экран настроек: API key, провайдеры, тема.

### Компоненты

1. **API Key** (секция)
   - Поле ввода API key (password type, с toggle visibility)
   - Кнопка "Проверить" → вызывает `GET /health` или `GET /v1/providers` с ключом
   - Статус: ✅ Подключено / ❌ Ошибка

2. **Провайдеры** (секция)
   - Список провайдеров из `GET /v1/providers`
   - Каждый: название, иконка, status badge
     - 🟢 Configured & enabled
     - 🟡 Enabled but not configured (нужны credentials)
     - ⚪ Disabled
   - Capabilities summary (icons: stream ✓, cache ✗, embed ✓, download ✗)

3. **Тема** (секция)
   - Toggle: Светлая / Тёмная / Системная

4. **О приложении** (секция)
   - Версия, ссылки на документацию, OpenAPI schema

---

## 10. Компоненты

### 10.1 Track Card

Универсальная карточка трека, используется в Search, Library, Playlist Detail.

```
Props:
- track: TrackRead (или FavoriteRead + загрузка трека)
- showProviderBadge: bool (default true)
- showRiskBadge: bool (default true)
- compact: bool (default false, для mini player context)

Actions:
- onPlay: (trackId) → void
- onFavorite: (trackId) → void
- onAddToPlaylist: (trackId) → void
- onMore: (trackId) → void (bottom sheet)
```

### 10.2 Playlist Card

```
Props:
- playlist: PlaylistSummary

Displays:
- name
- description (truncated)
- track_count
- created_at (relative: "2 дня назад")
```

### 10.3 Provider Badge

```
Props:
- providerId: string ("local" | "youtube_official" | "soundcloud_official")

Rendering:
- local → 🟢 "Local"
- youtube_official → 🔴 "YouTube"
- soundcloud_official → 🟡 "SoundCloud"
```

### 10.4 Risk Badge

```
Props:
- riskLevel: RiskLevel

Rendering:
- compliant → 🟢 "Compliant"
- constrained → 🟡 "Constrained"
- risky → 🔴 "Risky"
```

### 10.5 Bottom Sheet (Add to Playlist)

```
Content:
- Список существующих плейлистов (клик → добавить)
- Кнопка "Создать новый плейлист" → inline form
- Кнопка "Закрыть"
```

### 10.6 Toast / Notification

```
Types:
- success: "Добавлено в избранное ❤️"
- error: "Ошибка: {message}"
- info: "Трек недоступен для скачивания"
```

### 10.7 Search Input

```
Props:
- value: string
- onChange: (value) → void
- onClear: () → void
- debounce: 300ms
- placeholder: "Найди любую музыку..."
```

### 10.8 Empty State

```
Props:
- icon: string (emoji)
- title: string
- subtitle: string
- action?: { label: string, onClick: () → void }
```

---

## 11. Mobile-first layout

### Breakpoints

| Breakpoint | Layout |
|---|---|
| < 768px | Single column, bottom nav, mini player |
| 768–1023px | Sidebar nav, 2-column grid для карточек |
| ≥ 1024px | Sidebar nav, 3-column grid, expanded player |

### Размеры и отступы

| Элемент | Размер |
|---|---|
| Header bar height | 56px |
| Bottom nav height | 64px |
| Mini player height | 72px |
| Content padding | 16px |
| Card gap | 12px |
| Artwork (search result) | 48×48 px |
| Artwork (player) | min(80vw, 400px) |

### Touch targets

- Минимальный touch target: 44×44 px
- Кнопки воспроизведения: 48×48 px
- Отступ между кнопками: минимум 8px

### Скролл поведение

- Content area: `overflow-y: auto`, `-webkit-overflow-scrolling: touch`
- Mini player: `position: sticky; bottom: 0` (или fixed)
- Bottom nav: `position: fixed; bottom: 0`
- Mini player показывает progress bar как тонкую полоску сверху

### Swipe gestures

- Свайп вверх от Mini Player → Full Player
- Свайп вниз от Full Player → Mini Player
- Свайп влево по треку в списке → action buttons (delete, add to playlist)

---

## 12. Цветовая схема и стили

### Тёмная тема (primary)

| Элемент | Цвет |
|---|---|
| Background | `#121212` |
| Surface | `#1E1E1E` |
| Card | `#282828` |
| Primary | `#1DB954` (Spotify-green) |
| Text primary | `#FFFFFF` |
| Text secondary | `#B3B3B3` |
| Accent | `#1DB954` |
| Error | `#E53935` |
| Warning | `#FFA726` |
| Success | `#66BB6A` |

### Светлая тема

| Элемент | Цвет |
|---|---|
| Background | `#FAFAFA` |
| Surface | `#FFFFFF` |
| Card | `#F5F5F5` |
| Primary | `#1DB954` |
| Text primary | `#212121` |
| Text secondary | `#757575` |

### Типографика

| Элемент | Размер | Weight |
|---|---|---|
| H1 (page title) | 28px | 700 |
| H2 (section title) | 22px | 600 |
| H3 (card title) | 18px | 600 |
| Body | 16px | 400 |
| Caption | 14px | 400 |
| Small (badges) | 12px | 500 |

### Иконки

Рекомендуемый набор: Material Icons или Phosphor Icons.

| Действие | Иконка |
|---|---|
| Play | ▶️ (play) |
| Pause | ⏸ (pause) |
| Next | ⏭ (skip_next) |
| Previous | ⏮ (skip_previous) |
| Favorite | ❤️ (heart) / 🤍 (heart_outline) |
| Add to playlist | ➕ (playlist_add) |
| Download | ⬇️ (download) |
| More | ⋮ (more_vert) |
| Search | 🔍 (search) |
| Settings | ⚙️ (settings) |
| Home | 🏠 (home) |
| Library | 📚 (library_music) |
| Playlist | 🎵 (queue_music) |
