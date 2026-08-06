import 'package:flutter/foundation.dart';

/// Stub for non-web platforms. Real implementation is in web_media_session.dart
/// which is conditionally imported when dart.library.js_interop is available.
class WebMediaSession {
  static bool get isSupported => false;
  static void setMetadata({required String title, String? artist, String? artworkUrl, Duration? duration}) {}
  static void setActions({VoidCallback? onPlay, VoidCallback? onPause, VoidCallback? onPrev, VoidCallback? onNext}) {}
  static void setPlaybackState({required bool playing, Duration? position, Duration? duration}) {}
}
