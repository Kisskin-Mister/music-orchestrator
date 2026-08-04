import 'package:flutter/material.dart';
import '../api/models.dart';
import '../theme/tokens.dart';
import 'source_icon.dart';
import 'track_art.dart';

/// "Слушай снова" — port of CoverStrip in SearchPage.tsx: turns a flat list
/// into a browsable home row, the way real streaming apps do.
class CoverStrip extends StatelessWidget {
  const CoverStrip({
    super.key,
    required this.eyebrow,
    required this.tracks,
    required this.onPlay,
  });
  final String eyebrow;
  final List<Track> tracks;
  final void Function(Track track, int index) onPlay;

  @override
  Widget build(BuildContext context) {
    if (tracks.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(left: 4, bottom: 10),
          child: Text(
            eyebrow.toUpperCase(),
            style: Theme.of(context).textTheme.labelSmall,
          ),
        ),
        SizedBox(
          height: 190,
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            itemCount: tracks.length.clamp(0, 12),
            separatorBuilder: (context, index) => const SizedBox(width: 14),
            itemBuilder: (context, index) {
              final track = tracks[index];
              return GestureDetector(
                onTap: () => onPlay(track, index),
                child: SizedBox(
                  width: 140,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(
                        width: 140,
                        height: 140,
                        child: TrackArt(track: track, radius: AppRadius.card),
                      ),
                      const SizedBox(height: 8),
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
                              track.artist ?? 'Исполнитель не указан',
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
              );
            },
          ),
        ),
      ],
    );
  }
}
