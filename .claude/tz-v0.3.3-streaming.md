# TZ v0.3.3 — SoundCloud Progressive Streaming (Tier 0 + Tier 2)

## Контекст
Проект: `/home/kisskin/music-orchestrator`
Backend: Go, ветка `github/go-main`
Проблема: SoundCloud треки загружаются 10-30 сек потому что ffmpeg скачивает ВСЕ HLS-сегменты прежде чем отдать первый байт клиенту.

## Архитектура сейчас

### YouTube (быстро)
`main.go:466` stream() → `openUpstream()` → ranged GET `bytes=0-4MB` → `io.Copy` + `Flush()` → клиент получает аудио сразу. Warm cache < 1 сек.

### SoundCloud (медленно)
`main.go:485` → `serveHLS()` → `materialize()`:
1. Per-path mutex — второй слушатель блокируется
2. Cache miss → `exec ffmpeg` с `cmd.Run()` — **блокирующий, ждёт полного завершения**
3. ffmpeg скачивает все HLS-сегменты последовательно (~30-60 GETов)
4. AAC → MP4+faststart (два прохода), MP3 → copy
5. `os.Rename` → только потом `http.ServeContent`
6. **НИ ОДИН БАЙТ не доходит до клиента пока ВСЁ не скачается**
7. Клиент отменил → `r.Context()` kill ffmpeg → `.partial` удаляется → следующий раз с нуля

Файлы: `hls.go` (201 строка), `extractor.go` (StreamTarget ~строка 297), `main.go` (stream ~строка 466, playback ~строка 404)

---

## Tier 0 — бесплатные фиксы (hls.go + extractor.go + main.go)

### 0A. Отвязать remux от контекста запроса
Файл: `hls.go`, строка 114

**Сейчас:**
```go
runCtx, cancel := context.WithTimeout(ctx, c.timeout)
```
`ctx` = request context. Клиент закрыл вкладку → ctx cancelled → ffmpeg убит → `.partial` удалён.

**Надо:**
```go
runCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
```

Также убрать `defer os.Remove(tmp)` (строка 112) — оставлять `.partial` как кеш для следующего запроса. Удалять только если ffmpeg реально упал с ошибкой (не timeout, не cancel).

### 0B. Warm-on-play из /v1/playback
Файл: `main.go`, строка 428-449 (функция `playback`)

**Сейчас:** `/v1/playback` возвращает `stream_url = /v1/stream/{id}`. Клиент начинает грузить `/v1/stream` → только тогда запускается remux.

**Надо:** После строки 445 (`stream := "/v1/stream/" + id`), если `target.HLS` — запустить goroutine:
```go
go func() {
    trackID := providerID + ":" + pid
    target, err := a.providers.extractor.StreamTarget(providerID, pid)
    if err != nil { return }
    if target.HLS {
        _, _, _ = a.hls.materialize(context.Background(), trackID, target)
    }
}()
```

Это даёт 1-3 сек форы пока клиент обрабатывает playback response и начинает стрим.

**Важно:** нужно вызвать `StreamTarget` внутри goroutine (не раньше), чтобы не блокировать playback response. Проверить `target.HLS` тоже внутри.

### 0C. Single-flight для StreamTarget
Файл: `extractor.go`, функция `StreamTarget` (~строка 297)

**Сейчас:** два одновременных запроса к одному треку → два yt-dlp процесса.

**Надо:** добавить `inflight sync.Map` (ключ → `*singleFlight{wg, target, err}`):
```go
type streamFlight struct {
    done   chan struct{}
    target StreamTarget
    err    error
}
```

В `StreamTarget`: `LoadOrStore` → если уже в полёте — `wait` на канале и вернуть результат. Это предотвращает дублирование yt-dlp.

### 0D. Увеличить streamCacheTTL
Файл: `extractor.go`, строка 24

**Сейчас:** `streamCacheTTL = 300 * time.Second` (5 мин)

**Надо:** `streamCacheTTL = 1 * time.Hour`

Комментарий: YouTube 403 re-resolve (main.go:628) уже восстанавливает протухшие ссылки, а SoundCloud ссылки живут дольше часа. 5 минут — слишком мало, трек > 5 мин перерезолвится при seek.

---

## Tier 2 — прогрессивный стриминг (hls.go)

### Суть
ffmpeg пишет в `.partial`. Handler читает из того же файла, блокируется на EOF, ждёт пока ffmpeg допишет. Первый байт за 1-2 сек (первый HLS-сегмент), а не за 10-30.

