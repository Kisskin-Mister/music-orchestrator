import 'package:flutter_test/flutter_test.dart';
import 'package:music_orchestrator/api/models.dart';

void main() {
  const trackJson = <String, dynamic>{
    'id': 'youtube_stream:abc',
    'provider_id': 'youtube_stream',
    'title': 'Track',
    'artist': 'Artist',
    'artwork_url': 'https://i.ytimg.com/vi/abc/hqdefault.jpg',
    'duration_seconds': 123,
    'downloaded': false,
    'policy': {'download_allowed': true},
  };

  test('recent track JSON round-trips source and artwork', () {
    final addedAt = DateTime.utc(2026, 8, 4, 10, 30);
    final track = Track.fromJson(trackJson).copyWith(libraryAddedAt: addedAt);
    final restored = Track.fromJson(track.toJson());

    expect(restored.id, track.id);
    expect(restored.providerId, 'youtube_stream');
    expect(restored.artworkUrl, track.artworkUrl);
    expect(restored.libraryAddedAt, addedAt);
    expect(restored.policy.downloadAllowed, isTrue);
  });

  test('playlist parses automatic cover and playable tracks', () {
    final playlist = Playlist.fromJson({
      'id': 'pl_1',
      'name': 'Mix',
      'cover_url': trackJson['artwork_url'],
      'track_count': 1,
      'duration_seconds': 123,
      'tracks': [
        {
          'id': 'pli_1',
          'track_id': trackJson['id'],
          'position': 0,
          'track': trackJson,
        },
      ],
    });

    expect(playlist.coverUrl, trackJson['artwork_url']);
    expect(playlist.tracks.single.track?.providerId, 'youtube_stream');
    expect(playlist.tracks.single.track?.title, 'Track');
  });
}
