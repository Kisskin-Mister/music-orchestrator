# TZ: iOS MediaSession + SoundCloud fix

## Контекст
Проект: `/home/kisskin/music-orchestrator`
Flutter app: `mobile/`
Backend: Go, `:18080`
Текущая ветка: `github/go-main`

## Задача 1: iOS MediaSession (Now Playing Info)

### Проблема
На iOS в системном плеере (lock screen, Control Center, Dynamic Island) показывается:
- Название приложения вместо названия трека
- Кнопки ±30 сек вместо prev/next track
- Нет обложки и имени автора

### Причина
Используется только `just_audio` без `audio_service`. `just_audio` не устанавливает `MPNowPlayingInfoCenter` метаданные и не обрабатывает кнопки управления медиа.

### Решение
Добавить пакет `audio_service` и интегрировать его с `PlayerController`.

#### Шаг 1: Добавить зависимости в `mobile/pubspec.yaml`
```yaml
dependencies:
  audio_service: ^0.18.17
  audio_session: ^0.1.25
```

#### Шаг 2: Создать `mobile/lib/services/music_audio_handler.dart`
```dart
import 'package:audio_service/audio_service.dart';
import 'package:just_audio/just_audio.dart';

class MusicAudioHandler extends BaseAudioHandler with QueueHandler, SeekHandler {
  final AudioPlayer _player;
  
  MusicAudioHandler(this._player) {
    _player.playbackEventStream.listen(_broadcastState);
    _player.processingStateStream.listen((state) {
      if (state == ProcessingState.completed) {
        // Notify audio_service that playback ended
        playbackState.add(playbackState.value.copyWith(
          processingState: AudioProcessingState.completed,
        ));
      }
    });
  }

  void _broadcastState(PlaybackEvent event) {
    playbackState.add(playbackState.value.copyWith(
      controls: [
        MediaControl.skipToPrevious,
        if (_player.playing) MediaControl.pause else MediaControl.play,
        MediaControl.skipToNext,
        MediaControl.stop,
      ],
      systemActions: const {
        MediaAction.seek,
        MediaAction.seekForward,
        MediaAction.seekBackward,
      },
      androidCompactActionIndices: const [0, 1, 2],
      processingState: const {
        ProcessingState.idle: AudioProcessingState.idle,
        ProcessingState.loading: AudioProcessingState.loading,
        ProcessingState.buffering: AudioProcessingState.buffering,
        ProcessingState.ready: AudioProcessingState.ready,
        ProcessingState.completed: AudioProcessingState.completed,
      }[_player.processingState]!,
      playing: _player.playing,
      updatePosition: _player.position,
      bufferedPosition: _player.bufferedPosition,
      speed: _player.speed,
    ));
  }

  @override
  Future<void> play() => _player.play();

  @override
  Future<void> pause() => _player.pause();

  @override
  Future<void> stop() async {
    await _player.stop();
    await super.stop();
  }

  @override
  Future<void> seek(Duration position) => _player.seek(position);

  @override
  Future<void> skipToNext() async {
    // Will be handled by PlayerController
  }

  @override
  Future<void> skipToPrevious() async {
    // Will be handled by PlayerController
  }

  Future<void> setMediaItem({
    required String id,
    required String title,
    String? artist,
    String? artUri,
    Duration? duration,
  }) async {
    mediaItem.add(MediaItem(
      id: id,
      title: title,
      artist: artist ?? 'Неизвестный исполнитель',
      artUri: artUri != null ? Uri.parse(artUri) : null,
      duration: duration,
    ));
  }
}
```

#### Шаг 3: Обновить `mobile/lib/main.dart`
Инициализировать `AudioService` в `main()`:
```dart
import 'package:audio_service/audio_service.dart';
import 'services/music_audio_handler.dart';

late MusicAudioHandler audioHandler;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  
  audioHandler = await AudioService.init(
    builder: () => MusicAudioHandler(AudioPlayer()),
    config: const AudioServiceConfig(
      androidNotificationChannelId: 'com.music-orchestrator.channel.audio',
      androidNotificationChannelName: 'Music Orchestrator',
      androidNotificationOngoing: true,
      androidStopForegroundOnPause: true,
    ),
  );
  
  final api = ApiClient(baseUrl: _apiBaseUrl, apiKey: _apiKey);
  await api.loadPersistedSettings();
  // ... rest of init
}
```