### 2A. Новый метод `materializeProgressive`
Файл: `hls.go`

Вместо `materialize()` + блокирующего `cmd.Run()` + `ServeContent`:

```go
func (c *hlsCache) materializeProgressive(ctx context.Context, trackID string, target StreamTarget) (string, hlsContainer, <-chan error) {
    container := containerFor(target)
    path := c.pathFor(trackID, container)
    
    // Если файл уже есть и полный — вернуть сразу
    if info, err := os.Stat(path); err == nil && info.Size() > 0 {
        ch := make(chan error, 1)
        ch <- nil
        return path, container, ch
    }
    
    tmp := path + ".partial"
    os.MkdirAll(c.dir, 0755)
    
    // Если .partial уже существует и растёт — просто подключаемся
    if info, err := os.Stat(tmp); err == nil && info.Size() > 0 {
        ch := make(chan error, 1)
        // Проверим что ffmpeg ещё пишет (или файл уже готов)
        go func() {
            // Ждём пока .partial станет полным (rename произойдёт)
            for {
                if _, err := os.Stat(path); err == nil {
                    ch <- nil
                    return
                }
                time.Sleep(200 * time.Millisecond)
            }
        }()
        return tmp, container, ch
    }
    
    // Запускаем ffmpeg в фоне
    done := make(chan error, 1)
    go func() {
        runCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
        defer cancel()
        
        args := []string{"-hide_banner", "-loglevel", "error", "-y"}
        if headers := ffmpegHeaders(target.Headers); headers != "" {
            args = append(args, "-headers", headers)
        }
        args = append(args, "-i", target.URL, "-vn")
        args = append(args, container.args...)
        args = append(args, tmp)
        
        cmd := exec.CommandContext(runCtx, c.ffmpeg, args...)
        var stderr strings.Builder
        cmd.Stderr = &stderr
        
        if err := cmd.Run(); err != nil {
            os.Remove(tmp)
            done <- fmt.Errorf("ffmpeg remux failed: %w: %s", err, stderr.String())
            return
        }
        
        info, err := os.Stat(tmp)
        if err != nil || info.Size() == 0 {
            os.Remove(tmp)
            done <- fmt.Errorf("ffmpeg produced empty file")
            return
        }
        
        os.Rename(tmp, path)
        done <- nil
    }()
    
    return tmp, container, done
}
```

### 2B. Follower reader
Файл: `hls.go`

```go
// followerReader wraps an *os.File and blocks at EOF until either more data
// appears or the done channel signals completion.
type followerReader struct {
    f    *os.File
    done <-chan error
    pos  int64
}

func (fr *followerReader) Read(p []byte) (int, error) {
    for {
        n, err := fr.f.ReadAt(p, fr.pos)
        if n > 0 {
            fr.pos += int64(n)
            return n, nil
        }
        if err != io.EOF {
            return 0, err
        }
        // EOF — проверяем не завершился ли ffmpeg
        select {
        case remuxErr := <-fr.done:
            if remuxErr != nil {
                return 0, remuxErr
            }
            // Файл готов, пробуем ещё раз
            n, err = fr.f.ReadAt(p, fr.pos)
            if n > 0 {
                fr.pos += int64(n)
                return n, nil
            }
            return 0, io.EOF
        case <-time.After(100 * time.Millisecond):
            // ffmpeg ещё пишет, пробуем снова
            continue
        }
    }
}
```

### 2C. Новый `serveHLS` — прогрессивный
Файл: `hls.go`, замена `serveHLS` (строка 173)

```go
func (a *App) serveHLS(w http.ResponseWriter, r *http.Request, trackID string, target StreamTarget) {
    providerID, _, _ := splitTrackID(trackID)
    message := unavailableMessage(providerID)
    
    path, container, done := a.hls.materializeProgressive(r.Context(), trackID, target)
    
    // Ждём пока файл станет доступен (первый сегмент)
    waitForFile(path, 10*time.Second)
    
    file, err := os.Open(path)
    if err != nil {
        writeError(w, http.StatusBadGateway, message)
        return
    }
    defer file.Close()
    
    info, err := file.Stat()
    if err != nil || info.Size() == 0 {
        writeError(w, http.StatusBadGateway, message)
        return
    }
    
    w.Header().Set("Content-Type", container.contentType)
    w.Header().Set("Accept-Ranges", "bytes")  // Scrubber работает
    
    // Оценочный Content-Length для CBR MP3
    // (duration * bitrate / 8) — позволяет показать прогресс-бар
    if target.Duration > 0 && container.ext == "mp3" {
        estimated := int64(target.Duration * 192000 / 8)
        w.Header().Set("Content-Length", strconv.FormatInt(estimated, 10))
    }
    
    // Прогрессивный стриминг
    flusher, _ := w.(http.Flusher)
    fr := &followerReader{f: file, done: done}
    
    buf := make([]byte, 32*1024)  // 32KB chunks
    for {
        n, err := fr.Read(buf)
        if n > 0 {
            w.Write(buf[:n])
            if flusher != nil {
                flusher.Flush()
            }
        }
        if err != nil {
            break
        }
        // Клиент отменился
        if r.Context().Err() != nil {
            break
        }
    }
}
```

