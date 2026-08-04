import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/api_client.dart';
import '../api/models.dart';
import '../theme/tokens.dart';

class PlaylistArt extends StatelessWidget {
  const PlaylistArt({
    super.key,
    required this.playlist,
    this.radius = AppRadius.card,
  });

  final Playlist playlist;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final url = context.read<ApiClient>().artworkUrl(playlist.coverUrl);
    final fallback = _PlaylistFallback(seed: playlist.id);
    return ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      child: url == null
          ? fallback
          : CachedNetworkImage(
              imageUrl: url,
              fit: BoxFit.cover,
              placeholder: (_, _) => fallback,
              errorWidget: (_, _, _) => fallback,
            ),
    );
  }
}

class _PlaylistFallback extends StatelessWidget {
  const _PlaylistFallback({required this.seed});
  final String seed;

  @override
  Widget build(BuildContext context) {
    var hash = 0;
    for (final code in seed.codeUnits) {
      hash = (hash * 31 + code) & 0x7fffffff;
    }
    final first = HSLColor.fromAHSL(
      1,
      (hash % 360).toDouble(),
      0.55,
      0.28,
    ).toColor();
    final second = HSLColor.fromAHSL(
      1,
      ((hash + 58) % 360).toDouble(),
      0.48,
      0.15,
    ).toColor();
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [first, second],
        ),
      ),
      child: const Center(
        child: Icon(Icons.queue_music_rounded, color: Colors.white70, size: 42),
      ),
    );
  }
}
