import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/foundation.dart';
import 'package:just_audio/just_audio.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../api/api_client.dart';
import '../api/models.dart';
import 'offline_controller.dart';

enum PlayerRepeatMode { off, all, one }

/// Mirrors src/store/player.ts (queue, current track, play/pause) plus the
/// stream-resolving part of PlayerBar in SearchPage.tsx, backed by just_audio
/// instead of an <audio> element.
class PlayerController extends ChangeNotifier {
  PlayerController(this._api) {
    // just_audio flips play/pause and buffering state asynchronously, so the UI
    // must follow the player's own stream — not only our post-await guesses,
    // otherwise the mini-player keeps showing "play" while audio is running.
    _stateSub = _audio.playerStateStream.listen((state) {
      isBuffering =
          state.processingState == ProcessingState.loading ||
          state.processingState == ProcessingState.buffering;
      if (state.playing) error = null;
      notifyListeners();
    });
    _completedSub = _audio.processingStateStream.listen((state) {
      if (state == ProcessingState.completed) unawaited(_handleCompletion());
    });
    _eventSub = _audio.playbackEventStream.listen(
      (_) {},
      onError: (Object e, StackTrace _) {
        isBuffering = false;
        error = _playbackError(e);
        notifyListeners();
      },
    );
  }
  final ApiClient _api;
  final AudioPlayer _audio = AudioPlayer();

  /// Set once at startup (see main.dart). Lets playback prefer a device-local
  /// file without the player owning the offline index itself.
  OfflineController? offline;

  StreamSubscription<PlayerState>? _stateSub;
  StreamSubscription<ProcessingState>? _completedSub;
  StreamSubscription<PlaybackEvent>? _eventSub;

  List<Track> queue = [];
  int queueIndex = -1;

  /// Inserts right after the current track. With an empty queue this is the
  /// same as starting playback, so the action never silently does nothing.
  void playNext(Track track) {
    if (queueIndex < 0 || queue.isEmpty) {
      playFrom([track], 0);
      return;
    }
    final next = List<Track>.of(queue)..removeWhere((t) => t.id == track.id);
    // Removing a duplicate that sat before the cursor shifts the cursor left.
    final cursor = queue.indexWhere((t) => t.id == currentTrack?.id);
    final at = (cursor < 0 ? queueIndex : cursor) + 1;
    next.insert(at.clamp(0, next.length), track);
    queue = next;
    queueIndex = queue.indexWhere((t) => t.id == currentTrack?.id);
    notifyListeners();
  }

  void addToQueue(Track track) {
    if (queueIndex < 0 || queue.isEmpty) {
      playFrom([track], 0);
      return;
    }
    if (queue.any((t) => t.id == track.id)) return;
    queue = [...queue, track];
    notifyListeners();
  }
  bool isBuffering = false;
  String? error;
  List<Track> recentTracks = [];
  bool shuffleEnabled = false;
  PlayerRepeatMode repeatMode = PlayerRepeatMode.off;
  final Random _random = Random();

  Track? get currentTrack =>
      queueIndex >= 0 && queueIndex < queue.length ? queue[queueIndex] : null;
  bool get isPlaying => _audio.playing;
  Stream<Duration> get positionStream => _audio.positionStream;

  /// YouTube CDN reports Content-Length for the full video+audio container,
  /// so just_audio reports roughly 2x real duration. When the audio element
  /// claims far more than the provider metadata, clamp to provider value.
  static const _overreportedRatio = 1.3;
  Duration get duration {
    final raw = _audio.duration ?? Duration.zero;
    final track = currentTrack;
    if (track == null || track.durationSeconds <= 0) return raw;
    final providerMs = track.durationSeconds * 1000;
    if (raw.inMilliseconds > providerMs * _overreportedRatio) {
      return Duration(milliseconds: providerMs);
    }
    return raw;
  }

