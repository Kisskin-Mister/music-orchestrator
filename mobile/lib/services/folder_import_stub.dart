import '../api/models.dart';

/// Веб-сборка: обойти папку из браузера нельзя, поэтому импорт папки скрыт,
/// а импорт отдельных файлов работает как везде.
const bool folderImportSupported = false;

bool isAudioFile(String name) => true;

Future<List<UploadFile>> collectFolder(String root) async => const [];
