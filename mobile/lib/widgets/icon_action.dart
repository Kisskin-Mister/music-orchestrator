import 'package:flutter/material.dart';
import '../theme/tokens.dart';

/// Circular icon-only action.
///
/// Icon-only controls need two things the label used to provide: an accessible
/// name (tooltip + Semantics) and a state cue that is not colour alone — so an
/// active action also swaps to a filled icon and a tinted surface.
class IconAction extends StatelessWidget {
  const IconAction({
    super.key,
    required this.icon,
    required this.label,
    this.onPressed,
    this.active = false,
    this.activeColor,
    this.progress,
    this.busy = false,
  });

  final IconData icon;

  /// Spoken by screen readers and shown on long-press/hover.
  final String label;
  final VoidCallback? onPressed;
  final bool active;
  final Color? activeColor;

  /// 0..1 for determinate progress; null with [busy] gives a spinner.
  final double? progress;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    final accent = activeColor ?? Theme.of(context).colorScheme.primary;
    final enabled = onPressed != null && !busy;
    final foreground = !enabled
        ? AppColors.muted.withValues(alpha: 0.4)
        : active
        ? accent
        : AppColors.muted;

    return Tooltip(
      message: label,
      child: Semantics(
        button: true,
        enabled: enabled,
        label: label,
        child: InkWell(
          onTap: enabled ? onPressed : null,
          customBorder: const CircleBorder(),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            curve: Curves.easeOut,
            width: 48,
            height: 48,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: active ? accent.withValues(alpha: 0.14) : Colors.transparent,
              border: Border.all(
                color: active ? accent.withValues(alpha: 0.45) : AppColors.border,
              ),
            ),
            child: busy
                ? Padding(
                    padding: const EdgeInsets.all(13),
                    child: CircularProgressIndicator(
                      strokeWidth: 2.2,
                      value: progress,
                      color: accent,
                    ),
                  )
                : Icon(icon, size: 21, color: foreground),
          ),
        ),
      ),
    );
  }
}
