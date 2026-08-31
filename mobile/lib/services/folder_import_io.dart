import 'dart:io';
import 'package:path/path.dart' as p;
import '../api/models.dart';

const bool folderImportSupported = true;

/// Расширения, которые сервер вообще готов принять (см. importableExtensions
/// в import.go). Отсеиваем их здесь же: в папке с музыкой обычно лежат ещё и
/// обложки, тексты и служебные файлы, и гонять их по сети незачем.
const _audioExtensions = {
  '.mp3',
  '.m4a',
  '.m4b',
  '.aac',
  '.flac',
  '.ogg',
  '.opus',
  '.wav',
  '.wma',
  '.aiff',
  '.aif',
  '.alac',
  '.m4p',
};

bool isAudioFile(String name) =>
    _audioExtensions.contains(p.extension(name).toLowerCase());

/// Рекурсивно собирает аудиофайлы из папки.
///
/// Имя файла отправляется вместе с относительным путём — так сервер видит,
/// из какой папки трек пришёл, а разбирается с именем он сам (safeUploadName).
Future<List<UploadFile>> collectFolder(String root) async {
  final files = <UploadFile>[];
  final dir = Directory(root);
  if (!dir.existsSync()) return files;
  // followLinks: false — симлинк на родительскую папку иначе уводит обход в
  // бесконечный цикл.
  await for (final entity in dir.list(recursive: true, followLinks: false)) {
    if (entity is! File) continue;
    final name = p.basename(entity.path);
    if (name.startsWith('.') || !isAudioFile(name)) continue;
    files.add(
      UploadFile(
        name: p.relative(entity.path, from: root),
        path: entity.path,
      ),
    );
  }
  files.sort((a, b) => a.name.compareTo(b.name));
  return files;
}
