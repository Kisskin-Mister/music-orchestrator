# TZ: Design Parity Desktop=Flutter + Icon Redesign

## Goal
1. Make the React desktop web app look **identical** to the Flutter mobile app (tokens, layout, components)
2. Replace the app icon: black background + white music note WITHOUT the curved flag ("hat")

---

## TASK 1: Design Tokens — Sync CSS with Flutter `tokens.dart`

The Flutter app's `mobile/lib/theme/tokens.dart` defines the canonical design tokens. The React CSS in `frontend/frontend/src/styles.css` MUST match them exactly.

### Current Flutter tokens (`tokens.dart`):
```
bg         = #08090C
surface    = #0F1117
surface2   = #151923
surface3   = #1C212C
fg         = #F6F3EA
muted      = #8C919E
subtle     = #626875
border     = rgba(255,255,255,0.10)
borderStrong = rgba(255,255,255,0.15)
danger     = #EB777C
defaultAccent = #B8F545

AppRadius.control = 10
AppRadius.card    = 16
AppRadius.sheet   = 28

AppFonts.display = 'Unbounded'
AppFonts.mono    = 'JetBrains Mono'
Body font        = 'SF Pro Text'
```

### Current Flutter theme (`app_theme.dart`):
- `useMaterial3: true`
- `splashFactory: NoSplash.splashFactory` — no ripple effects
- `highlightColor: Colors.transparent`
- Headline Large: Unbounded, w800, letter-spacing -0.5, height 1.0
- Headline Medium: Unbounded, w700, letter-spacing -0.3
- Title Large: Unbounded, w700, letter-spacing -0.2
- Body Medium: 15px, height 1.4, color fg
- Body Small: 13px, color muted
- Label Small: JetBrains Mono, muted, 11px, letter-spacing 1.4
- Input fill: surface, border: borderStrong, radius: control (10), focus border: accent@60% alpha
- Buttons: accent bg, accentInk fg, radius: control, no elevation

### What to fix in `styles.css`:
- Ensure `--surface-2` matches `#151923` (already does)
- Ensure `--surface-3` matches `#1C212C` (already does)
- Body font stack should include `SF Pro Text`: `"SF Pro Text", "Geist", ui-sans-serif, system-ui, sans-serif`
- Add `--radius-control: 10px; --radius-card: 16px; --radius-sheet: 28px;` CSS variables
- Add `--subtle: #626875; --border-strong: rgba(255,255,255,0.15); --danger: #EB777C;`
- Global: `* { -webkit-tap-highlight-color: transparent; }` (no ripple, matches Flutter NoSplash)

---

## TASK 2: Page Headers — Match Flutter Pattern

Every Flutter screen has this header pattern:
```dart
Text('КОЛЛЕКЦИЯ', style: labelSmall.copyWith(color: accent))  // eyebrow uppercase, accent color
SizedBox(height: 6)
Text('Медиатека', style: headlineLarge.copyWith(fontSize: 40))  // big title
SizedBox(height: 8)
Text('Всё, что ты лайкнул или скачал, — в одном месте.')  // subtitle muted
```

### Flutter screens and their headers:
| Screen     | Eyebrow      | Title       | Subtitle                                          |
|------------|--------------|-------------|---------------------------------------------------|
| Home       | КОЛЛЕКЦИЯ    | Медиатека   | Всё, что ты лайкнул или скачал, — в одном месте. |
| Search     | ПОИСК        | Что послушаем? | (search input below)                           |
| Playlists  | КОЛЛЕКЦИИ    | Плейлисты   | (none)                                            |
| Downloads  | ЗАГРУЗКИ     | Загрузки    | (similar pattern)                                 |
| Settings   | ПРОФИЛЬ И BACKEND | Настройки | Аккаунт, источники и подключение к серверу.    |

### In React SearchPage.tsx:
Each view (LibraryView, SearchView, PlaylistsView, DownloadsView, SettingsView) must use this EXACT pattern:
- Eyebrow: `font-mono text-[11px] tracking-[1.4px] uppercase` with `text-lime-300` (accent color)
- Title: `font-display text-[40px] font-extrabold tracking-[-0.5px] leading-none`
- Subtitle: `text-sm text-[#8c919e]` (muted)
- Spacing: eyebrow→title = 6px, title→subtitle = 8px, subtitle→content = 16px

