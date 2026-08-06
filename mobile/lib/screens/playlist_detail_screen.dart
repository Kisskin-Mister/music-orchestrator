import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/models.dart';
import '../state/library_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import '../widgets/playlist_art.dart';
import '../widgets/track_art.dart';
import '../widgets/track_row.dart';

class PlaylistDetailScreen extends StatefulWidget {
  const PlaylistDetailScreen({super.key, required this.initialPlaylist});

  final Playlist initialPlaylist;

  @override
  State<PlaylistDetailScreen> createState() => _PlaylistDetailScreenState();
}

class _PlaylistDetailScreenState extends State<PlaylistDetailScreen> {
  late Playlist _playlist = widget.initialPlaylist;
  bool _loading = false;
  bool _uploading = false;

  List<Track> get _tracks =>
      _playlist.tracks.map((item) => item.track).whereType<Track>().toList();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _refresh());
  }

  Future<void> _refresh() async {
    if (_loading) return;
    setState(() => _loading = true);
    final playlist = await context.read<LibraryController>().getPlaylist(
      _playlist.id,
    );
    if (!mounted) return;
    setState(() {
      if (playlist != null) _playlist = playlist;
      _loading = false;
    });
  }

  Future<void> _pickCover() async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.image,
      withData: true,
    );
    if (result == null || result.files.isEmpty || !mounted) return;
    final selected = result.files.single;
    final Uint8List? bytes = selected.bytes;
    if (bytes == null || bytes.isEmpty || !mounted) return;
    if (bytes.length > 8 * 1024 * 1024) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Обложка должна быть меньше 8 МБ')),
      );
      return;
    }

    setState(() => _uploading = true);
    final playlist = await context
        .read<LibraryController>()
        .uploadPlaylistCover(_playlist.id, bytes, selected.name);
    if (!mounted) return;
    setState(() {
      if (playlist != null) _playlist = playlist;
      _uploading = false;
    });
    if (playlist == null) {
      final error = context.read<LibraryController>().actionError;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(error ?? 'Не удалось загрузить обложку')),
      );
    }
  }

  /// Filling a playlist used to mean leaving it, finding the track in Search
  /// and using its menu. The sheet is the same flow the web PlaylistCard got:
  /// search scoped to this playlist, add without losing your place.
  Future<void> _addTracks() async {
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(
          top: Radius.circular(AppRadius.sheet),
        ),
      ),
      builder: (context) => _AddTrackSheet(
        playlistId: _playlist.id,
        existingIds: _tracks.map((track) => track.id).toSet(),
      ),
    );
    if (!mounted) return;
    await _refresh();
  }

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final player = context.watch<PlayerController>();
    final tracks = _tracks;
    return Scaffold(
      floatingActionButton: FloatingActionButton(
        onPressed: _addTracks,
        tooltip: 'Добавить трек',
        child: const Icon(Icons.add),
      ),
      appBar: AppBar(
        title: const Text('Плейлист'),
        backgroundColor: AppColors.bg,
        actions: [
          IconButton(
            tooltip: 'Обновить',
            onPressed: _loading ? null : _refresh,
            icon: _loading
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: ListView(
        // Bottom pad clears the "+" button so it never sits on the last row.
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 96),
        children: [
          Center(
            child: SizedBox.square(
              dimension: 260,
              child: PlaylistArt(playlist: _playlist, radius: AppRadius.sheet),
            ),
          ),
          const SizedBox(height: 22),
          Text(
            _playlist.name,
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          if (_playlist.description?.trim().isNotEmpty == true) ...[
            const SizedBox(height: 6),
            Text(
              _playlist.description!,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          const SizedBox(height: 14),
          Text(
            '${tracks.length} треков · ${formatDuration(_playlist.durationSeconds)}',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: ElevatedButton.icon(
                  onPressed: tracks.isEmpty
                      ? null
                      : () => player.playFrom(tracks, 0),
                  icon: const Icon(Icons.play_arrow_rounded),
                  label: const Text('Слушать'),
                ),
              ),
              const SizedBox(width: 10),
              OutlinedButton.icon(
                onPressed: _uploading ? null : _pickCover,
                icon: _uploading
                    ? const SizedBox(
                        width: 17,
                        height: 17,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.add_photo_alternate_outlined),
                label: const Text('Обложка'),
              ),
            ],
          ),
          const Divider(height: 36),
          if (tracks.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Column(
                children: [
                  Icon(
                    Icons.queue_music_rounded,
                    size: 42,
                    color: AppColors.muted,
                  ),
                  SizedBox(height: 12),
                  Text('В этом плейлисте пока нет треков'),
                ],
              ),
            )
          else
            for (var index = 0; index < tracks.length; index++)
              TrackRow(
                track: tracks[index],
                liked: library.favoriteIds.contains(tracks[index].id),
                isCurrent: player.currentTrack?.id == tracks[index].id,
                downloaded: library.downloadedIds.contains(tracks[index].id),
                downloading: library.downloadingIds.contains(tracks[index].id),
                onPlay: () => player.playFrom(tracks, index),
                onLike: () => library.toggleFavorite(tracks[index]),
                onDownload: tracks[index].policy.downloadAllowed
                    ? () async {
                        final ok = await library.downloadTrack(tracks[index]);
                        if (!ok && context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(
                              content: Text(
                                library.actionError ??
                                    'Не удалось скачать трек',
                              ),
                            ),
                          );
                        }
                      }
                    : null,
              ),
        ],
      ),
    );
  }
}

