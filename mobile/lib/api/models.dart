// Mirrors frontend/frontend/src/api/types.ts — keep both in sync with the Go backend's openapi.json.

class Policy {
  final bool downloadAllowed;
  Policy({required this.downloadAllowed});
  factory Policy.fromJson(Map<String, dynamic> j) =>
      Policy(downloadAllowed: j['download_allowed'] == true);
  Map<String, dynamic> toJson() => {'download_allowed': downloadAllowed};
}

class Track {
  final String id;
  final String providerId;
  final String title;
  final String? artist;
  final int? durationSeconds;
  final String? artworkUrl;
  final bool downloaded;
  final String? downloadMediaUrl;
  final DateTime? libraryAddedAt;
  final Policy policy;

  Track({
    required this.id,
    required this.providerId,
    required this.title,
    this.artist,
    this.durationSeconds,
    this.artworkUrl,
    this.downloaded = false,
    this.downloadMediaUrl,
    this.libraryAddedAt,
    required this.policy,
  });

  factory Track.fromJson(Map<String, dynamic> j) => Track(
    id: j['id'] as String,
    providerId: j['provider_id'] as String,
    title: j['title'] as String,
    artist: j['artist'] as String?,
    durationSeconds: j['duration_seconds'] as int?,
    artworkUrl: j['artwork_url'] as String?,
    downloaded: j['downloaded'] == true,
    downloadMediaUrl: j['download_media_url'] as String?,
    libraryAddedAt: DateTime.tryParse(j['library_added_at'] as String? ?? ''),
    policy: Policy.fromJson((j['policy'] as Map<String, dynamic>?) ?? const {}),
  );

  Track copyWith({
    bool? downloaded,
    String? downloadMediaUrl,
    DateTime? libraryAddedAt,
  }) => Track(
    id: id,
    providerId: providerId,
    title: title,
    artist: artist,
    durationSeconds: durationSeconds,
    artworkUrl: artworkUrl,
    downloaded: downloaded ?? this.downloaded,
    downloadMediaUrl: downloadMediaUrl ?? this.downloadMediaUrl,
    libraryAddedAt: libraryAddedAt ?? this.libraryAddedAt,
    policy: policy,
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'provider_id': providerId,
    'title': title,
    if (artist != null) 'artist': artist,
    if (durationSeconds != null) 'duration_seconds': durationSeconds,
    if (artworkUrl != null) 'artwork_url': artworkUrl,
    'downloaded': downloaded,
    if (downloadMediaUrl != null) 'download_media_url': downloadMediaUrl,
    if (libraryAddedAt != null)
      'library_added_at': libraryAddedAt!.toIso8601String(),
    'policy': policy.toJson(),
  };
}

class MusicProvider {
  final String id;
  final String name;
  final bool enabled;
  final String riskLevel;
  final bool canSearch;
  MusicProvider({
    required this.id,
    required this.name,
    required this.enabled,
    required this.riskLevel,
    required this.canSearch,
  });
  factory MusicProvider.fromJson(Map<String, dynamic> j) => MusicProvider(
    id: j['id'] as String,
    name: j['name'] as String,
    enabled: j['enabled'] == true,
    riskLevel: j['risk_level'] as String? ?? 'safe',
    canSearch: (j['capabilities']?['search']) == true,
  );
}

class SearchResult {
  final String query;
  final int total;
  final List<Track> items;
  SearchResult({required this.query, required this.total, required this.items});
  factory SearchResult.fromJson(Map<String, dynamic> j) => SearchResult(
    query: j['query'] as String? ?? '',
    total: j['total'] as int? ?? 0,
    items: ((j['items'] as List?) ?? [])
        .map((e) => Track.fromJson(e as Map<String, dynamic>))
        .toList(),
  );
}

class Playback {
  final String? streamUrl;
  final String playbackType;
  Playback({this.streamUrl, required this.playbackType});
  factory Playback.fromJson(Map<String, dynamic> j) => Playback(
    streamUrl: j['stream_url'] as String?,
    playbackType: j['playback_type'] as String? ?? 'unavailable',
  );
}

class Playlist {
  final String id;
  final String name;
  final String? description;
  final String? coverUrl;
  final int trackCount;
  final int durationSeconds;
  final List<PlaylistTrack> tracks;
  Playlist({
    required this.id,
    required this.name,
    this.description,
    this.coverUrl,
    required this.trackCount,
    required this.durationSeconds,
    this.tracks = const [],
  });
  factory Playlist.fromJson(Map<String, dynamic> j) => Playlist(
    id: j['id'] as String,
    name: j['name'] as String,
    description: j['description'] as String?,
    coverUrl: j['cover_url'] as String?,
    trackCount: j['track_count'] as int? ?? 0,
    durationSeconds: j['duration_seconds'] as int? ?? 0,
    tracks: ((j['tracks'] as List?) ?? const [])
        .map((e) => PlaylistTrack.fromJson(e as Map<String, dynamic>))
        .toList(),
  );
}

class PlaylistTrack {
  final String id;
  final String trackId;
  final Track? track;
  final int position;

  PlaylistTrack({
    required this.id,
    required this.trackId,
    this.track,
    required this.position,
  });

  factory PlaylistTrack.fromJson(Map<String, dynamic> j) => PlaylistTrack(
    id: j['id'] as String? ?? '',
    trackId: j['track_id'] as String,
    track: j['track'] is Map<String, dynamic>
        ? Track.fromJson(j['track'] as Map<String, dynamic>)
        : null,
    position: j['position'] as int? ?? 0,
  );
}

class SessionInfo {
  final bool authenticated;
  final String? username;
  final String? role;
  final bool setupRequired;
  final bool loginEnabled;
  SessionInfo({
    required this.authenticated,
    this.username,
    this.role,
    required this.setupRequired,
    required this.loginEnabled,
  });
  factory SessionInfo.fromJson(Map<String, dynamic> j) => SessionInfo(
    authenticated: j['authenticated'] == true,
    username: j['username'] as String?,
    role: j['role'] as String?,
    setupRequired: j['setup_required'] == true,
    loginEnabled: j['login_enabled'] != false,
  );
  bool get isAdmin => role == 'admin';
}

class AppUser {
  final String id;
  final String username;
  AppUser({required this.id, required this.username});
  factory AppUser.fromJson(Map<String, dynamic> j) =>
      AppUser(id: j['id'] as String, username: j['username'] as String);
}