#### Шаг 4: Обновить `mobile/lib/state/player_controller.dart`
В конструкторе `PlayerController`:
- Получить `MusicAudioHandler` из `main.dart`
- В `_resolveAndPlay()` после `_audio.setUrl()` вызвать `audioHandler.setMediaItem(...)`
- Обработать `skipToNext` / `skipToPrevious` из `audioHandler`

В `_resolveAndPlay()` после строки `await _audio.setUrl(streamURL!);`:
```dart
// Set media session metadata for iOS/Android lock screen
audioHandler.setMediaItem(
  id: track.id,
  title: track.title,
  artist: track.artist,
  artUri: track.artworkUrl != null 
    ? _api.artworkUrl(track.artworkUrl)
    : null,
  duration: duration,
);
```

В конструкторе подписать `audioHandler` на кнопки:
```dart
// Handle media button events from lock screen / notification
audioHandler.skipToNextCallback = () => next();
audioHandler.skipToPreviousCallback = () => previous();
```

Для этого в `MusicAudioHandler` добавить:
```dart
VoidCallback? skipToNextCallback;
VoidCallback? skipToPreviousCallback;

@override
Future<void> skipToNext() async => skipToNextCallback?.call();

@override
Future<void> skipToPrevious() async => skipToPreviousCallback?.call();
```

#### Шаг 5: iOS Info.plist
В `mobile/ios/Runner/Info.plist` добавить:
```xml
<key>UIBackgroundModes</key>
<array>
  <string>audio</string>
</array>
```

Это уже должно быть настроено для `just_audio`, но проверить.

#### Шаг 6: Android манифест
В `mobile/android/app/src/main/AndroidManifest.xml` в `<manifest>`:
```xml
<uses-permission android:name="android.permission.WAKE_LOCK"/>
<uses-permission android:name="android.permission.FOREGROUND_SERVICE"/>
<uses-permission android:name="android.permission.FOREGROUND_SERVICE_MEDIA_PLAYBACK"/>
```

В `<application>`:
```xml
<service
    android:name="com.ryanheise.audioservice.AudioService"
    android:foregroundServiceType="mediaPlayback"
    android:exported="true">
    <intent-filter>
        <action android:name="android.media.browse.MediaBrowserService" />
    </intent-filter>
</service>
```

### Ожидаемый результат
- На iOS lock screen: название трека, имя автора, обложка
- Кнопки: prev track, play/pause, next track
- На Android: notification с тем же набором
- Control Center / Dynamic Island на iOS показывает правильную информацию

---

## Задача 2: SoundCloud

### Текущее состояние
Backend SoundCloud работает:
- Поиск: `curl localhost:18080/v1/search?q=lofi&providers=soundcloud_stream` → `total=4, items=3`
- Playback: `curl localhost:18080/v1/playback/<id>` → `playback_type=extractor_stream`
- Stream: `curl -H "Range: bytes=0-1023" localhost:18080/v1/stream/<id>` → `HTTP 206, audio/mpeg`

### Проблема
Пользователь говорит "SoundCloud не работает". Возможные причины:
1. Flutter app не видит SoundCloud в провайдерах (CORS, API URL)
2. Треки DRM-protected (30s preview) — для Drake, major labels
3. Фронтенд не воспроизводит HLS потоки

### Что проверить
1. Flutter app: в Settings → Providers, SoundCloud отображается как enabled?
2. При поиске — запрос уходит на `/v1/search?providers=soundcloud_stream`?
3. При воспроизведении — URL stream корректный?

### Примечание
yt-dlp `--js-runtimes node` уже добавлен в backend (конфиг `APP_YTDLP_JS_RUNTIMES`, по умолчанию `node`). Некоторые SoundCloud треки DRM-protected — это ограничение SoundCloud, не баг.

---

## Порядок исправления
1. **Задача 1 (MediaSession)** — основная, делаем через `audio_service`
2. **Задача 2 (SoundCloud)** — проверить после задачи 1

## Проверки
```bash
cd mobile
flutter pub get
flutter test
flutter build web --release

# iOS (нужен Mac с Xcode)
flutter build ios

# Android
flutter build apk
```

## Важные файлы
- `mobile/lib/state/player_controller.dart` — PlayerController, воспроизведение
- `mobile/lib/main.dart` — инициализация приложения
- `mobile/pubspec.yaml` — зависимости
- `mobile/ios/Runner/Info.plist` — iOS конфиг
- `mobile/android/app/src/main/AndroidManifest.xml` — Android конфиг
