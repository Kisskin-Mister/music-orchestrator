import 'dart:convert';
import 'dart:typed_data';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'models.dart';

const _baseUrlPrefsKey = 'mo_backend_url';
const _apiKeyPrefsKey = 'mo_api_key';
const _sessionCookiePrefsKey = 'mo_session_cookie';
const _sessionCookieName =
    'mo_session'; // must match sessionCookieName in auth.go

/// Mirrors frontend/frontend/src/api/client.ts. Talks to the same Go backend
/// (main.go) — no separate mobile backend, one API contract for both clients.
///
/// Auth note: the backend checks X-API-Key *before* the session cookie
/// (see currentIdentity() in auth.go) and an API-key identity is never an
/// "admin" account, so /v1/account and /v1/users always 403 for it. Only send
/// X-API-Key when the caller actually wants that service-token shortcut for
/// resource endpoints (favorites/playlists/downloads); leave it empty to rely
/// on real session login for account/user management, same as the web app.
class ApiClient {
  ApiClient({required String baseUrl, String apiKey = ''})
    : _baseUrl = baseUrl,
      _apiKey = apiKey;

  String _baseUrl;
  String _apiKey;
  String? _sessionCookie;

  String get baseUrl => _baseUrl;
  String get apiKey => _apiKey;

  /// True when a session cookie was persisted by an earlier successful login.
  /// Lets the app open offline instead of stranding the user on the login
  /// screen when the server is unreachable but downloads exist on the device.
  bool get hasStoredSession => _sessionCookie != null && _sessionCookie!.isNotEmpty;

  Future<void> loadPersistedSettings() async {
    final prefs = await SharedPreferences.getInstance();
    _baseUrl = prefs.getString(_baseUrlPrefsKey) ?? _baseUrl;
    _apiKey = prefs.getString(_apiKeyPrefsKey) ?? _apiKey;
    _sessionCookie = prefs.getString(_sessionCookiePrefsKey);
  }

