import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/api_client.dart';
import '../api/models.dart';
import '../theme/tokens.dart';

/// Deterministic two-tone gradient per track id — port of artHues()/TrackArt
/// in SearchPage.tsx, so tracks without artwork still get a distinct cover
/// instead of a flat grey box.
({double a, double b}) _artHues(String seed) {
  var h = 0;
  for (final code in seed.codeUnits) {
    h = (h * 31 + code) & 0x7fffffff;
  }
  final a = (h % 360).toDouble();
  final b = (a + 46) % 360;
  return (a: a, b: b);
}

class TrackArt extends StatelessWidget {
  const TrackArt({
    super.key,
    required this.track,
    this.radius = AppRadius.control,
  });
  final Track track;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final url = context.read<ApiClient>().artworkUrl(track.artworkUrl);
    return ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      // The gradient doubles as the placeholder so a cover slot is never blank
      // while a remote thumbnail loads (or fails) — same behaviour as the web TrackArt.
      child: url != null
          ? CachedNetworkImage(
              imageUrl: url,
              fit: BoxFit.cover,
              fadeInDuration: const Duration(milliseconds: 200),
              placeholder: (context, url) => _gradient(track.id),
              errorWidget: (context, url, error) => _gradient(track.id),
            )
          : _gradient(track.id),
    );
  }

  Widget _gradient(String seed) {
    final hues = _artHues(seed);
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            HSLColor.fromAHSL(1, hues.a, 0.58, 0.26).toColor(),
            HSLColor.fromAHSL(1, hues.b, 0.46, 0.15).toColor(),
          ],
        ),
      ),
    );
  }
}
