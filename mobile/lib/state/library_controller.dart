import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../api/api_client.dart';
import '../api/models.dart';

enum LoadState { idle, loading, loaded, error }

const _selectedProvidersPrefsKey = 'mo_enabled_search_providers';
const _favoritesCacheKey = 'mo_cache_favorites';
const _downloadsCacheKey = 'mo_cache_downloads';

/// Mirrors the data-fetching parts of SearchPage.tsx (useProviders, useSearch,
/// useFavorites, usePlaylists, selectedProviders) — one controller instead of
/// React Query hooks + local component state.
class LibraryController extends ChangeNotifier {
  LibraryController(this._api) {
    refreshAll();
  }
  final ApiClient _api;

  List<MusicProvider> providers = [];
  List<Track> favorites = [];
  List<Track> downloads = [];
  List<Playlist> playlists = [];
  Set<String> selectedProviderIds = {};

  String searchQuery = '';
  List<Track> searchResults = [];
  LoadState searchState = LoadState.idle;
  Set<String> downloadingIds = {};
  String? actionError;

  Set<String> get favoriteIds => favorites.map((t) => t.id).toSet();
  Set<String> get downloadedIds => downloads.map((t) => t.id).toSet();
  List<Track> get libraryTracks {
    final byId = <String, Track>{};
    for (final track in [...favorites, ...downloads]) {
      if (track.providerId == 'local') continue;
      final current = byId[track.id];
      if (current == null) {
        byId[track.id] = track;
        continue;
      }
      final currentAt = current.libraryAddedAt;
      final trackAt = track.libraryAddedAt;
      final newest =
          currentAt == null || (trackAt != null && trackAt.isAfter(currentAt))
          ? track
          : current;
      byId[track.id] = newest.copyWith(
        downloaded: current.downloaded || track.downloaded,
        downloadMediaUrl: track.downloadMediaUrl ?? current.downloadMediaUrl,
        libraryAddedAt:
            trackAt != null && (currentAt == null || trackAt.isAfter(currentAt))
            ? trackAt
            : currentAt,
      );
    }
    final tracks = byId.values.toList();
    tracks.sort((a, b) {
      final aAt = a.libraryAddedAt;
      final bAt = b.libraryAddedAt;
      if (aAt == null && bAt == null) return 0;
      if (aAt == null) return 1;
      if (bAt == null) return -1;
      return bAt.compareTo(aAt);
    });
    return tracks;
  }

  Future<void> refreshAll() async {
    try {
      providers = (await _api.providers())
          .where((provider) => provider.id != 'local')
          .toList();
    } catch (_) {
      providers = [];
    }
    final prefs = await SharedPreferences.getInstance();
    final stored = prefs.getStringList(_selectedProvidersPrefsKey);
    final available = providers
        .where((p) => p.enabled && p.canSearch)
        .map((p) => p.id)
        .toSet();
    selectedProviderIds = (stored ?? const [])
        .where(available.contains)
        .toSet();
    if (selectedProviderIds.isEmpty) selectedProviderIds = available;
    await Future.wait([
      refreshFavorites(),
      refreshPlaylists(),
      refreshDownloads(),
    ]);
    notifyListeners();
  }

  Future<void> toggleProvider(String id) async {
    if (selectedProviderIds.contains(id)) {
      if (selectedProviderIds.length == 1) return;
      selectedProviderIds = {...selectedProviderIds}..remove(id);
    } else {
      selectedProviderIds = {...selectedProviderIds, id};
    }
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(
      _selectedProvidersPrefsKey,
      selectedProviderIds.toList(),
    );
    if (searchQuery.trim().isNotEmpty) await search(searchQuery);
  }

  Future<void> refreshDownloads() async {
    try {
      downloads = (await _api.downloads())
          .where((track) => track.providerId != 'local')
          .toList();
      await _writeCache(_downloadsCacheKey, downloads);
    } catch (_) {
      // Server unreachable: fall back to the last known list instead of
      // showing an empty library, which reads as "your music is gone".
      downloads = await _readCache(_downloadsCacheKey);
    }
    notifyListeners();
  }

  Future<void> refreshFavorites() async {
    try {
      favorites = (await _api.favorites())
          .where((track) => track.providerId != 'local')
          .toList();
      await _writeCache(_favoritesCacheKey, favorites);
    } catch (_) {
      favorites = await _readCache(_favoritesCacheKey);
    }
    notifyListeners();
  }