---

## TASK 3: Navigation Bar

### Flutter (`shell.dart`):
- 5 tabs: Медиатека, Поиск, Плейлисты, Загрузки, Настройки
- Icons: library_music, search, queue_music, download, settings (outlined when inactive, rounded when active)
- Bottom navigation: `NavigationDestination` with icon + label
- MiniPlayer sits ABOVE the nav bar
- Active tab: accent color foreground

### Current React:
- Sidebar on desktop (lg+), bottom bar on mobile
- Desktop sidebar: icon only (xl shows label), 76px (lg) / 240px (xl)
- Active: `bg-lime-300 text-black`

### Required changes:
- The current approach (sidebar desktop, bottom bar mobile) is CORRECT for web — keep it
- But ensure the desktop sidebar matches Flutter's visual weight:
  - Nav item padding, radius, hover states should feel the same
  - Active state: accent bg + dark text (already does this)
  - Inactive: muted text `#9aa0ad` (already does this)
- Ensure the sidebar has the same 5 tabs as Flutter (already does)
- The sidebar logo area: `<Music2>` icon in accent box + "Orchestrator" text on xl (already does)

---

## TASK 4: Track Row — Match Flutter `TrackRow`

### Flutter (`track_row.dart`):
- Padding: `horizontal: 4, vertical: 8`
- Current track: `accent.withValues(alpha: 0.08)` background, radius control (10)
- Art: 52×52, radius control (10)
- Gap: 12px
- Title: `fontWeight: w600`, 1 line, ellipsis
- Below title (3px gap): row with source icon (14px) + artist (muted, 13px) + dot + duration (muted, mono, 12px)
- Right side: action buttons (like, download, more) as `IconAction` widgets (48×48 circular)

### Required in React:
- Match EXACTLY: 52×52 art, radius 10px, 12px gap, same typography
- Current track highlight: accent at 8% opacity bg
- Action buttons: circular, 40-48px, muted when inactive, accent when active
- Duration in mono font (JetBrains Mono)

---

## TASK 5: Mini Player — Match Flutter `MiniPlayer`