  Future<void> loadHistory() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_recentTracksPrefsKey);
    if (raw == null || raw.isEmpty) return;
    try {
      final decoded = jsonDecode(raw) as List;
      recentTracks = decoded
          .whereType<Map<String, dynamic>>()
          .map(Track.fromJson)
          .where((track) => track.providerId != 'local')
          .take(20)
          .toList();
      notifyListeners();
    } catch (_) {
      await prefs.remove(_recentTracksPrefsKey);
    }
  }

  Future<void> playFrom(List<Track> list, int index) async {
    if (list.isEmpty || index < 0 || index >= list.length) return;
    queue = List<Track>.of(list);
    queueIndex = index;
    notifyListeners();
    await _resolveAndPlay();
  }

  Future<void> togglePlayPause([Track? track]) async {
    if (track != null && track.id != currentTrack?.id) {
      final index = queue.indexWhere((t) => t.id == track.id);
      await playFrom(index >= 0 ? queue : [track], index >= 0 ? index : 0);
      return;
    }
    if (_audio.playing) {
      await _audio.pause();
    } else if (_audio.processingState == ProcessingState.idle ||
        error != null) {
      await _resolveAndPlay();
    } else {
      await _audio.play();
    }
    notifyListeners();
  }

  Future<void> next() async {
    final index = _nextIndex();
    if (index == null) return;
    queueIndex = index;
    notifyListeners();
    await _resolveAndPlay();
  }

  Future<void> previous() async {
    if (_audio.position > const Duration(seconds: 3)) {
      await _audio.seek(Duration.zero);
      return;
    }
    final index = _previousIndex();
    if (index == null) return;
    queueIndex = index;
    notifyListeners();
    await _resolveAndPlay();
  }

  void toggleShuffle() {
    shuffleEnabled = !shuffleEnabled;
    notifyListeners();
  }

  void cycleRepeatMode() {
    repeatMode = switch (repeatMode) {
      PlayerRepeatMode.off => PlayerRepeatMode.all,
      PlayerRepeatMode.all => PlayerRepeatMode.one,
      PlayerRepeatMode.one => PlayerRepeatMode.off,
    };
    notifyListeners();
  }

  Future<void> seek(Duration position) => _audio.seek(position);

  int? _nextIndex() {
    if (queue.isEmpty || queueIndex < 0) return null;
    if (shuffleEnabled && queue.length > 1) {
      var next = queueIndex;
      while (next == queueIndex) {
        next = _random.nextInt(queue.length);
      }
      return next;
    }
    if (queueIndex + 1 < queue.length) return queueIndex + 1;
    if (repeatMode == PlayerRepeatMode.all && queue.isNotEmpty) return 0;
    return null;
  }

  int? _previousIndex() {
    if (queue.isEmpty || queueIndex < 0) return null;
    if (shuffleEnabled && queue.length > 1) {
      var previous = queueIndex;
      while (previous == queueIndex) {
        previous = _random.nextInt(queue.length);
      }
      return previous;
    }
    if (queueIndex > 0) return queueIndex - 1;
    if (repeatMode == PlayerRepeatMode.all && queue.isNotEmpty) {
      return queue.length - 1;
    }
    return null;
  }

  Future<void> _handleCompletion() async {
    if (repeatMode == PlayerRepeatMode.one) {
      await _audio.seek(Duration.zero);
      unawaited(_audio.play());
      return;
    }
    await next();
  }

  Future<void> _resolveAndPlay() async {
    final track = currentTrack;
    if (track == null) return;
    isBuffering = true;
    error = null;
    notifyListeners();
    try {
      // Playback source priority: a file on this device (works with no network
      // at all) > the server's transcoded copy > a fresh provider stream.
      final devicePath = offline?.localPathFor(track.id);
      final localURL = devicePath == null ? _api.resolveUrl(track.downloadMediaUrl) : null;
      final playback = (devicePath == null && localURL == null) ? await _api.playback(track.id) : null;
      final streamURL = localURL ?? playback?.streamUrl;
      if (devicePath == null && streamURL == null) {
        error = 'Источник недоступен для этого трека';
      } else {
        if (devicePath != null) {
          await _audio.setFilePath(devicePath);
        } else {
          await _audio.setUrl(streamURL!);
        }
        await _remember(track);
        // Not awaited: play() only completes when playback *finishes*, so
        // awaiting it here would block the whole resolve path until the end
        // of the track. playerStateStream drives the UI from here on.
        unawaited(_audio.play());
      }
    } catch (e) {
      error = _playbackError(e);
    } finally {
      isBuffering = false;
      notifyListeners();
    }
  }

  String _playbackError(Object error) {
    if (error is PlayerException) {
      return 'Не удалось воспроизвести аудио (${error.code}). Попробуй другой источник.';
    }
    return 'Не удалось получить поток: $error';
  }

  Future<void> _remember(Track track) async {
    if (track.providerId == 'local') return;
    recentTracks = [
      track,
      ...recentTracks.where((item) => item.id != track.id),
    ].take(20).toList();
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(
      _recentTracksPrefsKey,
      jsonEncode(recentTracks.map((item) => item.toJson()).toList()),
    );
  }

  @override
  void dispose() {
    _stateSub?.cancel();
    _completedSub?.cancel();
    _eventSub?.cancel();
    _audio.dispose();
    super.dispose();
  }
}

const _recentTracksPrefsKey = 'mo_recent_tracks';
