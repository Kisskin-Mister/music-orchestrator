library;

/// Device-local storage for downloaded audio.
///
/// Web builds cannot use `dart:io`, so the real implementation lives in
/// offline_store_io.dart and the web build gets a stub that reports the
/// feature as unsupported. Callers check [OfflineStore.isSupported] before
/// offering "save to device" in the UI.
export 'offline_store_stub.dart'
    if (dart.library.io) 'offline_store_io.dart';
