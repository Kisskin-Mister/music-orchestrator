import 'package:flutter/material.dart';

/// Same palette/radius/font scale as the web app's styles.css, so the two
/// clients read as one product. Keep values in sync with:
/// frontend/frontend/src/styles.css (:root and @theme blocks).
class AppColors {
  static const bg = Color(0xFF08090C);
  static const surface = Color(0xFF0F1117);
  static const surface2 = Color(0xFF151923);
  static const surface3 = Color(0xFF1C212C);
  static const fg = Color(0xFFF6F3EA);
  static const muted = Color(0xFF8C919E);
  static const subtle = Color(0xFF626875);
  static const border = Color(0x1AFFFFFF); // white 10%
  static const borderStrong = Color(0x26FFFFFF); // white 15%
  static const danger = Color(0xFFEB777C);
  static const defaultAccent = Color(0xFFB8F545);
}

class AppRadius {
  static const control = 10.0;
  static const card = 16.0;
  static const sheet = 28.0;
}

class AppFonts {
  static const display = 'Unbounded';
  static const mono = 'JetBrains Mono';
}

const accentPresets = <Color>[
  Color(0xFFB8F545), // лайм
  Color(0xFF45D8F5), // циан
  Color(0xFFF545C8), // маджента
  Color(0xFFF5A545), // янтарь
  Color(0xFF5A8BFF), // синий
  Color(0xFFFF6B5F), // коралл
];

/// Mirrors the web's `oklch(from var(--accent-raw) 82% 0.16 h)` clamp:
/// keep the picked hue, normalize lightness/saturation so black text and
/// glows stay legible no matter which color the user picks.
Color clampAccent(Color raw) {
  final hsl = HSLColor.fromColor(raw);
  return hsl.withLightness(0.62).withSaturation(0.85).toColor();
}
