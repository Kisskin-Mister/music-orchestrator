/// Web build: the browser owns the filesystem, so there is no app-managed
/// offline copy. The UI hides "save to device" when [isSupported] is false
/// instead of showing a control that cannot work.
class OfflineStore {
  static bool get isSupported => false;

  Future<String?> pathFor(String fileName) async => null;

  Future<String> save(String fileName, Stream<List<int>> bytes, {void Function(int received, int? total)? onProgress}) {
    throw UnsupportedError('Offline downloads are not available in the browser');
  }

  Future<void> delete(String fileName) async {}

  Future<int> totalBytes() async => 0;
}