### Flutter (`mini_player.dart`):
- Margin: `fromLTRB(12, 0, 12, 8)` — floats above nav
- Padding: 8px all
- Background: `surface2` (#151923)
- Radius: card (16)
- Shadow: `Colors.black45, blurRadius: 24, offset: Offset(0, 8)`
- Layout: Row with [44×44 art] [10px gap] [title+artist column] [action buttons]
- Title: 13px, w600, 1 line ellipsis
- Artist: 12px, muted
- Progress: thin accent-colored bar at bottom of the container
- Tap → opens NowPlayingSheet

### Required in React:
- Match the floating bar style: `bg-[#151923]`, rounded-2xl (16px), shadow
- Same layout: art | info | controls
- Progress bar: thin accent bar at bottom

---

## TASK 6: Now Playing / Full Player — Match Flutter `NowPlayingSheet`

### Flutter (`now_playing_sheet.dart`):
- `DraggableScrollableSheet`: initial 0.9, min 0.5, max 0.95
- Background: `surface` (#0F1117)
- Top radius: sheet (28)
- Drag handle: 44×5 white24 rounded bar, centered, 24px below top
- Art: AspectRatio 1:1, radius card (16), full width minus padding
- Padding: `fromLTRB(20, 12, 20, 32)`
- Below art (24px gap): title (headlineMedium, 1 line) + artist (bodyMedium, muted)
- Below (24px): progress bar (thin, accent-colored track, rounded)
- Below (8px): time row (current / total, mono, muted, 12px)
- Below (24px): action row (like, download, shuffle, repeat) as IconAction circles
- Below (24px): transport row (prev, play/pause, next) — play/pause is 64px accent circle
- Below (32px): queue section with "Сейчас играет" eyebrow

### Required in React:
- Bottom sheet morph from mini player (already has this pattern)
- Match the exact layout order and spacing
- Play/pause: large 64px accent circle with dark icon
- Transport: prev/next as larger icon buttons (48px)
- Progress bar: rounded, accent track, thin

---

## TASK 7: Search Input — Match Flutter

### Flutter (`search_screen.dart`):
- TextField with hint: "Название, исполнитель или альбом"
- Input decoration: fill surface, border borderStrong, radius control (10)
- Below input (12px): source filter chips (horizontal Wrap, spacing 8)
- Each chip: icon + name, rounded, tinted bg when selected
- 350ms debounce

### Required in React:
- Match the input style: dark fill, border, rounded-2xl? No — rounded-[10px] (control radius)
- Source chips below input, not inline

---

## TASK 8: Settings Screen — Match Flutter

### Flutter (`settings_screen.dart`):
- Same header pattern (eyebrow + title + subtitle)
- Sections separated by `Divider(height: 40)`
- Section headers: labelSmall uppercase accent
- Accent picker: `Wrap` with `spacing: 14, runSpacing: 14`, each swatch is a circular color button
- Current swatch: ring border (2px accent)
- Account info card: surface bg, rounded card
- Source providers: list with icon + name + status badge
- Backend URL: in a text field

### Required in React:
- Match section structure exactly
- Accent picker: circular swatches with ring on selected
- Clean sections with dividers

---

## TASK 9: Empty States — Match Flutter

### Flutter patterns:
- Library empty: centered column with icon (40px, muted) + "Медиатека пуста" title + subtitle + accent CTA button
- Playlists empty: same pattern with queue_music icon
- Downloads empty: same pattern with download icon

---

## TASK 10: Cover Strip (Слушай снова) — Match Flutter

### Flutter (`cover_strip.dart`):
- Horizontal scrollable row
- Eyebrow: uppercase labelSmall (e.g., "СЛУШАЙ СНОВА")
- Each card: 140px wide, art is 140×140 radius card (16)
- Below art (8px): title (13px, w600, 1 line) + artist (12px, muted, 1 line)
- Gap between cards: 14px
- Max 12 items

---

## TASK 11: Icon Redesign

### Current icon:
- Lime green (#B8F545) square background
- Black music note with curved flag ("hat") at top of stem

### New icon design:
- **Background**: solid black (#08090C or #000000)
- **Foreground**: white music note
- **Note shape**: simple quarter note — oval note head + straight vertical stem, NO curved flag/hat at top
- **Style**: minimal, flat, same proportions as current
- **Padding**: ~15% margin from edges (same as current)

### Files to regenerate:
- `frontend/frontend/public/icon-16.png`
- `frontend/frontend/public/icon-32.png`
- `frontend/frontend/public/icon-192.png`
- `frontend/frontend/public/icon-512.png`
- `frontend/frontend/public/icon-maskable-512.png`
- `frontend/frontend/public/apple-touch-icon.png`
- `frontend/frontend/public/favicon.ico`
- `mobile/android/app/src/main/res/mipmap-*/ic_launcher.png` (hdpi, mdpi, xhdpi, xxhdpi, xxxhdpi)

### How to generate:
Use Python PIL/Pillow to create the icons programmatically:
```python
from PIL import Image, ImageDraw
# Black background, white note (oval + straight stem, no flag)
# Export all sizes
```

---

## TASK 12: Playlists Screen — Match Flutter

### Flutter (`playlists_screen.dart`):
- Header: КОЛЛЕКЦИИ / Плейлисты
- Grid or list of playlist cards
- Each card: cover art (playlist_art widget) + name + track count
- FAB: accent circle with + icon, bottom-right
- Empty state: queue_music icon + "Плейлистов пока нет" + subtitle + CTA

---

## CONSTRAINTS

1. **Do NOT touch**: `auth.go`, `store.go`, `mobile/`, `.env`, deployment files, backend Go code
2. **Only modify**: `frontend/frontend/src/` files and `frontend/frontend/public/` icon files
3. **Tests must pass**: `cd frontend/frontend && npm test -- --run`
4. **Build must pass**: `cd frontend/frontend && npm run build`
5. **Go tests must still pass**: `cd /home/kisskin/music-orchestrator && go test ./...`
6. **Commit** with descriptive message

---

## VERIFICATION

After all changes:
1. `cd /home/kisskin/music-orchestrator && go test ./...` — must pass
2. `cd frontend/frontend && npm test -- --run` — must pass
3. `cd frontend/frontend && npm run build` — must pass
4. `git diff --check` — no whitespace errors
5. Visual: desktop sidebar, track rows, mini player, full player, search, settings — all must match Flutter screenshots
