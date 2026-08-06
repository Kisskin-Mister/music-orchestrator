# Music Orchestrator v0.3.1 — Исправление 4 критических багов

## Контекст

Проект: `/home/kisskin/music-orchestrator`  
Backend: Go (`main.go` и др. в корне)  
Flutter web: `mobile/` (Docker контейнер `music-flutter-web` на порту 5174)  
React frontend: `frontend/` (Vite на порту 5173)  
Backend: `:18080`  

Текущая ветка: `github/go-main`  
Фронтенд отдаётся через Traefik → nginx (Flutter web контейнер). API запросы идут same-origin через Traefik на backend.

---

## БАГ 1: Бесконечный скролл сломан — только 20 результатов

### Симптом
Поиск в Flutter показывает ровно 20 результатов и не подгружает больше при скролле.

### Корневая причина
Файл: `main.go`, функция `search()`, ~строка 340-360.

```go
fetch := offset + limit + 1          // запрашивает 21 элемент
items := a.providers.Search(q, providers, fetch)  // yt-dlp возвращает N элементов
total := len(items)                  // ← ПРОБЛЕМА: total = реальное кол-во от yt-dlp
```

**Проблема**: yt-dlp (`ytsearch21:query`) **не гарантирует** ровно 21 результат. Часто возвращает 20. Когда `total = 20` и `_searchFetched = 20`, Flutter видит `canLoadMoreResults = (20 < 20) = false` и пагинация прекращается.

Трюк "+1" (запросить на 1 больше) работает ТОЛЬКО если yt-dlp возвращает запрошенное количество. Если yt-dlp вернул 20 вместо 21, `total = 20` и клиент думает что это все результаты.

### Решение
В `main.go` в функции `search()`:

```go
// БЫЛО:
total := len(items)

// СТАЛО:
total := len(items)
// Если вернули столько же, сколько запросили — возможно есть ещё.
// Устанавливаем total > fetch чтобы клиент попробовал загрузить ещё.
if total >= fetch {
    total = fetch + 1
}
```

Это заставит клиента вызвать `loadMoreResults()` снова. Если следующая страница вернёт пустой массив — пагинация остановится корректно (в `loadMoreResults()` Flutter: `if (result.items.isEmpty) searchTotal = _searchFetched;`).

### Файлы для изменения
- `main.go` — функция `search()`, строка ~351

---

## БАГ 2: SoundCloud не работает

### Симптом
SoundCloud поиск ничего не возвращает или треки не играют.

### Корневая причина
yt-dlp выдаёт предупреждение при каждом запуске:
```
WARNING: No supported JavaScript runtime could be found. Only deno is enabled by default.
```

SoundCloud использует HLS (m3u8 плейлисты). Без JS runtime yt-dlp может не корректно обрабатывать некоторые SoundCloud URL. Также `deno` не установлен, а `node` есть но не сконфигурирован для yt-dlp.

Дополнительно: SoundCloud DRM-protected треки возвращают только 30-секундные превью — это ограничение SoundCloud, не баг.

### Решение
1. Создать конфиг yt-dlp чтобы использовать `node` как JS runtime:
   ```bash
   mkdir -p ~/.config/yt-dlp
   echo "--js-runtimes node" > ~/.config/yt-dlp/config
   ```
   `node` уже установлен: `/usr/bin/node`.

2. В `extractor.go` добавить `--js-runtimes node` к аргументам yt-dlp (функция `dump()`), чтобы не зависеть от системного конфига.

### Файлы для изменения
- `extractor.go` — функция `dump()`, добавить `--js-runtimes node` к аргументам команды
- (Опционально) создать `~/.config/yt-dlp/config` как fallback

---

## БАГ 3: Мини-плейер пропадает на странице плейлиста

### Симптом
При переходе из списка плейлистов в конкретный плейлист — нижний мини-плейер исчезает.

### Корневая причина
Файл: `playlist_detail_screen.dart`

`PlaylistDetailScreen` открывается через `Navigator.push()` как новый route. Это создаёт **новый Scaffold** без `bottomNavigationBar` из `shell.dart`. В shell'е мини-плейер расположен в `bottomNavigationBar`:

```dart
// shell.dart
bottomNavigationBar: Column(
  mainAxisSize: MainAxisSize.min,
  children: [
    const MiniPlayer(),      // ← ЖИВЁТ ЗДЕСЬ
    NavigationBar(...),
  ],
),
```

Когда `PlaylistDetailScreen` делает `Navigator.push()`, он перекрывает shell своим Scaffold, и `MiniPlayer` исчезает.

### Решение
Добавить `MiniPlayer` в `PlaylistDetailScreen` как `bottomNavigationBar`:

```dart
// playlist_detail_screen.dart
@override
Widget build(BuildContext context) {
  return Scaffold(
    appBar: AppBar(...),
    bottomNavigationBar: const MiniPlayer(),  // ← ДОБАВИТЬ
    body: ListView(...),
    floatingActionButton: FloatingActionButton(...),
  );
}
```

