import 'package:flutter/material.dart';
import '../theme/tokens.dart';

/// Shimmering placeholder block.
///
/// Skeletons beat a spinner here because the result shape is known in advance:
/// the layout stops jumping when data lands, and the wait reads as "this list
/// is filling in" instead of "something is happening somewhere".
class SkeletonBox extends StatefulWidget {
  const SkeletonBox({super.key, this.width, this.height = 14, this.radius = 6, this.widthFactor});

  final double? width;
  final double? widthFactor;
  final double height;
  final double radius;

  @override
  State<SkeletonBox> createState() => _SkeletonBoxState();
}

class _SkeletonBoxState extends State<SkeletonBox> with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Respect the OS "reduce motion" setting: a static block still communicates
    // the pending shape without an endlessly moving highlight.
    final animate = !MediaQuery.disableAnimationsOf(context);
    final box = Container(
      width: widget.width,
      height: widget.height,
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(widget.radius),
      ),
    );
    final sized = widget.widthFactor != null
        ? FractionallySizedBox(alignment: Alignment.centerLeft, widthFactor: widget.widthFactor, child: box)
        : box;
    if (!animate) return sized;

    return AnimatedBuilder(
      animation: _controller,
      child: sized,
      builder: (context, child) => ShaderMask(
        blendMode: BlendMode.srcATop,
        shaderCallback: (bounds) {
          final slide = (_controller.value * 2) - 0.5;
          return LinearGradient(
            begin: Alignment.centerLeft,
            end: Alignment.centerRight,
            colors: const [Colors.transparent, Colors.white10, Colors.transparent],
            stops: [(slide - 0.3).clamp(0.0, 1.0), slide.clamp(0.0, 1.0), (slide + 0.3).clamp(0.0, 1.0)],
          ).createShader(bounds);
        },
        child: child,
      ),
    );
  }
}

/// Placeholder shaped like a TrackRow, so the list does not reflow on load.
class TrackRowSkeleton extends StatelessWidget {
  const TrackRowSkeleton({super.key});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 8),
      child: Row(
        children: [
          const SkeletonBox(width: 52, height: 52, radius: AppRadius.control),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: const [
                SkeletonBox(height: 15, widthFactor: 0.72),
                SizedBox(height: 8),
                SkeletonBox(height: 12, widthFactor: 0.42),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Placeholder shaped like the horizontal cover strip on the library screen.
class CoverStripSkeleton extends StatelessWidget {
  const CoverStripSkeleton({super.key});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 190,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: EdgeInsets.zero,
        itemCount: 4,
        separatorBuilder: (context, index) => const SizedBox(width: 14),
        itemBuilder: (context, index) => const SizedBox(
          width: 140,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              SkeletonBox(width: 140, height: 140, radius: AppRadius.card),
              SizedBox(height: 10),
              SkeletonBox(height: 13, widthFactor: 0.85),
              SizedBox(height: 6),
              SkeletonBox(height: 11, widthFactor: 0.55),
            ],
          ),
        ),
      ),
    );
  }
}
