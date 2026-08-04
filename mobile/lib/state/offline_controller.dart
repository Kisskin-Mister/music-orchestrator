import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../api/api_client.dart';
import '../api/models.dart';
import '../services/offline_store.dart';

const _indexPrefsKey = 'mo_offline_index';

/// Where a track can be played from, in the order the player prefers.
enum OfflineState {
  /// Nowhere yet — streaming only, needs the network every time.
  none,

  /// Server has a transcoded copy (APP_MEDIA_ROOT). Still needs the server
  /// to be reachable, but no longer depends on YouTube/SoundCloud.
  onServer,

  /// A copy exists in this device's app storage — plays with no network.
  onDevice,
}

/// Owns the "save to device" half of downloads.
///
/// The two download destinations are a pipeline, not two independent options:
/// a device copy is fetched from the server's `/media/...` file, so a track
/// with no server copy is downloaded to the server first and then pulled down.
/// That ordering is why the UI presents them as one escalating action rather
/// than two equal buttons.
class OfflineController extends ChangeNotifier {
  OfflineController(this._api) {
    _load();
  }
  final ApiClient _api;
  final OfflineStore _store = OfflineStore();

  /// trackId -> local absolute file path.
  Map<String, String> _paths = {};

  /// trackId -> track metadata, stored alongside the file so the downloads
  /// list can render titles, artists and artwork with no server reachable.
  Map<String, Track> _tracks = {};
  final Map<String, double> _progress = {};
  final Set<String> _busy = {};
  String? error;

  static bool get isSupported => OfflineStore.isSupported;

  bool isOnDevice(String trackId) => _paths.containsKey(trackId);
  bool isBusy(String trackId) => _busy.contains(trackId);

  /// null while the size is unknown (server sent no Content-Length).
  double? progressFor(String trackId) => _progress[trackId];

  String? localPathFor(String trackId) => _paths[trackId];

  /// Tracks saved on this device, newest first. Served entirely from the
  /// local index, so this list is identical online and offline.
  List<Track> get deviceTracks {
    final tracks = _paths.keys.map((id) => _tracks[id]).whereType<Track>().toList();
    tracks.sort((a, b) {
      final aAt = a.libraryAddedAt, bAt = b.libraryAddedAt;
      if (aAt == null && bAt == null) return 0;
      if (aAt == null) return 1;
      if (bAt == null) return -1;
      return bAt.compareTo(aAt);
    });
    return tracks;
  }

  /// Fills in metadata for files that were saved before the index stored it
  /// (or whose metadata was lost), using tracks the library already loaded.
  /// Without this a migrated download shows as "0 tracks" while still taking
  /// up disk space.
  void adoptMetadata(Iterable<Track> tracks) {
    var changed = false;
    for (final track in tracks) {
      if (!_paths.containsKey(track.id) || _tracks.containsKey(track.id)) continue;
      _tracks = {..._tracks, track.id: track};
      changed = true;
    }
    if (!changed) return;
    _persist();
    notifyListeners();
  }

  OfflineState stateFor(Track track) {
    if (isOnDevice(track.id)) return OfflineState.onDevice;
    if (track.downloaded || (track.downloadMediaUrl?.isNotEmpty ?? false)) return OfflineState.onServer;
    return OfflineState.none;
  }

  Future<void> _load() async {
    if (!isSupported) return;
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_indexPrefsKey);
    if (raw == null) return;
    try {
      final decoded = jsonDecode(raw) as Map<String, dynamic>;
      // Re-resolve every path from the file name instead of trusting the stored
      // one: iOS gives the app a new container UUID on reinstall, so an
      // absolute path saved earlier points at a directory that no longer
      // exists. Entries whose file is genuinely gone are dropped, so the UI
      // never claims a track is offline when it is not.
      final verified = <String, String>{};
      final restored = <String, Track>{};
      for (final entry in decoded.entries) {
        final actual = await _store.pathFor(_fileNameFor(entry.key));
        if (actual == null) continue;
        verified[entry.key] = actual;
        // Entries written before metadata was stored are plain path strings.
        final value = entry.value;
        if (value is Map<String, dynamic> && value['track'] is Map<String, dynamic>) {
          restored[entry.key] = Track.fromJson(value['track'] as Map<String, dynamic>);
        }
      }
      final changed = verified.length != decoded.length ||
          verified.entries.any((e) => (decoded[e.key] is Map ? (decoded[e.key] as Map)['path'] : decoded[e.key]) != e.value);
      _paths = verified;
      _tracks = restored;
      if (changed) await _persist();
      notifyListeners();
    } catch (_) {
      // Corrupt index: start clean rather than blocking downloads forever.
      _paths = {};
    }
  }

  Future<void> _persist() async {
    final prefs = await SharedPreferences.getInstance();
    final payload = {
      for (final entry in _paths.entries)
        entry.key: {
          'path': entry.value,
          if (_tracks[entry.key] != null) 'track': _tracks[entry.key]!.toJson(),
        },
    };
    await prefs.setString(_indexPrefsKey, jsonEncode(payload));
  }

  String _fileNameFor(String trackId) => '${trackId.replaceAll(RegExp(r'[^A-Za-z0-9_-]'), '_')}.mp3';

  /// Saves a playable copy onto this device, downloading it to the server
  /// first when needed. [onServerCopyCreated] lets the caller refresh its
  /// track lists when a server copy appeared as a side effect.
  Future<bool> saveToDevice(Track track, {Future<void> Function()? onServerCopyCreated}) async {
    if (!isSupported || _busy.contains(track.id)) return false;
    _busy.add(track.id);
    _progress[track.id] = 0;
    error = null;
    notifyListeners();
    try {
      var mediaUrl = track.downloadMediaUrl;
      if (mediaUrl == null || mediaUrl.isEmpty) {
        mediaUrl = await _api.createDownload(track.id);
        if (onServerCopyCreated != null) await onServerCopyCreated();
      }
      if (mediaUrl == null || mediaUrl.isEmpty) {
        throw Exception('Сервер не вернул путь к файлу');
      }
      final media = await _api.openMedia(mediaUrl);
      var received = 0;
      final total = media.contentLength;
      final counted = media.bytes.map((chunk) {
        received += chunk.length;
        if (total != null && total > 0) {
          _progress[track.id] = received / total;
          notifyListeners();
        }
        return chunk;
      });
      final path = await _store.save(_fileNameFor(track.id), counted);
      _paths = {..._paths, track.id: path};
      _tracks = {..._tracks, track.id: track.copyWith(libraryAddedAt: DateTime.now().toUtc())};
      await _persist();
      return true;
    } catch (e) {
      error = 'Не удалось сохранить на устройство: $e';
      return false;
    } finally {
      _busy.remove(track.id);
      _progress.remove(track.id);
      notifyListeners();
    }
  }

  Future<void> removeFromDevice(String trackId) async {
    if (!isSupported) return;
    await _store.delete(_fileNameFor(trackId));
    _paths = {..._paths}..remove(trackId);
    _tracks = {..._tracks}..remove(trackId);
    await _persist();
    notifyListeners();
  }

  Future<int> usedBytes() => isSupported ? _store.totalBytes() : Future.value(0);
}
