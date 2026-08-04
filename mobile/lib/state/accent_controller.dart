import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../theme/tokens.dart';

const _prefsKey = 'mo_accent_argb';

/// Mirrors src/lib/theme.ts — same storage concept (persisted raw pick),
/// same clamp-for-legibility rule, applied here instead of via CSS.
class AccentController extends ChangeNotifier {
  Color _raw = AppColors.defaultAccent;
  Color get raw => _raw;
  Color get accent => clampAccent(_raw);

  Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    final stored = prefs.getInt(_prefsKey);
    if (stored != null) {
      _raw = Color(stored);
      notifyListeners();
    }
  }

  Future<void> choose(Color color) async {
    _raw = color;
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setInt(_prefsKey, color.toARGB32());
  }
}