### 2D. Добавить Duration в StreamTarget
Файл: `extractor.go`, структура `StreamTarget` (строка 60)

Добавить поле:
```go
type StreamTarget struct {
    URL      string
    Headers  map[string]string
    HLS      bool
    ACodec   string
    Ext      string
    Duration float64  // секунды из yt-dlp metadata
}
```

В `StreamTarget()` функции — заполнять `Duration` из `info.Duration` (yt-dlp даёт это поле).

### 2E. Формат: prefer MP3 over AAC for progressive
Файл: `hls.go`, функция `containerFor` (строка 62)

Для прогрессивного стриминга MP3 лучше AAC:
- MP3 не нужен faststart (нет moov atom)
- MP3 можно пайпить/читать прогрессивно
- AAC+faststart требует два прохода → не стримится

Изменить `containerFor` так чтобы prefer MP3 copy over AAC+faststart когда доступны оба.

---

## Тесты

### Файл: `hls_test.go` (новый)

1. `TestFollowerReader` — создать файл, писать в него из goroutine с задержками, читать из followerReader, проверить что Read блокируется на EOF и продолжает когда данные появляются.

2. `TestMaterializeProgressiveCacheHit` — закешированный файл возвращается сразу.

3. `TestMaterializeProgressiveConcurrent` — два reader'а подключаются к одному .partial файлу.

4. `TestHLSDetachFromRequestContext` — отмена request context не убивает ffmpeg (Tier 0A).

### Файл: `stream_test.go` — добавить

5. `TestStreamHLSProgressive` — fake m3u8 server + stub ffmpeg script, проверить что байты доходят до клиента ДО завершения remux.

---

## НЕ ТРОГАТЬ
- YouTube стриминг (main.go stream handler) — работает идеально
- Поиск, пагинация, interleaving — всё в порядке
- Flutter/Dart код — не менять
- auth, store, playlists — не трогать

## Порядок реализации
1. Tier 0A (detach context) — одна строка
2. Tier 0D (streamCacheTTL) — одна строка
3. Tier 0C (single-flight) — ~30 строк
4. Tier 0D (warm-on-play) — ~15 строк
5. Tier 2A-2E (progressive streaming) — основная работа
6. Тесты

## Проверки
```bash
go test ./...
go build -o bin/music-orchestrator .
# Ручной smoke test:
APP_ADDR=:18080 APP_ENABLE_RISKY_EXTRACTORS=true ./bin/music-orchestrator
# В другом терминале:
time curl -o /dev/null -w "TTFB: %{time_starttransfer}s, Total: %{time_total}s\n" \
  -H "Range: bytes=0-1023" \
  "http://localhost:18080/v1/stream/soundcloud_stream:url_$(echo -n 'https://soundcloud.com/tien-pham-418156952/she-neva-know-lofi-ver' | base64 -w0)"
# Ожидание: TTFB < 3 сек (было 10-30 сек)
```

## Важные файлы
| Файл | Строка | Что менять |
|------|--------|------------|
| `hls.go` | 114 | `context.Background()` вместо `ctx` |
| `hls.go` | 112 | Убрать `defer os.Remove(tmp)` |
| `hls.go` | 62 | prefer MP3 over AAC |
| `hls.go` | NEW | `followerReader`, `materializeProgressive` |
| `hls.go` | 173 | Новый `serveHLS` с прогрессивным стримингом |
| `extractor.go` | 24 | `streamCacheTTL = 1 * time.Hour` |
| `extractor.go` | 60 | Добавить `Duration` в `StreamTarget` |
| `extractor.go` | ~297 | Single-flight для StreamTarget |
| `main.go` | ~445 | Warm-on-play goroutine |