### Файлы для изменения
- `mobile/lib/screens/playlist_detail_screen.dart` — добавить `import '../widgets/mini_player.dart'` и `bottomNavigationBar: const MiniPlayer()`

---

## БАГ 4: YouTube загрузка очень медленная

### Симптом
Поиск YouTube треков занимает 5+ секунд. С самой Pi (localhost) — быстро, но с внешних клиентов — медленно.

### Корневая причина
1. Каждый вызов поиска запускает **холодный процесс yt-dlp** — fork + Python startup ~2-3s overhead.
2. Для страницы 2 пагинации: `offset=20, limit=20` → `fetch = 41` → yt-dlp запрашивает 41 результат **с нуля** (нет реального offset у yt-dlp search), каждый раз ~5s.
3. Нет кеширования результатов поиска — один и тот же запрос `ytsearch21:query` запускается заново.
4. yt-dlp без JS runtime (deno) — fallback на `android_vr` client, который может быть медленнее.

### Решение
1. **Кешировать результаты поиска** в памяти (in-memory cache с TTL 60-300s):
   ```go
   // extractor.go или новый search_cache.go
   type searchCacheEntry struct {
       items   []Track
       fetched time.Time
   }
   // key = "providerID:query:limit"
   // TTL = 5 минут
   ```
   
   Это сделает повторные поиски мгновенными и ускорит пагинацию (страница 2 с `fetch=41` не будет перезапускать yt-dlp если результаты для `fetch=21` уже в кеше).

2. **Установить `--js-runtimes node`** (см. Баг 2) — ускорит SoundCloud и некоторые YouTube извлечения.

3. **Увеличить timeout** для Flutter API search с 45s до 60s (`api_client.dart` строка 242), чтобы медленные запросы не обрывались.

### Файлы для изменения
- `extractor.go` — добавить in-memory кеш для `Search()` с TTL
- (Опционально) `mobile/lib/api/api_client.dart` — увеличить search timeout

---

## Порядок исправления

1. **Баг 1** (infinite scroll) — одна строка в `main.go`, самое критичное
2. **Баг 3** (мини-плейер) — 2 строки в Flutter, быстро
3. **Баг 2** (SoundCloud JS runtime) — 1 строка в `extractor.go` + конфиг
4. **Баг 4** (медленный YouTube) — кеш в `extractor.go`, самое объёмное

## Проверки после исправления

```bash
# Backend тесты
cd /home/kisskin/music-orchestrator && go test ./...

# Backend сборка и запуск
go build -o bin/music-orchestrator . && APP_ADDR=:18080 APP_ENABLE_RISKY_EXTRACTORS=true ./bin/music-orchestrator

# Проверка infinite scroll
curl 'http://localhost:18080/v1/search?q=test&limit=20&offset=0&providers=youtube_stream' | jq '.total'
# total должен быть > 20 если есть ещё результаты

# Проверка SoundCloud
curl 'http://localhost:18080/v1/search?q=test&limit=3&providers=soundcloud_stream' | jq '.items | length'

# Flutter сборка
cd /home/kisskin/music-orchestrator/mobile && flutter pub get && flutter build web --release

# Docker контейнер
docker build -f Dockerfile.flutter -t music-flutter-web:latest .
docker restart music-flutter-web

# Flutter тесты
cd /home/kisskin/music-orchestrator/mobile && flutter test
```

## Важные файлы (для справки)

| Файл | Описание |
|------|----------|
| `main.go` | Backend HTTP handlers, CORS, search pagination |
| `extractor.go` | yt-dlp обёртка, Search(), dump() |
| `config.go` | APP_CORS_ORIGINS, APP_ENABLE_RISKY_EXTRACTORS |
| `mobile/lib/state/library_controller.dart` | Flutter search state, loadMoreResults() |
| `mobile/lib/screens/search_screen.dart` | Flutter search UI, scroll listener |
| `mobile/lib/screens/shell.dart` | Shell scaffold с MiniPlayer в bottomNavigationBar |
| `mobile/lib/screens/playlist_detail_screen.dart` | Playlist detail — нет MiniPlayer |
| `mobile/lib/widgets/mini_player.dart` | MiniPlayer виджет |
| `mobile/lib/api/api_client.dart` | Flutter API клиент |

## Архитектурные замечания

- yt-dlp **не поддерживает** настоящее пагинацию для search. `ytsearch` всегда возвращает результаты с начала. Backend компенсирует это перезапросом с большим `limit`, но это медленно.
- SoundCloud DRM: некоторые треки (крупные лейблы) возвращают только 30s preview — это ограничение SoundCloud, не баг.
- Flutter web отдаётся через nginx в Docker контейнере. API запросы идут same-origin через Traefik на backend.
