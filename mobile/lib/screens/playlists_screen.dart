import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../theme/tokens.dart';
import '../widgets/cover_picker_tile.dart';
import '../widgets/playlist_art.dart';
import 'playlist_detail_screen.dart';

class PlaylistsScreen extends StatelessWidget {
  const PlaylistsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final playlists = library.playlists;
    return CustomScrollView(
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'КОЛЛЕКЦИИ',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                    color: Theme.of(context).colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  'Плейлисты',
                  style: Theme.of(
                    context,
                  ).textTheme.headlineLarge?.copyWith(fontSize: 40),
                ),
              ],
            ),
          ),
        ),
        if (playlists.isEmpty)
          SliverFillRemaining(
            hasScrollBody: false,
            child: Center(
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(
                      Icons.queue_music_rounded,
                      size: 40,
                      color: AppColors.muted,
                    ),
                    const SizedBox(height: 16),
                    Text(
                      'Плейлистов пока нет',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Нажми «Создать плейлист» и выбери треки из медиатеки.',
                      textAlign: TextAlign.center,
                    ),
                  ],
                ),
              ),
            ),
          )
        else
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 100),
            sliver: SliverGrid(
              gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: 2,
                mainAxisSpacing: 16,
                crossAxisSpacing: 16,
                childAspectRatio: 0.76,
              ),
              delegate: SliverChildBuilderDelegate((context, index) {
                final playlist = playlists[index];
                return GestureDetector(
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) =>
                          PlaylistDetailScreen(initialPlaylist: playlist),
                    ),
                  ),
                  onLongPress: () => library.deletePlaylist(playlist.id),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      AspectRatio(
                        aspectRatio: 1,
                        child: PlaylistArt(playlist: playlist),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        playlist.name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                      Text(
                        '${playlist.trackCount} треков',
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    ],
                  ),
                );
              }, childCount: playlists.length),
            ),
          ),
      ],
    );
  }
}

class CreatePlaylistSheet extends StatefulWidget {
  const CreatePlaylistSheet({super.key});
  @override
  State<CreatePlaylistSheet> createState() => CreatePlaylistSheetState();
}

class CreatePlaylistSheetState extends State<CreatePlaylistSheet> {
  final _name = TextEditingController();
  final _description = TextEditingController();
  final _selected = <String>{};
  Uint8List? _coverBytes;
  String? _coverFilename;
  String? _coverError;
  bool _pickingCover = false;
  bool _saving = false;

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    super.dispose();
  }

  Future<void> _pickCover() async {
    setState(() {
      _pickingCover = true;
      _coverError = null;
    });
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.image,
        withData: true,
      );
      if (result == null || result.files.isEmpty || !mounted) return;
      final file = result.files.single;
      final bytes = file.bytes;
      if (bytes == null || bytes.isEmpty) {
        setState(() => _coverError = 'Не удалось прочитать изображение');
        return;
      }
      if (bytes.length > 8 * 1024 * 1024) {
        setState(() => _coverError = 'Обложка должна быть меньше 8 МБ');
        return;
      }
      setState(() {
        _coverBytes = bytes;
        _coverFilename = file.name;
      });
    } catch (error) {
      if (mounted) setState(() => _coverError = '$error');
    } finally {
      if (mounted) setState(() => _pickingCover = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final candidates = {
      for (final t in [...library.favorites, ...library.downloads]) t.id: t,
    }.values.toList();

    return DraggableScrollableSheet(
      initialChildSize: 0.85,
      minChildSize: 0.5,
      maxChildSize: 0.95,
      builder: (context, scrollController) => Container(
        decoration: const BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.vertical(
            top: Radius.circular(AppRadius.sheet),
          ),
        ),
        child: ListView(
          controller: scrollController,
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
          children: [
            Text(
              'Создать подборку',
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _name,
              autofocus: true,
              onChanged: (_) => setState(() {}),
              decoration: const InputDecoration(
                labelText: 'Название плейлиста',
              ),
            ),
            const SizedBox(height: 10),
            TextField(
              controller: _description,
              decoration: const InputDecoration(
                labelText: 'Описание (необязательно)',
              ),
            ),
            const SizedBox(height: 16),
            Text('ОБЛОЖКА', style: Theme.of(context).textTheme.labelSmall),
            const SizedBox(height: 10),
            CoverPickerTile(
              bytes: _coverBytes,
              busy: _pickingCover,
              onPick: _pickCover,
              onClear: () => setState(() {
                _coverBytes = null;
                _coverFilename = null;
              }),
            ),
            if (_coverError != null) ...[
              const SizedBox(height: 6),
              Text(
                _coverError!,
                style: const TextStyle(color: AppColors.danger, fontSize: 12),
              ),
            ],
            const SizedBox(height: 20),
            Text(
              'Треки из медиатеки (${_selected.length} выбрано)',
              style: Theme.of(context).textTheme.labelSmall,
            ),
            const SizedBox(height: 8),
            if (candidates.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 16),
                child: Text(
                  'Медиатека пустая — лайкни или скачай треки.',
                  style: TextStyle(color: AppColors.muted),
                ),
              )
            else
              for (final track in candidates)
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  value: _selected.contains(track.id),
                  onChanged: (v) => setState(
                    () => v == true
                        ? _selected.add(track.id)
                        : _selected.remove(track.id),
                  ),
                  title: Text(
                    track.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                  subtitle: Text(
                    track.artist ?? 'Исполнитель не указан',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
            const SizedBox(height: 16),
            ElevatedButton(
              onPressed: _name.text.trim().isEmpty || _saving
                  ? null
                  : () async {
                      setState(() => _saving = true);
                      final tracks = candidates
                          .where((t) => _selected.contains(t.id))
                          .toList();
                      final playlist = await library.createPlaylist(
                        _name.text.trim(),
                        description: _description.text.trim(),
                        tracks: tracks,
                        coverBytes: _coverBytes,
                        coverFilename: _coverFilename,
                      );
                      if (!context.mounted) return;
                      if (playlist != null) {
                        Navigator.of(context).pop();
                      } else {
                        setState(() => _saving = false);
                        ScaffoldMessenger.of(context).showSnackBar(
                          SnackBar(
                            content: Text(
                              library.actionError ??
                                  'Не удалось создать плейлист',
                            ),
                          ),
                        );
                      }
                    },
              child: _saving
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Создать'),
            ),
          ],
        ),
      ),
    );
  }
}
