import 'dart:io';
import 'package:path_provider/path_provider.dart';

/// Writes downloaded audio into the app's documents directory.
///
/// Files land in `<documents>/offline/`, which on iOS is inside the app
/// sandbox and is included in device backups — the user's own copy of a track
/// survives app updates and is removed when they delete the app.
class OfflineStore {
  static bool get isSupported => true;

  Directory? _dir;

  Future<Directory> _ensureDir() async {
    if (_dir != null) return _dir!;
    final base = await getApplicationDocumentsDirectory();
    final dir = Directory('${base.path}/offline');
    if (!await dir.exists()) await dir.create(recursive: true);
    _dir = dir;
    return dir;
  }

  Future<String?> pathFor(String fileName) async {
    final dir = await _ensureDir();
    final file = File('${dir.path}/$fileName');
    return await file.exists() ? file.path : null;
  }

  /// Streams to a `.part` file and renames on completion, so an interrupted
  /// download can never be mistaken for a complete offline copy.
  Future<String> save(String fileName, Stream<List<int>> bytes, {void Function(int received, int? total)? onProgress}) async {
    final dir = await _ensureDir();
    final target = File('${dir.path}/$fileName');
    final partial = File('${target.path}.part');
    final sink = partial.openWrite();
    try {
      await for (final chunk in bytes) {
        sink.add(chunk);
      }
      await sink.flush();
    } finally {
      await sink.close();
    }
    await partial.rename(target.path);
    return target.path;
  }

  Future<void> delete(String fileName) async {
    final dir = await _ensureDir();
    for (final path in ['${dir.path}/$fileName', '${dir.path}/$fileName.part']) {
      final file = File(path);
      if (await file.exists()) await file.delete();
    }
  }

  Future<int> totalBytes() async {
    final dir = await _ensureDir();
    var total = 0;
    await for (final entity in dir.list()) {
      if (entity is File) total += await entity.length();
    }
    return total;
  }
}
