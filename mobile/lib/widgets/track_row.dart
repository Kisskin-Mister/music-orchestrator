import 'package:flutter/material.dart';
import '../api/models.dart';
import '../theme/tokens.dart';
import 'source_icon.dart';
import 'track_art.dart';
import 'track_actions_sheet.dart';

String formatDuration(int? seconds) {
  if (seconds == null || seconds <= 0) return '—';
  final m = seconds ~/ 60;
  final s = (seconds % 60).toString().padLeft(2, '0');
  return '$m:$s';
}

class TrackRow extends StatelessWidget {
  const TrackRow({
    super.key,
    required this.track,
    required this.liked,
    required this.onPlay,
    required this.onLike,
    this.isCurrent = false,
    this.downloaded = false,
    this.downloading = false,
    this.onDownload,
    this.playlistId,
    this.onRemovedFromPlaylist,
  });
  final Track track;
  final bool liked;
  final bool isCurrent;
  final VoidCallback onPlay;
  final VoidCallback onLike;
  final bool downloaded;
  final bool downloading;
  final VoidCallback? onDownload;

  /// Set when the row lives inside a playlist, so the actions menu can offer
  /// "remove from this playlist".
  final String? playlistId;
  final VoidCallback? onRemovedFromPlaylist;

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    return InkWell(
      onTap: onPlay,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 8),
        decoration: BoxDecoration(
          color: isCurrent ? accent.withValues(alpha: 0.08) : null,
          borderRadius: BorderRadius.circular(AppRadius.control),
        ),
        child: Row(
          children: [
            SizedBox(
              width: 52,
              height: 52,
              child: TrackArt(track: track, radius: AppRadius.control),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    track.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 3),
                  Row(
                    children: [
                      SourceIcon(providerId: track.providerId, size: 15),
                      const SizedBox(width: 5),
                      Expanded(
                        child: Text(
                          '${track.artist ?? 'Исполнитель не указан'} · ${formatDuration(track.durationSeconds)}',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            IconButton(
              onPressed: onLike,
              tooltip: liked ? 'Убрать лайк' : 'Поставить лайк',
              icon: Icon(
                liked ? Icons.favorite : Icons.favorite_border,
                color: liked ? AppColors.danger : AppColors.muted,
                size: 20,
              ),
            ),
            // Download state stays visible as a static badge, because the
            // actions menu hides it behind a tap — "is this already offline?"
            // has to be answerable while scrolling.
            if (downloading)
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 12),
                child: SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                ),
              )
            else if (downloaded)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                child: Icon(
                  Icons.download_done_rounded,
                  color: accent,
                  size: 18,
                ),
              ),
            IconButton(
              onPressed: () => showTrackActions(
                context,
                track: track,
                playlistId: playlistId,
                onRemovedFromPlaylist: onRemovedFromPlaylist,
              ),
              tooltip: 'Действия с треком',
              icon: const Icon(
                Icons.more_vert_rounded,
                color: AppColors.muted,
                size: 20,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
