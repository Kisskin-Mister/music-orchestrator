import 'dart:async';
import 'package:flutter/foundation.dart';
import '../api/api_client.dart';
import '../api/models.dart';

enum AuthStatus {
  loading,
  needsSetup,
  needsLogin,
  authenticated,

  /// Server unreachable, but this device has a stored session and possibly
  /// downloaded tracks — the app opens read-only instead of blocking on login.
  offline,
  backendUnreachable,
}

/// Mirrors App.tsx's session query + LoginScreen.tsx + the account/users part
/// of SettingsView — session-cookie auth, since /v1/account and /v1/users
/// require a real admin session (see the note in api_client.dart).
class AuthController extends ChangeNotifier {
  AuthController(this._api);
  final ApiClient _api;

  AuthStatus status = AuthStatus.loading;
  SessionInfo? session;
  String? error;
  List<AppUser> users = [];

  Future<void> refresh() async {
    status = AuthStatus.loading;
    notifyListeners();
    try {
      final info = await _api.session();
      session = info;
      if (info.authenticated) {
        status = AuthStatus.authenticated;
        if (info.isAdmin) unawaited(refreshUsers());
      } else {
        status = info.setupRequired
            ? AuthStatus.needsSetup
            : AuthStatus.needsLogin;
      }
    } catch (e) {
      status = _api.hasStoredSession ? AuthStatus.offline : AuthStatus.backendUnreachable;
      error = '$e';
    }
    notifyListeners();
  }

  Future<bool> register(String username, String password) =>
      _run(() => _api.register(username, password));
  Future<bool> login(String username, String password) =>
      _run(() => _api.login(username, password));

  Future<bool> _run(Future<SessionInfo> Function() action) async {
    error = null;
    try {
      session = await action();
      status = AuthStatus.authenticated;
      notifyListeners();
      if (session!.isAdmin) unawaited(refreshUsers());
      return true;
    } catch (e) {
      error = '$e';
      notifyListeners();
      return false;
    }
  }

  Future<void> logout() async {
    await _api.logout();
    session = null;
    users = [];
    status = AuthStatus.needsLogin;
    notifyListeners();
  }

  Future<bool> updateAccount({
    required String username,
    String? password,
  }) async {
    try {
      await _api.updateAccount(username: username, password: password);
      await refresh();
      return true;
    } catch (e) {
      error = '$e';
      notifyListeners();
      return false;
    }
  }

  Future<void> refreshUsers() async {
    try {
      users = await _api.users();
      notifyListeners();
    } catch (_) {
      // Non-admin or backend without multi-user support — leave the list empty.
    }
  }

  Future<bool> createUser(String username, String password) async {
    try {
      await _api.createUser(username, password);
      await refreshUsers();
      return true;
    } catch (e) {
      error = '$e';
      notifyListeners();
      return false;
    }
  }

  Future<void> deleteUser(String userId) async {
    await _api.deleteUser(userId);
    await refreshUsers();
  }
}
