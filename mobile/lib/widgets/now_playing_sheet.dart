import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/models.dart';
import '../state/library_controller.dart';
import '../state/offline_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import 'icon_action.dart';
import 'source_icon.dart';
import 'track_art.dart';
import 'track_row.dart';

/// Full player — port of the expanded PlayerBar state in SearchPage.tsx.
/// A modal bottom sheet is the native-idiomatic container here (matches
/// the "bottom sheet for secondary content" pattern current streaming apps use).
class NowPlayingSheet extends StatelessWidget {
  const NowPlayingSheet({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer2<PlayerController, LibraryController>(
      builder: (context, player, library, _) {
        final track = player.currentTrack;
        if (track == null) return const SizedBox.shrink();
        final accent = Theme.of(context).colorScheme.primary;
        final liked = library.favoriteIds.contains(track.id);
        final downloaded =
            track.downloaded || library.downloadedIds.contains(track.id);
        final downloading = library.downloadingIds.contains(track.id);
        return DraggableScrollableSheet(
          initialChildSize: 0.9,
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
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
              children: [
                Center(
                  child: Container(
                    width: 44,
                    height: 5,
                    decoration: BoxDecoration(
                      color: Colors.white24,
                      borderRadius: BorderRadius.circular(3),
                    ),
                  ),
                ),
                const SizedBox(height: 24),
                AspectRatio(
                  aspectRatio: 1,
                  child: TrackArt(track: track, radius: AppRadius.sheet),
                ),
                const SizedBox(height: 24),
                Text(
                  track.title,
                  style: Theme.of(context).textTheme.headlineMedium,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 4),
                Wrap(
                  crossAxisAlignment: WrapCrossAlignment.center,
                  spacing: 10,
                  runSpacing: 8,
                  children: [
                    SourceBadge(providerId: track.providerId),
                    Text(
                      track.artist ?? 'Исполнитель не указан',
                      style: Theme.of(
                        context,
                      ).textTheme.bodySmall?.copyWith(fontSize: 15),
                    ),
                  ],
                ),
                const SizedBox(height: 20),
                StreamBuilder<Duration>(
                  stream: player.positionStream,
                  builder: (context, snapshot) {
                    final position = snapshot.data ?? Duration.zero;
                    final total = player.duration;
                    final max = total.inMilliseconds > 0
                        ? total.inMilliseconds.toDouble()
                        : 1.0;
                    return Column(
                      children: [
                        SliderTheme(
                          data: SliderTheme.of(context).copyWith(
                            trackHeight: 3,
                            thumbShape: const RoundSliderThumbShape(
                              enabledThumbRadius: 6,
                            ),
                          ),
                          child: Slider(
                            value: position.inMilliseconds
                                .clamp(0, max.toInt())
                                .toDouble(),
                            max: max,
                            activeColor: accent,
                            inactiveColor: Colors.white12,
                            onChanged: total.inMilliseconds > 0
                                ? (v) => player.seek(
                                    Duration(milliseconds: v.round()),
                                  )
                                : null,
                          ),
                        ),
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 4),
                          child: Row(
                            mainAxisAlignment: MainAxisAlignment.spaceBetween,
                            children: [
                              Text(
                                formatDuration(position.inSeconds),
                                style: Theme.of(context).textTheme.labelSmall,
                              ),
                              Text(
                                formatDuration(total.inSeconds),
                                style: Theme.of(context).textTheme.labelSmall,
                              ),
                            ],
                          ),
                        ),
                      ],
                    );
                  },
                ),
                const SizedBox(height: 12),
                if (player.error != null)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: Text(
                      player.error!,
                      style: const TextStyle(
                        color: AppColors.danger,
                        fontSize: 13,
                      ),
                      textAlign: TextAlign.center,
                    ),
                  ),
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                  children: [
                    IconButton(
                      tooltip: player.shuffleEnabled
                          ? 'Выключить перемешивание'
                          : 'Перемешать очередь',
                      color: player.shuffleEnabled ? accent : null,
                      onPressed: player.toggleShuffle,
                      icon: const Icon(Icons.shuffle_rounded),
                    ),
                    IconButton(
                      iconSize: 32,
                      onPressed: player.previous,
                      icon: const Icon(Icons.skip_previous_rounded),
                    ),
                    Container(
                      decoration: BoxDecoration(
                        color: accent,
                        shape: BoxShape.circle,
                      ),
                      child: IconButton(
                        iconSize: 40,
                        color: accent.computeLuminance() > 0.5
                            ? Colors.black
                            : Colors.white,
                        onPressed: () => player.togglePlayPause(),
                        icon: Icon(
                          player.isPlaying
                              ? Icons.pause_rounded
                              : Icons.play_arrow_rounded,
                        ),
                      ),
                    ),
                    IconButton(
                      iconSize: 32,
                      onPressed: player.next,
                      icon: const Icon(Icons.skip_next_rounded),
                    ),
                    IconButton(
                      tooltip: switch (player.repeatMode) {
                        PlayerRepeatMode.off => 'Включить повтор очереди',
                        PlayerRepeatMode.all => 'Повторять текущий трек',
                        PlayerRepeatMode.one => 'Выключить повтор',
                      },
                      color: player.repeatMode == PlayerRepeatMode.off
                          ? null
                          : accent,
                      onPressed: player.cycleRepeatMode,
                      icon: Icon(
                        player.repeatMode == PlayerRepeatMode.one
                            ? Icons.repeat_one_rounded
                            : Icons.repeat_rounded,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 18),
                _TrackActions(track: track, liked: liked, downloadedOnServer: downloaded, serverBusy: downloading),
                if (player.queue.length > 1) ...[
                  const SizedBox(height: 28),
                  Text(
                    'ДАЛЬШЕ В ОЧЕРЕДИ',
                    style: Theme.of(context).textTheme.labelSmall,
                  ),
                  const SizedBox(height: 8),
                  ...List.generate(player.queue.length, (i) {
                    if (i == player.queueIndex) return const SizedBox.shrink();
                    final t = player.queue[i];
                    return TrackRow(
                      track: t,
                      liked: library.favoriteIds.contains(t.id),
                      isCurrent: false,
                      onPlay: () => player.playFrom(player.queue, i),
                      onLike: () => library.toggleFavorite(t),
                      downloaded:
                          t.downloaded || library.downloadedIds.contains(t.id),
                      downloading: library.downloadingIds.contains(t.id),
                      onDownload: t.policy.downloadAllowed
                          ? () => library.downloadTrack(t)
                          : null,
                    );
                  }),
                ],
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Like / save-to-device / save-to-server as icon-only actions.
///
/// The two download targets are ordered device-then-server because a device
/// copy is the stronger guarantee (plays with no network at all) and it
/// implicitly creates the server copy when one is missing.
class _TrackActions extends StatelessWidget {
  const _TrackActions({
    required this.track,
    required this.liked,
    required this.downloadedOnServer,
    required this.serverBusy,
  });

  final Track track;
  final bool liked;
  final bool downloadedOnServer;
  final bool serverBusy;

  @override
  Widget build(BuildContext context) {
    final library = context.read<LibraryController>();
    final offline = context.watch<OfflineController>();
    final onDevice = offline.isOnDevice(track.id);
    final deviceBusy = offline.isBusy(track.id);
    final canDownload = track.policy.downloadAllowed;

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        IconAction(
          icon: liked ? Icons.favorite_rounded : Icons.favorite_border_rounded,
          label: liked ? 'Убрать из медиатеки' : 'В медиатеку',
          active: liked,
          activeColor: AppColors.danger,
          onPressed: () => library.toggleFavorite(track),
        ),
        if (OfflineController.isSupported) ...[
          const SizedBox(width: 12),
          IconAction(
            icon: onDevice ? Icons.offline_pin_rounded : Icons.download_for_offline_outlined,
            label: onDevice ? 'Есть на устройстве — удалить копию' : 'Скачать на устройство',
            active: onDevice,
            busy: deviceBusy,
            progress: offline.progressFor(track.id),
            onPressed: !canDownload
                ? null
                : () async {
                    if (onDevice) {
                      await offline.removeFromDevice(track.id);
                      return;
                    }
                    final ok = await offline.saveToDevice(track, onServerCopyCreated: library.refreshDownloads);
                    if (!ok && context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(content: Text(offline.error ?? 'Не удалось сохранить на устройство')),
                      );
                    }
                  },
          ),
        ],
        const SizedBox(width: 12),
        IconAction(
          icon: downloadedOnServer ? Icons.cloud_done_rounded : Icons.cloud_download_outlined,
          label: downloadedOnServer ? 'Сохранено на сервере' : 'Скачать на сервер',
          active: downloadedOnServer,
          busy: serverBusy,
          onPressed: !canDownload || downloadedOnServer
              ? null
              : () async {
                  final ok = await library.downloadTrack(track);
                  if (!ok && context.mounted) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(content: Text(library.actionError ?? 'Не удалось скачать трек')),
                    );
                  }
                },
        ),
      ],
    );
  }
}
