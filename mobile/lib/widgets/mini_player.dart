import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import 'now_playing_sheet.dart';
import 'source_icon.dart';
import 'track_art.dart';

/// Compact docked bar — port of the collapsed state of PlayerBar in
/// SearchPage.tsx. Tapping it opens NowPlayingSheet as a draggable bottom
/// sheet, the native-idiomatic equivalent of the web's expand-to-fullscreen morph.
class MiniPlayer extends StatelessWidget {
  const MiniPlayer({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<PlayerController>(
      builder: (context, player, _) {
        final track = player.currentTrack;
        if (track == null) return const SizedBox.shrink();
        final accent = Theme.of(context).colorScheme.primary;
        return GestureDetector(
          onTap: () => showModalBottomSheet(
            context: context,
            isScrollControlled: true,
            backgroundColor: Colors.transparent,
            builder: (_) => const NowPlayingSheet(),
          ),
          child: Container(
            margin: const EdgeInsets.fromLTRB(12, 0, 12, 8),
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: AppColors.surface2,
              borderRadius: BorderRadius.circular(AppRadius.card),
              boxShadow: const [
                BoxShadow(
                  color: Colors.black45,
                  blurRadius: 24,
                  offset: Offset(0, 8),
                ),
              ],
            ),
            child: Row(
              children: [
                SizedBox(width: 44, height: 44, child: TrackArt(track: track)),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        track.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w600,
                          fontSize: 13,
                        ),
                      ),
                      Row(
                        children: [
                          SourceIcon(providerId: track.providerId, size: 13),
                          const SizedBox(width: 4),
                          Expanded(
                            child: Text(
                              track.artist ?? '',
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
                  onPressed: () => player.togglePlayPause(),
                  icon: player.isBuffering
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: accent,
                          ),
                        )
                      : Icon(
                          player.isPlaying
                              ? Icons.pause_circle_filled
                              : Icons.play_circle_filled,
                          size: 34,
                          color: accent,
                        ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}
