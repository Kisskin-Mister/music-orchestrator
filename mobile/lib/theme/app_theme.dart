import 'package:flutter/material.dart';
import 'tokens.dart';

ThemeData buildAppTheme(Color accent) {
  final accentInk = accent.computeLuminance() > 0.5
      ? const Color(0xFF10131A)
      : Colors.white;
  final scheme = ColorScheme.dark(
    surface: AppColors.bg,
    primary: accent,
    onPrimary: accentInk,
    secondary: AppColors.surface2,
    error: AppColors.danger,
  );
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: AppColors.bg,
    fontFamily: 'SF Pro Text',
    splashFactory: NoSplash.splashFactory,
    highlightColor: Colors.transparent,
    textTheme: const TextTheme(
      headlineLarge: TextStyle(
        fontFamily: AppFonts.display,
        fontWeight: FontWeight.w800,
        letterSpacing: -0.5,
        height: 1.0,
        color: AppColors.fg,
      ),
      headlineMedium: TextStyle(
        fontFamily: AppFonts.display,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.3,
        color: AppColors.fg,
      ),
      titleLarge: TextStyle(
        fontFamily: AppFonts.display,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.2,
        color: AppColors.fg,
      ),
      bodyMedium: TextStyle(color: AppColors.fg, fontSize: 15, height: 1.4),
      bodySmall: TextStyle(color: AppColors.muted, fontSize: 13),
      labelSmall: TextStyle(
        fontFamily: AppFonts.mono,
        color: AppColors.muted,
        fontSize: 11,
        letterSpacing: 1.4,
      ),
    ),
    iconTheme: const IconThemeData(color: AppColors.muted),
    dividerColor: AppColors.border,
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.surface,
      hintStyle: const TextStyle(color: AppColors.subtle),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.control),
        borderSide: const BorderSide(color: AppColors.borderStrong),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.control),
        borderSide: const BorderSide(color: AppColors.borderStrong),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.control),
        borderSide: BorderSide(color: accent.withValues(alpha: 0.6)),
      ),
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: accent,
        foregroundColor: accentInk,
        elevation: 0,
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.control),
        ),
        textStyle: const TextStyle(fontWeight: FontWeight.w600),
      ),
    ),
  );
}
