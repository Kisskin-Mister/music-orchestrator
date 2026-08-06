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
  int searchTotal = 0;
  bool searchLoadingMore = false;
  Set<String> downloadingIds = {};
  String? actionError;

  static const int searchPageSize = 20;

  /// Raw items the server has handed us so far — the offset of the next page.
  /// Counted before the `local` filter below, otherwise dropping a local track
  /// would shift every following page by one and lose results.
  int _searchFetched = 0;

  /// Bumped on every new query so a page that lands late cannot append itself
  /// to the results of a query the user has already replaced.
  int _searchGeneration = 0;

  bool get canLoadMoreResults =>
      searchState == LoadState.loaded && _searchFetched < searchTotal;

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
    final generation = ++_searchGeneration;
    searchTotal = 0;
    _searchFetched = 0;
    searchLoadingMore = false;
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
        limit: searchPageSize,
        offset: 0,
      );
      if (generation != _searchGeneration) return;
      searchResults = result.items
          .where((track) => track.providerId != 'local')
          .toList();
      _searchFetched = result.items.length;
      // A total the server cannot actually deliver would make the scroll
      // sentinel refetch the same empty page forever, so an empty first page
      // ends pagination regardless of what `total` claims.
      searchTotal = result.items.isEmpty ? 0 : result.total;
      searchState = LoadState.loaded;
    } catch (_) {
      if (generation != _searchGeneration) return;
      searchState = LoadState.error;
    }
    notifyListeners();
  }

  /// Appends the next page to [searchResults]. Called from the search screen
  /// as the list end comes into view; safe to call repeatedly — it no-ops
  /// while a page is in flight or once every result has been fetched.
  Future<void> loadMoreResults() async {
    if (searchLoadingMore || !canLoadMoreResults) return;
    final generation = _searchGeneration;
    searchLoadingMore = true;
    notifyListeners();
    try {
      final result = await _api.search(
        searchQuery,
        providerIds: selectedProviderIds.toList(),
        limit: searchPageSize,
        offset: _searchFetched,
      );
      if (generation != _searchGeneration) return;
      if (result.items.isEmpty) {
        searchTotal = _searchFetched;
      } else {
        _searchFetched += result.items.length;
        searchTotal = result.total;
        // Providers can repeat a track across pages; `seen.add` returning
        // false drops both those and any duplicate inside this page.
        final seen = searchResults.map((track) => track.id).toSet();
        searchResults = [
          ...searchResults,
          ...result.items.where(
            (track) => track.providerId != 'local' && seen.add(track.id),
          ),
        ];
      }
    } catch (_) {
      // Keep what is already on screen; scrolling again retries the page.
    } finally {
      if (generation == _searchGeneration) {
        searchLoadingMore = false;
        notifyListeners();
      }
    }
  }

  /// One-off search that leaves [searchResults] alone — the playlist "add
  /// track" sheet must not overwrite what the Search tab is showing.
  Future<List<Track>> searchTracks(String query) async {
    final result = await _api.search(
      query,
      providerIds: selectedProviderIds.toList(),
      limit: searchPageSize,
      offset: 0,
    );
    return result.items
        .where((track) => track.providerId != 'local')
        .toList();
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
