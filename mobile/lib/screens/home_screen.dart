import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../state/player_controller.dart';
import '../widgets/cover_strip.dart';
import '../widgets/track_row.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final player = context.watch<PlayerController>();
    final tracks = library.libraryTracks;
    final recentTracks = player.recentTracks;

    return CustomScrollView(
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'КОЛЛЕКЦИЯ',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                    color: Theme.of(context).colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  'Медиатека',
                  style: Theme.of(
                    context,
                  ).textTheme.headlineLarge?.copyWith(fontSize: 40),
                ),
                const SizedBox(height: 8),
                const Text('Всё, что ты лайкнул или скачал, — в одном месте.'),
              ],
            ),
          ),
        ),
        if (tracks.isEmpty && recentTracks.isEmpty)
          const SliverFillRemaining(
            hasScrollBody: false,
            child: _EmptyLibrary(),
          )
        else ...[
          if (recentTracks.isNotEmpty)
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
              sliver: SliverToBoxAdapter(
                child: CoverStrip(
                  eyebrow: 'Слушай снова',
                  tracks: recentTracks,
                  onPlay: (track, index) =>
                      player.playFrom(recentTracks, index),
                ),
              ),
            ),
          if (tracks.isNotEmpty)
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 100),
              sliver: SliverList.builder(
                itemCount: tracks.length,
                itemBuilder: (context, index) {
                  final track = tracks[index];
                  return TrackRow(
                    track: track,
                    liked: library.favoriteIds.contains(track.id),
                    isCurrent: player.currentTrack?.id == track.id,
                    downloaded:
                        track.downloaded ||
                        library.downloadedIds.contains(track.id),
                    downloading: library.downloadingIds.contains(track.id),
                    onPlay: () => player.playFrom(tracks, index),
                    onLike: () => library.toggleFavorite(track),
                    onDownload: track.policy.downloadAllowed
                        ? () async {
                            final ok = await library.downloadTrack(track);
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
                  );
                },
              ),
            ),
        ],
      ],
    );
  }
}

class _EmptyLibrary extends StatelessWidget {
  const _EmptyLibrary();
  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.favorite_border, size: 40),
            const SizedBox(height: 16),
            Text('Пока пусто', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 6),
            const Text(
              'Лайкни трек или скачай его — и он появится здесь.',
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}
