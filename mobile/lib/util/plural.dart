/// Русские числительные: «1 трек», «2 трека», «5 треков».
///
/// Формы 11–14 — исключение из правила по последней цифре, поэтому проверяются
/// первыми: «11 треков», а не «11 трек».
String plural(int n, String one, String few, String many) {
  final mod100 = n % 100;
  if (mod100 >= 11 && mod100 <= 14) return many;
  return switch (n % 10) {
    1 => one,
    2 || 3 || 4 => few,
    _ => many,
  };
}
