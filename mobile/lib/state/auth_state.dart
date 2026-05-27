import 'package:flutter/foundation.dart';
import '../api/client.dart';
import '../api/models.dart';
import '../storage/session.dart';

class AuthState extends ChangeNotifier {
  ApiClient? _client;
  User? _user;
  bool _loading = true;

  ApiClient? get client => _client;
  User? get user => _user;
  bool get loading => _loading;
  bool get loggedIn => _client != null && _user != null;

  Future<void> init() async {
    try {
      final session = await loadSession();
      if (session != null) {
        final (baseUrl, token) = session;
        _client = ApiClient(baseUrl: baseUrl, token: token);
        try {
          _user = await _client!.me();
        } catch (_) {
          _client = null;
          try {
            await clearSession();
          } catch (_) {}
        }
      }
    } catch (e) {
      // Keystore / secure storage failure on first run — start fresh.
      _client = null;
      _user = null;
      try {
        await clearSession();
      } catch (_) {}
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  Future<void> login(String baseUrl, String username, String password) async {
    final (token, user) = await ApiClient.login(baseUrl, username, password);
    await saveSession(baseUrl, token);
    _client = ApiClient(baseUrl: baseUrl, token: token);
    _user = user;
    notifyListeners();
  }

  @visibleForTesting
  void setLoggedIn(ApiClient client, User user) {
    _client = client;
    _user = user;
    _loading = false;
    notifyListeners();
  }

  Future<void> logout() async {
    try {
      await _client?.logout();
    } catch (_) {}
    try {
      await clearSession();
    } catch (_) {}
    _client = null;
    _user = null;
    notifyListeners();
  }
}