/// Search-and-add sheet behind the playlist's "+" button. Keeps its own
/// results rather than going through LibraryController.search(), so opening it
/// does not wipe out whatever the Search tab is showing.
class _AddTrackSheet extends StatefulWidget {
  const _AddTrackSheet({required this.playlistId, required this.existingIds});

  final String playlistId;
  final Set<String> existingIds;

  @override
  State<_AddTrackSheet> createState() => _AddTrackSheetState();
}

class _AddTrackSheetState extends State<_AddTrackSheet> {
  final _controller = TextEditingController();
  List<Track> _results = [];
  final Set<String> _added = {};
  bool _searching = false;
  bool _searched = false;
  String? _error;
  String? _addingId;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final query = _controller.text.trim();
    if (query.isEmpty || _searching) return;
    setState(() {
      _searching = true;
      _error = null;
    });
    try {
      final found = await context.read<LibraryController>().searchTracks(query);
      if (!mounted) return;
      setState(() {
        _results = found;
        _searching = false;
        _searched = true;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _searching = false;
        _searched = true;
        _error = 'Поиск не удался. Попробуй ещё раз.';
      });
    }
  }

  Future<void> _add(Track track) async {
    if (_addingId != null) return;
    setState(() => _addingId = track.id);
    final library = context.read<LibraryController>();
    final ok = await library.addPlaylistTrack(widget.playlistId, track.id);
    if (!mounted) return;
    setState(() {
      _addingId = null;
      if (ok) _added.add(track.id);
    });
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(library.actionError ?? 'Не удалось добавить трек'),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      // Lifts the sheet above the keyboard the search field just opened.
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      'Добавить трек',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                  ),
                  IconButton(
                    tooltip: 'Закрыть',
                    onPressed: () => Navigator.of(context).pop(),
                    icon: const Icon(Icons.close_rounded),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _controller,
                autofocus: true,
                textInputAction: TextInputAction.search,
                onSubmitted: (_) => _search(),
                decoration: InputDecoration(
                  hintText: 'Название или исполнитель',
                  prefixIcon: const Icon(Icons.search),
                  suffixIcon: _searching
                      ? const Padding(
                          padding: EdgeInsets.all(14),
                          child: SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          ),
                        )
                      : IconButton(
                          tooltip: 'Найти',
                          onPressed: _search,
                          icon: const Icon(Icons.arrow_forward_rounded),
                        ),
                ),
              ),
              const SizedBox(height: 12),
              ConstrainedBox(
                constraints: BoxConstraints(
                  maxHeight: MediaQuery.of(context).size.height * 0.42,
                ),
                child: _body(context),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _body(BuildContext context) {
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 24),
        child: Text(_error!, style: Theme.of(context).textTheme.bodySmall),
      );
    }
    if (_results.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 24),
        child: Text(
          _searched
              ? 'Ничего не нашлось — попробуй другой запрос.'
              : 'Введи название и нажми поиск.',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      );
    }
    return ListView.separated(
      shrinkWrap: true,
      itemCount: _results.length,
      separatorBuilder: (context, index) => const SizedBox(height: 6),
      itemBuilder: (context, index) {
        final track = _results[index];
        final already =
            widget.existingIds.contains(track.id) || _added.contains(track.id);
        return Row(
          children: [
            SizedBox.square(
              dimension: 44,
              child: TrackArt(track: track),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    track.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '${track.artist ?? 'Исполнитель не указан'} · ${formatDuration(track.durationSeconds)}',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            if (_addingId == track.id)
              const SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            else
              IconButton(
                tooltip: already ? 'Уже в плейлисте' : 'Добавить',
                onPressed: already ? null : () => _add(track),
                icon: Icon(
                  already ? Icons.check_rounded : Icons.add_rounded,
                  color: already
                      ? Theme.of(context).colorScheme.primary
                      : AppColors.muted,
                ),
              ),
          ],
        );
      },
    );
  }
}
