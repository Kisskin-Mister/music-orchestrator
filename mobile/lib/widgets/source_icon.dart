import 'package:flutter/material.dart';

String sourceName(String providerId) {
  if (providerId.contains('youtube')) return 'YouTube';
  if (providerId.contains('soundcloud')) return 'SoundCloud';
  return providerId;
}

IconData sourceIconData(String providerId) {
  if (providerId.contains('youtube')) return Icons.smart_display_rounded;
  if (providerId.contains('soundcloud')) return Icons.cloud_rounded;
  return Icons.graphic_eq_rounded;
}

Color sourceColor(String providerId, BuildContext context) {
  if (providerId.contains('youtube')) return const Color(0xFFFF5B63);
  if (providerId.contains('soundcloud')) return const Color(0xFFFF8A3D);
  return Theme.of(context).colorScheme.primary;
}

class SourceIcon extends StatelessWidget {
  const SourceIcon({
    super.key,
    required this.providerId,
    this.size = 16,
    this.color,
  });

  final String providerId;
  final double size;
  final Color? color;

  @override
  Widget build(BuildContext context) => Semantics(
    label: sourceName(providerId),
    child: Icon(
      sourceIconData(providerId),
      size: size,
      color: color ?? sourceColor(providerId, context),
    ),
  );
}

class SourceBadge extends StatelessWidget {
  const SourceBadge({
    super.key,
    required this.providerId,
    this.compact = false,
  });

  final String providerId;
  final bool compact;

  @override
  Widget build(BuildContext context) => Container(
    padding: EdgeInsets.symmetric(
      horizontal: compact ? 7 : 9,
      vertical: compact ? 4 : 6,
    ),
    decoration: BoxDecoration(
      color: sourceColor(providerId, context).withValues(alpha: 0.12),
      borderRadius: BorderRadius.circular(999),
      border: Border.all(
        color: sourceColor(providerId, context).withValues(alpha: 0.28),
      ),
    ),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        SourceIcon(providerId: providerId, size: compact ? 13 : 15),
        const SizedBox(width: 5),
        Text(
          sourceName(providerId),
          style: TextStyle(fontSize: compact ? 10 : 12),
        ),
      ],
    ),
  );
}
