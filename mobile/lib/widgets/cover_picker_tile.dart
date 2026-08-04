import 'dart:typed_data';
import 'package:flutter/material.dart';
import '../theme/tokens.dart';

/// Cover picker where the preview *is* the control.
///
/// Replaces a preview plus two stacked buttons ("Выбрать обложку" / "Убрать"):
/// tapping the tile picks or replaces, and a small badge clears it. One target
/// instead of three, and the empty state shows the exact square the image will
/// occupy rather than describing it in a label.
class CoverPickerTile extends StatelessWidget {
  const CoverPickerTile({
    super.key,
    required this.bytes,
    required this.onPick,
    required this.onClear,
    this.busy = false,
    this.size = 116,
  });

  final Uint8List? bytes;
  final VoidCallback onPick;
  final VoidCallback onClear;
  final bool busy;
  final double size;

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    final hasCover = bytes != null;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        SizedBox(
          width: size,
          height: size,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              Positioned.fill(
                child: Semantics(
                  button: true,
                  label: hasCover ? 'Заменить обложку' : 'Добавить обложку',
                  child: InkWell(
                    onTap: busy ? null : onPick,
                    borderRadius: BorderRadius.circular(AppRadius.card),
                    child: hasCover
                        ? _filled(context, accent)
                        : _empty(context, accent),
                  ),
                ),
              ),
              if (hasCover && !busy)
                Positioned(
                  top: -6,
                  right: -6,
                  child: Semantics(
                    button: true,
                    label: 'Убрать обложку',
                    child: Tooltip(
                      message: 'Убрать обложку',
                      child: InkWell(
                        onTap: onClear,
                        customBorder: const CircleBorder(),
                        child: Container(
                          width: 28,
                          height: 28,
                          decoration: BoxDecoration(
                            color: AppColors.surface3,
                            shape: BoxShape.circle,
                            border: Border.all(color: AppColors.borderStrong),
                          ),
                          child: const Icon(Icons.close_rounded, size: 15, color: AppColors.fg),
                        ),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            hasCover
                ? 'Нажми на обложку, чтобы заменить.'
                : 'Необязательно. Без своей картинки возьмём обложку первого трека.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ),
      ],
    );
  }

  Widget _filled(BuildContext context, Color accent) => ClipRRect(
        borderRadius: BorderRadius.circular(AppRadius.card),
        child: Stack(
          fit: StackFit.expand,
          children: [
            Image.memory(bytes!, fit: BoxFit.cover),
            if (busy)
              Container(
                color: Colors.black54,
                child: Center(child: CircularProgressIndicator(strokeWidth: 2.4, color: accent)),
              )
            else
              // A quiet affordance so the image does not read as static decoration.
              Positioned(
                left: 0,
                right: 0,
                bottom: 0,
                child: Container(
                  padding: const EdgeInsets.symmetric(vertical: 6),
                  color: Colors.black.withValues(alpha: 0.55),
                  child: const Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(Icons.edit_rounded, size: 13, color: Colors.white),
                      SizedBox(width: 5),
                      Text('Заменить', style: TextStyle(fontSize: 11, color: Colors.white)),
                    ],
                  ),
                ),
              ),
          ],
        ),
      );

  Widget _empty(BuildContext context, Color accent) => CustomPaint(
        painter: _DashedBorderPainter(color: accent.withValues(alpha: 0.5), radius: AppRadius.card),
        child: Container(
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.06),
            borderRadius: BorderRadius.circular(AppRadius.card),
          ),
          child: busy
              ? Center(child: CircularProgressIndicator(strokeWidth: 2.4, color: accent))
              : Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(Icons.add_photo_alternate_outlined, size: 26, color: accent),
                    const SizedBox(height: 6),
                    Text(
                      'Обложка',
                      style: TextStyle(fontSize: 11, color: accent, fontWeight: FontWeight.w600),
                    ),
                  ],
                ),
        ),
      );
}

class _DashedBorderPainter extends CustomPainter {
  _DashedBorderPainter({required this.color, required this.radius});
  final Color color;
  final double radius;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    final rrect = RRect.fromRectAndRadius(Offset.zero & size, Radius.circular(radius));
    final path = Path()..addRRect(rrect);
    const dash = 6.0;
    const gap = 4.0;
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(metric.extractPath(distance, (distance + dash).clamp(0, metric.length)), paint);
        distance += dash + gap;
      }
    }
  }

  @override
  bool shouldRepaint(_DashedBorderPainter oldDelegate) => oldDelegate.color != color;
}
