import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

const _keyToken = 'godrive_token';
const _keyBaseUrl = 'godrive_base_url';

const _secure = FlutterSecureStorage();

Future<void> saveSession(String baseUrl, String token) async {
  final prefs = await SharedPreferences.getInstance();
  await prefs.setString(_keyBaseUrl, baseUrl);
  await _secure.write(key: _keyToken, value: token);
}

Future<(String baseUrl, String token)?> loadSession() async {
  final prefs = await SharedPreferences.getInstance();
  final baseUrl = prefs.getString(_keyBaseUrl);
  if (baseUrl == null || baseUrl.isEmpty) return null;
  final token = await _secure.read(key: _keyToken);
  if (token == null || token.isEmpty) return null;
  return (baseUrl, token);
}

Future<void> clearSession() async {
  final prefs = await SharedPreferences.getInstance();
  await prefs.remove(_keyBaseUrl);
  await _secure.delete(key: _keyToken);
}