  Future<void> updateConnection({
    required String baseUrl,
    required String apiKey,
  }) async {
    _baseUrl = baseUrl.trim().replaceAll(RegExp(r'/+$'), '');
    _apiKey = apiKey.trim();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_baseUrlPrefsKey, _baseUrl);
    await prefs.setString(_apiKeyPrefsKey, _apiKey);
  }

  Future<void> _storeSessionCookie(http.Response res) async {
    final setCookie = res.headers['set-cookie'];
    if (setCookie == null) return;
    final match = RegExp('$_sessionCookieName=([^;]*)').firstMatch(setCookie);
    if (match == null) return;
    _sessionCookie = match.group(1);
    final prefs = await SharedPreferences.getInstance();
    if (_sessionCookie == null || _sessionCookie!.isEmpty) {
      await prefs.remove(_sessionCookiePrefsKey);
    } else {
      await prefs.setString(_sessionCookiePrefsKey, _sessionCookie!);
    }
  }

  Map<String, String> get _headers => {
    'Accept': 'application/json',
    if (_apiKey.isNotEmpty) 'X-API-Key': _apiKey,
    if (_sessionCookie != null && _sessionCookie!.isNotEmpty)
      'Cookie': '$_sessionCookieName=$_sessionCookie',
  };

  Uri _uri(String path, [Map<String, String>? query]) =>
      Uri.parse('$_baseUrl$path').replace(queryParameters: query);

  String? resolveUrl(String? raw) {
    if (raw == null || raw.isEmpty) return null;
    final parsed = Uri.tryParse(raw);
    if (parsed != null && parsed.hasScheme) return raw;
    return '$_baseUrl${raw.startsWith('/') ? raw : '/$raw'}';
  }

  /// Route cover art through the backend instead of hitting the CDN directly:
  /// the device may not reach that CDN (restricted network / no shared proxy),
  /// and this keeps the user's IP off third-party hosts. See artwork.go.
  String? artworkUrl(String? raw) {
    if (raw == null || raw.isEmpty) return null;
    if (raw.startsWith('/')) return resolveUrl(raw);
    return '$_baseUrl/v1/artwork?url=${Uri.encodeComponent(raw)}';
  }

  Future<Map<String, dynamic>> _getJson(
    String path, {
    Duration timeout = const Duration(seconds: 12),
  }) async {
    final res = await http.get(_uri(path), headers: _headers).timeout(timeout);
    _throwIfError(res);
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<T> _send<T>(
    Future<http.Response> Function() request,
    T Function(http.Response) onSuccess,
  ) async {
    final res = await request().timeout(const Duration(seconds: 12));
    _throwIfError(res);
    await _storeSessionCookie(res);
    return onSuccess(res);
  }

  void _throwIfError(http.Response res) {
    if (res.statusCode >= 200 && res.statusCode < 300) return;
    String message = 'HTTP ${res.statusCode}';
    try {
      final body = jsonDecode(res.body) as Map<String, dynamic>;
      message = (body['error']?['message'] as String?) ?? message;
    } catch (_) {}
    throw ApiException(message, res.statusCode);
  }

  // --- Auth -----------------------------------------------------------

  Future<SessionInfo> session() async =>
      SessionInfo.fromJson(await _getJson('/v1/auth/session'));

  Future<SessionInfo> register(String username, String password) => _send(
    () => http.post(
      _uri('/v1/auth/register'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    ),
    (res) => SessionInfo.fromJson(jsonDecode(res.body) as Map<String, dynamic>),
  );

  Future<SessionInfo> login(String username, String password) => _send(
    () => http.post(
      _uri('/v1/auth/login'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    ),
    (res) => SessionInfo.fromJson(jsonDecode(res.body) as Map<String, dynamic>),
  );

  Future<void> logout() async {
    await http
        .post(_uri('/v1/auth/logout'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    _sessionCookie = null;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_sessionCookiePrefsKey);
  }

  Future<void> updateAccount({required String username, String? password}) =>
      _send(
        () => http.patch(
          _uri('/v1/account'),
          headers: {..._headers, 'Content-Type': 'application/json'},
          body: jsonEncode({
            'username': username,
            if (password != null && password.isNotEmpty) 'password': password,
          }),
        ),
        (res) {},
      );

  Future<List<AppUser>> users() async {
    final res = await http
        .get(_uri('/v1/users'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    _throwIfError(res);
    final list = jsonDecode(res.body) as List;
    return list
        .map((e) => AppUser.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> createUser(String username, String password) => _send(
    () => http.post(
      _uri('/v1/users'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    ),
    (res) {},
  );

  Future<void> updateUser(
    String userId, {
    String? username,
    String? password,
  }) => _send(
    () => http.patch(
      _uri('/v1/users/${Uri.encodeComponent(userId)}'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({
        'username': ?username,
        if (password != null && password.isNotEmpty) 'password': password,
      }),
    ),
    (res) {},
  );

  Future<void> deleteUser(String userId) => http
      .delete(
        _uri('/v1/users/${Uri.encodeComponent(userId)}'),
        headers: _headers,
      )
      .timeout(const Duration(seconds: 12));

  // --- Library ----------------------------------------------------------

  Future<List<MusicProvider>> providers() async {
    final res = await http
        .get(_uri('/v1/providers'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    final list = jsonDecode(res.body) as List;
    return list
        .map((e) => MusicProvider.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<SearchResult> search(
    String query, {
    List<String> providerIds = const [],
    int limit = 20,
    int offset = 0,
  }) async {
    final res = await http
        .get(
          _uri('/v1/search', {
            'q': query,
            'limit': '$limit',
            'offset': '$offset',
            if (providerIds.isNotEmpty) 'providers': providerIds.join(','),
          }),
          headers: _headers,
        )
        .timeout(const Duration(seconds: 45));
    _throwIfError(res);
    return SearchResult.fromJson(jsonDecode(res.body) as Map<String, dynamic>);
  }

  Future<Playback> playback(String trackId) async {
    final res = await http
        .get(
          _uri('/v1/playback/${Uri.encodeComponent(trackId)}'),
          headers: _headers,
        )
        .timeout(const Duration(seconds: 12));
    _throwIfError(res);
    final playback = Playback.fromJson(
      jsonDecode(res.body) as Map<String, dynamic>,
    );
    return Playback(
      streamUrl: resolveUrl(playback.streamUrl),
      playbackType: playback.playbackType,
    );
  }

  Future<List<Track>> downloads() async {
    final res = await http
        .get(_uri('/v1/downloads'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    _throwIfError(res);
    final list = jsonDecode(res.body) as List;
    return list
        .map((entry) {
          final job = entry as Map<String, dynamic>;
          final result = job['result'] as Map<String, dynamic>?;
          final payload = job['payload'] as Map<String, dynamic>?;
          final rawTrack = result?['track'] ?? payload?['track'];
          if (rawTrack is! Map<String, dynamic>) return null;
          return Track.fromJson(rawTrack).copyWith(
            downloaded: true,
            downloadMediaUrl: resolveUrl(result?['media_url'] as String?),
            libraryAddedAt: DateTime.tryParse(
              job['updated_at'] as String? ??
                  job['created_at'] as String? ??
                  '',
            ),
          );
        })
        .whereType<Track>()
        .toList();
  }

  Future<List<Track>> favorites() async {
    final res = await http
        .get(_uri('/v1/favorites'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    _throwIfError(res);
    final list = jsonDecode(res.body) as List;
    return list
        .map((raw) {
          final favorite = raw as Map<String, dynamic>;
          final track = favorite['track'];
          if (track is! Map<String, dynamic>) return null;
          return Track.fromJson(track).copyWith(
            libraryAddedAt: DateTime.tryParse(
              favorite['created_at'] as String? ?? '',
            ),
          );
        })
        .whereType<Track>()
        .toList();
  }

  Future<void> addFavorite(Track track) => _send(
    () => http.post(
      _uri('/v1/favorites'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({'track_id': track.id}),
    ),
    (res) {},
  );

  Future<void> removeFavorite(String trackId) => _send(
    () => http.delete(
      _uri('/v1/favorites/${Uri.encodeComponent(trackId)}'),
      headers: _headers,
    ),
    (res) {},
  );

  /// Asks the server to fetch and transcode the track into APP_MEDIA_ROOT.
  /// Returns the resulting `/media/...` path, which is also what a
  /// save-to-device download reads from (see OfflineController).
  Future<String?> createDownload(String trackId, {String format = 'mp3'}) async {
    final res = await http
        .post(
          _uri('/v1/downloads'),
          headers: {..._headers, 'Content-Type': 'application/json'},
          body: jsonEncode({'track_id': trackId, 'format': format}),
        )
        .timeout(const Duration(minutes: 5));
    _throwIfError(res);
    final job = jsonDecode(res.body) as Map<String, dynamic>;
    if (job['status'] != 'succeeded') {
      throw ApiException(
        job['error'] as String? ?? 'Не удалось скачать трек',
        res.statusCode,
      );
    }
    return (job['result'] as Map<String, dynamic>?)?['media_url'] as String?;
  }

  /// Streams a server-side media file so it can be written to device storage.
  /// Returns the byte stream plus the total length when the server sends one.
  Future<({Stream<List<int>> bytes, int? contentLength})> openMedia(String mediaUrl) async {
    final resolved = resolveUrl(mediaUrl)!;
    final request = http.Request('GET', Uri.parse(resolved))..headers.addAll(_headers);
    final response = await http.Client().send(request).timeout(const Duration(minutes: 5));
    if (response.statusCode != 200) {
      throw ApiException('Не удалось получить файл (HTTP ${response.statusCode})', response.statusCode);
    }
    return (bytes: response.stream.cast<List<int>>(), contentLength: response.contentLength);
  }

  Future<List<Playlist>> playlists() async {
    final res = await http
        .get(_uri('/v1/playlists'), headers: _headers)
        .timeout(const Duration(seconds: 12));
    _throwIfError(res);
    final list = jsonDecode(res.body) as List;
    return list
        .map((e) => Playlist.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<Playlist> playlist(String playlistId) async => Playlist.fromJson(
    await _getJson('/v1/playlists/${Uri.encodeComponent(playlistId)}'),
  );

  Future<Playlist> createPlaylist(String name, {String? description}) => _send(
    () => http.post(
      _uri('/v1/playlists'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({
        'name': name,
        if (description != null && description.isNotEmpty)
          'description': description,
      }),
    ),
    (res) => Playlist.fromJson(jsonDecode(res.body) as Map<String, dynamic>),
  );

  Future<void> deletePlaylist(String playlistId) => _send(
    () => http.delete(
      _uri('/v1/playlists/${Uri.encodeComponent(playlistId)}'),
      headers: _headers,
    ),
    (res) {},
  );

  Future<void> removePlaylistTrack(String playlistId, String trackId) => http
      .delete(
        _uri(
          '/v1/playlists/${Uri.encodeComponent(playlistId)}/tracks/${Uri.encodeComponent(trackId)}',
        ),
        headers: _headers,
      )
      .timeout(const Duration(seconds: 12));

  Future<void> addPlaylistTrack(String playlistId, String trackId) => _send(
    () => http.post(
      _uri('/v1/playlists/${Uri.encodeComponent(playlistId)}/tracks'),
      headers: {..._headers, 'Content-Type': 'application/json'},
      body: jsonEncode({'track_id': trackId}),
    ),
    (res) {},
  );

  Future<Playlist> uploadPlaylistCover(
    String playlistId,
    Uint8List bytes,
    String filename,
  ) async {
    final request = http.MultipartRequest(
      'POST',
      _uri('/v1/playlists/${Uri.encodeComponent(playlistId)}/cover'),
    );
    request.headers.addAll(_headers);
    request.files.add(
      http.MultipartFile.fromBytes('cover', bytes, filename: filename),
    );
    final streamed = await request.send().timeout(const Duration(seconds: 45));
    final res = await http.Response.fromStream(streamed);
    _throwIfError(res);
    return Playlist.fromJson(jsonDecode(res.body) as Map<String, dynamic>);
  }
}

class ApiException implements Exception {
  ApiException(this.message, this.statusCode);
  final String message;
  final int statusCode;
  @override
  String toString() => message;
}