  /// The library list is cached verbatim (titles, artists, artwork URLs) so an
  /// offline launch shows the same collection as an online one. Artwork itself
  /// is cached separately by cached_network_image.
  Future<void> _writeCache(String key, List<Track> tracks) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(key, jsonEncode(tracks.map((t) => t.toJson()).toList()));
  }

  Future<List<Track>> _readCache(String key) async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(key);
    if (raw == null) return [];
    try {
      return (jsonDecode(raw) as List)
          .map((e) => Track.fromJson(e as Map<String, dynamic>))
          .toList();
    } catch (_) {
      return [];
    }
  }

  Future<void> refreshPlaylists() async {
    try {
      playlists = await _api.playlists();
    } catch (_) {
      playlists = [];
    }
    notifyListeners();
  }

  Future<void> search(String query) async {
    searchQuery = query;
    if (query.trim().isEmpty) {
      searchResults = [];
      searchState = LoadState.idle;
      notifyListeners();
      return;
    }
    searchState = LoadState.loading;
    notifyListeners();
    try {
      final result = await _api.search(
        query,
        providerIds: selectedProviderIds.toList(),
      );
      searchResults = result.items
          .where((track) => track.providerId != 'local')
          .toList();
      searchState = LoadState.loaded;
    } catch (_) {
      searchState = LoadState.error;
    }
    notifyListeners();
  }

  Future<void> toggleFavorite(Track track) async {
    final liked = favoriteIds.contains(track.id);
    if (liked) {
      favorites = favorites.where((t) => t.id != track.id).toList();
    } else {
      favorites = [
        track.copyWith(libraryAddedAt: DateTime.now().toUtc()),
        ...favorites,
      ];
    }
    notifyListeners();
    try {
      if (liked) {
        await _api.removeFavorite(track.id);
      } else {
        await _api.addFavorite(track);
      }
    } catch (_) {
      await refreshFavorites();
    }
  }

  Future<bool> downloadTrack(Track track) async {
    if (downloadingIds.contains(track.id) || downloadedIds.contains(track.id)) {
      return true;
    }
    downloadingIds = {...downloadingIds, track.id};
    actionError = null;
    notifyListeners();
    try {
      await _api.createDownload(track.id);
      await refreshDownloads();
      return true;
    } catch (e) {
      actionError = '$e';
      return false;
    } finally {
      downloadingIds = {...downloadingIds}..remove(track.id);
      notifyListeners();
    }
  }

  Future<Playlist?> createPlaylist(
    String name, {
    String? description,
    List<Track> tracks = const [],
    Uint8List? coverBytes,
    String? coverFilename,
  }) async {
    actionError = null;
    Playlist? created;
    try {
      created = await _api.createPlaylist(name, description: description);
      var playlist = created;
      if (coverBytes != null && coverFilename != null) {
        playlist = await _api.uploadPlaylistCover(
          playlist.id,
          coverBytes,
          coverFilename,
        );
      }
      for (final track in tracks) {
        await _api.addPlaylistTrack(playlist.id, track.id);
      }
      await refreshPlaylists();
      return playlist;
    } catch (e) {
      if (created != null) {
        try {
          await _api.deletePlaylist(created.id);
        } catch (_) {}
      }
      actionError = '$e';
      await refreshPlaylists();
      return null;
    }
  }

  Future<bool> addPlaylistTrack(String playlistId, String trackId) async {
    actionError = null;
    try {
      await _api.addPlaylistTrack(playlistId, trackId);
      await refreshPlaylists();
      return true;
    } catch (e) {
      actionError = '$e';
      notifyListeners();
      return false;
    }
  }

  Future<bool> removePlaylistTrack(String playlistId, String trackId) async {
    actionError = null;
    try {
      await _api.removePlaylistTrack(playlistId, trackId);
      await refreshPlaylists();
      return true;
    } catch (e) {
      actionError = '$e';
      notifyListeners();
      return false;
    }
  }

  Future<void> deletePlaylist(String playlistId) async {
    await _api.deletePlaylist(playlistId);
    await refreshPlaylists();
  }

  Future<Playlist?> getPlaylist(String playlistId) async {
    actionError = null;
    try {
      return await _api.playlist(playlistId);
    } catch (e) {
      actionError = '$e';
      notifyListeners();
      return null;
    }
  }

  Future<Playlist?> uploadPlaylistCover(
    String playlistId,
    Uint8List bytes,
    String filename,
  ) async {
    actionError = null;
    try {
      final playlist = await _api.uploadPlaylistCover(
        playlistId,
        bytes,
        filename,
      );
      await refreshPlaylists();
      return playlist;
    } catch (e) {
      actionError = '$e';
      notifyListeners();
      return null;
    }
  }
}
