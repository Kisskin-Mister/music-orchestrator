import 'dart:js_util' as js_util;
import 'package:flutter/foundation.dart';

/// Browser MediaSession API bridge for Flutter web.
///
/// On native platforms (iOS/Android) `audio_service` handles the lock screen /
/// notification.  On the web the browser's own MediaSession API does the same
/// job, but `audio_service` returns null — so this file fills the gap.
class WebMediaSession {
  static bool get isSupported {
    if (!kIsWeb) return false;
    try {
      return _mediaSession() != null;
    } catch (_) {
      return false;
    }
  }

  static void setMetadata({
    required String title,
    String? artist,
    String? artworkUrl,
    Duration? duration,
  }) {
    if (!isSupported) return;
    try {
      final artwork = <Map<String, String>>[];
      if (artworkUrl != null && artworkUrl.isNotEmpty) {
        artwork.add({'src': artworkUrl, 'sizes': '512x512', 'type': 'image/jpeg'});
      }
      final metadata = js_util.callConstructor(
        _MediaMetadataCtor,
        [js_util.jsify({'title': title, 'artist': artist ?? '', 'album': 'Music Orchestrator', 'artwork': artwork})],
      );
      js_util.setProperty(_mediaSession()!, 'metadata', metadata);
    } catch (_) {}
  }

  static void setActions({
    VoidCallback? onPlay,
    VoidCallback? onPause,
    VoidCallback? onPrev,
    VoidCallback? onNext,
  }) {
    if (!isSupported) return;
    final ms = _mediaSession()!;
    _bind(ms, 'play', onPlay);
    _bind(ms, 'pause', onPause);
    _bind(ms, 'previoustrack', onPrev);
    _bind(ms, 'nexttrack', onNext);
  }

  static void setPlaybackState({required bool playing, Duration? position, Duration? duration}) {
    if (!isSupported) return;
    try {
      // MediaSession.playbackState is the main signal for browser chrome.
      js_util.setProperty(_mediaSession()!, 'playbackState', playing ? 'playing' : 'paused');
    } catch (_) {}
  }
}

// JS helpers

dynamic _mediaSession() {
  final nav = js_util.getProperty(js_util.globalThis, 'navigator');
  return js_util.getProperty(nav, 'mediaSession');
}

// MediaMetadata constructor — available in browsers that support MediaSession.
final _MediaMetadataCtor = js_util.getProperty(js_util.globalThis, 'MediaMetadata');

void _bind(dynamic ms, String action, VoidCallback? fn) {
  try {
    js_util.callMethod(ms, 'setActionHandler', [action, fn == null ? null : js_util.allowInterop(fn)]);
  } catch (_) {}
}
