import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/models.dart';
import '../state/library_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import '../widgets/playlist_art.dart';
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

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final player = context.watch<PlayerController>();
    final tracks = _tracks;
    return Scaffold(
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
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 40),
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
