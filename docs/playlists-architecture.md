# Playlists Architecture

## Цель

Сделать плейлисты нормальным продуктовым слоем Music Orchestrator, а не набором временных кнопок: пользователь создаёт плейлисты, добавляет туда треки из поиска/медиатеки, сортирует порядок, удаляет треки, запускает плейлист как очередь и не видит технические `track_id`.

## Принципы

- **Track snapshot везде.** Плейлист хранит не только `track_id`, а user-friendly snapshot: `title`, `artist`, `album`, `duration_seconds`, `artwork_url`, `provider_id`, `provider_track_id`, `source_url`, `official`, `downloaded`, `download_media_url`.
- **Плейлист ≠ очередь.** Плейлист — сохранённая коллекция. Queue — runtime состояние плеера.
- **Никаких raw id в UI.** В списках показываем обложку, название, артист, duration, source icon, local folder icon.
- **Операции идемпотентные.** Повторное добавление трека в тот же плейлист не должно создавать дубль без явного действия duplicate.
- **Порядок — first-class.** У каждого item есть `position`; reorder не должен пересоздавать плейлист.
- **Soft future-proof.** Сейчас JSON store, но модель должна легко переехать в SQLite/Postgres без изменения API.

## Domain model

### Playlist

```go
type Playlist struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    CoverURL    string          `json:"cover_url,omitempty"`
    TrackCount  int             `json:"track_count"`
    DurationSeconds int         `json:"duration_seconds"`
    Tracks      []PlaylistTrack `json:"tracks,omitempty"`
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`
}
```

### PlaylistTrack

```go
type PlaylistTrack struct {
    ID        string    `json:"id"`          // stable item id for delete/reorder
    TrackID   string    `json:"track_id"`
    Track     Track     `json:"track"`       // snapshot at add time
    Position  int       `json:"position"`
    AddedAt   time.Time `json:"added_at"`
}
```

Почему нужен `PlaylistTrack.ID`: удалять/двигать строку удобнее по item id. Если один и тот же трек позже разрешим дублировать, API не сломается.

## Backend API v1

### Уже есть, но нужно усилить

- `GET /v1/playlists`
  - Возвращает lightweight список без полного массива tracks или с `tracks` пустым.
  - Поля: id, name, description, cover_url, track_count, duration_seconds, created_at, updated_at.

- `POST /v1/playlists`
  - Body: `{ "name": string, "description"?: string }`
  - Validation: name 1..80 символов, trim, no empty.

- `GET /v1/playlists/{playlist_id}`
  - Возвращает playlist + ordered tracks.

- `POST /v1/playlists/{playlist_id}/tracks`
  - Body: `{ "track_id": string }`
  - Backend резолвит `Track` через providers/store, сохраняет snapshot.
  - Если track уже есть: вернуть существующий playlist без дубля или `409 duplicate_track` — выбрать один режим и закрепить тестом. Для текущего UX лучше **идемпотентно без дубля**.

### Добавить

- `DELETE /v1/playlists/{playlist_id}`
  - Удаляет playlist, не трогает downloads/favorites/media files.

- `PATCH /v1/playlists/{playlist_id}`
  - Body: `{ "name"?: string, "description"?: string }`
  - Переименование/описание.

- `DELETE /v1/playlists/{playlist_id}/tracks/{item_id}`
  - Удаляет конкретную строку, затем нормализует positions `0..n-1`.

- `PUT /v1/playlists/{playlist_id}/tracks/reorder`
  - Body: `{ "items": [{ "id": string, "position": number }] }`
  - Проверить, что ids принадлежат этому playlist, positions уникальные.

- `POST /v1/playlists/{playlist_id}/play`
  - Не обязательно сразу: можно пока делать на frontend через `setQueue(playlist.tracks.map(t => t.track))`.

## Store layer

Текущий `Store` JSON можно оставить, но методы должны быть атомарными под mutex:

- `CreatePlaylist(name, desc string) (Playlist, error)`
- `UpdatePlaylist(id string, patch PlaylistPatch) (Playlist, error)`
- `DeletePlaylist(id string) error`
- `ListPlaylists() []Playlist` — lightweight с агрегатами.
- `GetPlaylist(id string) (Playlist, bool)` — full.
- `AddPlaylistTrack(id string, track Track) (Playlist, error)` — сохраняет snapshot, без дублей.
- `DeletePlaylistTrack(id, itemID string) (Playlist, error)`
- `ReorderPlaylistTracks(id string, positions map[string]int) (Playlist, error)`

Агрегаты считаем при каждом изменении:

- `track_count = len(tracks)`
- `duration_seconds = sum(track.duration_seconds)`
- `cover_url = first non-empty artwork_url`

## Frontend architecture

### API client

Добавить в `frontend/frontend/src/api/types.ts`:

- `Playlist`
- `PlaylistTrack`
- `PlaylistSummary`

Добавить в `frontend/frontend/src/api/client.ts`:

- `playlists()`
- `playlist(id)`
- `createPlaylist(name, description?)`
- `updatePlaylist(id, patch)`
- `deletePlaylist(id)`
- `addPlaylistTrack(playlistId, trackId)`
- `deletePlaylistTrack(playlistId, itemId)`
- `reorderPlaylistTracks(playlistId, items)`

Добавить React Query hooks в `queries.ts`:

- `usePlaylists`
- `usePlaylist`
- `useCreatePlaylist`
- `useUpdatePlaylist`
- `useDeletePlaylist`
- `useAddPlaylistTrack`
- `useDeletePlaylistTrack`
- `useReorderPlaylistTracks`

Invalidation:

- Любая mutation по playlist invalidates `['playlists']` и `['playlist', playlistId]`.

### UI flow

- Sidebar order: `Медиатека`, `Поиск`, `Плейлисты`, `Загрузки`, `Настройки`.
- Overflow menu у track:
  - `Скачать mp3`
  - `Добавить в плейлист` → opens playlist picker sheet.
  - `Удалить локально`, если downloaded.
- Playlist picker:
  - список существующих плейлистов;
  - `+ Новый плейлист` inline;
  - после добавления toast/status: `Добавлено в <name>`.
- Playlists view:
  - grid/list cards: cover collage, name, track_count, duration;
  - click card → detail view;
  - detail view uses same `TrackRow` component;
  - row actions: remove from playlist, download, delete local, like.

## Player/queue integration

В `store/player.ts` расширить runtime state:

```ts
type PlayerState = {
  queue: Track[];
  queueIndex: number;
  setQueue: (tracks: Track[], startIndex?: number) => void;
  playNext: () => void;
  playPrevious: () => void;
}
```

Behavior:

- Click track in playlist detail: `setQueue(playlistTracks, index)`.
- Next:
  - shuffle off: next index;
  - shuffle on: random index except current when possible;
  - repeat one: handled by audio loop;
  - repeat all: after last → first;
  - repeat off: after last → pause.
- Previous: previous index or restart if currentTime > 3s later.

## Testing plan

### Go tests

Add tests in `main_test.go` or dedicated playlist test file:

1. `POST /v1/playlists` validates empty name.
2. `POST /v1/playlists` creates playlist with aggregates zero.
3. `POST /v1/playlists/{id}/tracks` stores track snapshot, not just id.
4. Adding same track twice does not duplicate.
5. `DELETE /v1/playlists/{id}/tracks/{item_id}` removes and normalizes positions.
6. `PUT reorder` rejects foreign ids / duplicate positions.
7. `DELETE /v1/playlists/{id}` does not delete favorite/download/media.

Commands:

```bash
go test ./...
go build ./...
```

### Frontend tests

Add/extend Vitest tests:

1. API client encodes playlist ids and protected requests.
2. Playlist hooks invalidate `playlists` and `playlist:id`.
3. Player queue next/previous/repeat behavior in store unit tests.

Commands:

```bash
cd frontend/frontend
npm test -- --run
npm run build
```

### Browser smoke

- Open public URL with cache buster.
- Create playlist.
- Search track.
- Add track via three dots.
- Open playlist.
- Click cover → plays immediately.
- Next/previous buttons change current track if queue has >1.
- Delete track from playlist.
- Confirm no console errors.

## Implementation order

1. Backend models/store tests RED.
2. Backend store + handlers GREEN.
3. API client/types/hooks tests RED/GREEN.
4. Player queue store tests RED/GREEN.
5. Playlist picker UI in TrackRow.
6. Playlists list/detail UI.
7. Browser smoke + deploy.

## Risks

- Current JSON store is OK for MVP, but reorder/concurrent writes need mutex discipline.
- YouTube provider can fail to re-resolve old track metadata; поэтому snapshot обязателен.
- UI menus can become overloaded: keep row actions behind three dots, player controls only in player.
- Mobile bottom nav/player overlap: every playlist sheet/modal must account for safe bottom padding.
