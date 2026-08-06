import 'package:audio_service/audio_service.dart';
import 'package:audio_session/audio_session.dart';
import 'package:just_audio/just_audio.dart';

/// Bridges the app's single [AudioPlayer] to the platform media session, so the
/// iOS lock screen / Control Center and the Android notification show the real
/// track (title, artist, artwork) with prev/play/next instead of the app name
/// and ±30s buttons. just_audio alone never fills MPNowPlayingInfoCenter.
///
/// The handler does not own the queue: skip requests are forwarded to
/// [PlayerController] through [skipToNextCallback] / [skipToPreviousCallback],
/// which already knows about shuffle and repeat.
class MusicAudioHandler extends BaseAudioHandler with QueueHandler, SeekHandler {
  MusicAudioHandler(this._player) {
    _player.playbackEventStream.listen(_broadcastState);
    _player.processingStateStream.listen((state) {
      if (state == ProcessingState.completed) {
        playbackState.add(
          playbackState.value.copyWith(
            processingState: AudioProcessingState.completed,
          ),
        );
      }
    });
  }

  final AudioPlayer _player;

  /// Set by PlayerController — lock-screen skips must go through the queue.
  Future<void> Function()? skipToNextCallback;
  Future<void> Function()? skipToPreviousCallback;

  void _broadcastState(PlaybackEvent event) {
    playbackState.add(
      playbackState.value.copyWith(
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
      ),
    );
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
  Future<void> skipToNext() async => skipToNextCallback?.call();

  @override
  Future<void> skipToPrevious() async => skipToPreviousCallback?.call();

  /// Publishes what the system player should show for the current track.
  void setMediaItem({
    required String id,
    required String title,
    String? artist,
    String? artUri,
    Duration? duration,
  }) {
    mediaItem.add(
      MediaItem(
        id: id,
        title: title,
        artist: (artist == null || artist.isEmpty)
            ? 'Неизвестный исполнитель'
            : artist,
        artUri: (artUri == null || artUri.isEmpty)
            ? null
            : Uri.tryParse(artUri),
        duration: (duration == null || duration == Duration.zero)
            ? null
            : duration,
      ),
    );
  }
}

/// Starts the platform media session around [player].
///
/// Returns null when the platform has no media session we can attach to (the
/// web build, or a test binding without the plugin): playback still works,
/// only the system-level metadata is missing, so the app must not fail to boot.
Future<MusicAudioHandler?> initMusicAudioHandler(AudioPlayer player) async {
  try {
    // Music category: ducks nothing, keeps playing when the ringer is silent
    // and lets iOS hand the session to the Now Playing UI.
    await (await AudioSession.instance).configure(
      const AudioSessionConfiguration.music(),
    );
    return await AudioService.init(
      builder: () => MusicAudioHandler(player),
      config: const AudioServiceConfig(
        androidNotificationChannelId: 'com.musicorchestrator.channel.audio',
        androidNotificationChannelName: 'Music Orchestrator',
        androidNotificationOngoing: true,
        androidStopForegroundOnPause: true,
      ),
    );
  } catch (_) {
    return null;
  }
}
