import 'dart:convert';
import 'package:http/http.dart' as http;
import 'models.dart';

class ApiException implements Exception {
  final int statusCode;
  final String message;
  const ApiException(this.statusCode, this.message);
  @override
  String toString() => message;
}

class ApiClient {
  final String baseUrl;
  final String token;

  const ApiClient({required this.baseUrl, required this.token});

  Map<String, String> get _headers => {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Authorization': 'Bearer $token',
      };

  Uri _uri(String path, [Map<String, String>? params]) {
    final base = baseUrl.endsWith('/')
        ? baseUrl.substring(0, baseUrl.length - 1)
        : baseUrl;
    final uri = Uri.parse('$base$path');
    return params != null ? uri.replace(queryParameters: params) : uri;
  }

  Future<Map<String, dynamic>> _parseResponse(http.Response resp) async {
    if (resp.statusCode >= 200 && resp.statusCode < 300) {
      if (resp.body.isEmpty) return {};
      return jsonDecode(resp.body) as Map<String, dynamic>;
    }
    String message = 'Request failed';
    try {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      message = body['error'] as String? ?? message;
    } catch (_) {}
    throw ApiException(resp.statusCode, message);
  }

  Future<Map<String, dynamic>> _get(String path,
      [Map<String, String>? params]) async {
    final resp = await http.get(_uri(path, params), headers: _headers);
    return _parseResponse(resp);
  }

  Future<Map<String, dynamic>> _post(String path, Object body) async {
    final resp =
        await http.post(_uri(path), headers: _headers, body: jsonEncode(body));
    return _parseResponse(resp);
  }

  Future<Map<String, dynamic>> _patch(String path, Object body) async {
    final resp =
        await http.patch(_uri(path), headers: _headers, body: jsonEncode(body));
    return _parseResponse(resp);
  }

  Future<Map<String, dynamic>> _delete(String path,
      [Map<String, String>? params]) async {
    final resp = await http.delete(_uri(path, params), headers: _headers);
    return _parseResponse(resp);
  }

  // Auth
  static Future<(String token, User user)> login(
      String baseUrl, String username, String password) async {
    final resp = await http.post(
      Uri.parse(
          '${baseUrl.trimRight().replaceAll(RegExp(r'/$'), '')}/api/auth/login'),
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      body: jsonEncode({'username': username, 'password': password}),
    );
    if (resp.statusCode == 200) {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      return (
        body['token'] as String,
        User.fromJson(body['user'] as Map<String, dynamic>)
      );
    }
    String message = 'Invalid credentials';
    try {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      message = body['error'] as String? ?? message;
    } catch (_) {}
    throw ApiException(resp.statusCode, message);
  }

  Future<void> logout() async {
    await _post('/api/auth/logout', {});
  }

  Future<User> me() async {
    final body = await _get('/api/me');
    return User.fromJson(body['user'] as Map<String, dynamic>);
  }

  // Files
  Future<ListPage> listFiles(String path,
      {int offset = 0, int limit = 500, String? cursor}) async {
    final body = await _get('/api/files/list', {
      'path': path,
      'limit': '$limit',
      if (cursor != null && cursor.isNotEmpty)
        'cursor': cursor
      else
        'offset': '$offset',
    });
    return ListPage.fromJson(body);
  }

  Future<List<FileEntry>> searchFiles(String query, {int limit = 50}) async {
    final body =
        await _get('/api/files/search', {'q': query, 'limit': '$limit'});
    return (body['entries'] as List<dynamic>)
        .map((e) => FileEntry.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<FileEntry> mkdir(String path) async {
    final body = await _post('/api/files/mkdir', {'path': path});
    return FileEntry.fromJson(body['entry'] as Map<String, dynamic>);
  }

  Future<FileEntry> move(String from, String to) async {
    final body = await _post('/api/files/move', {'from': from, 'to': to});
    return FileEntry.fromJson(body['entry'] as Map<String, dynamic>);
  }

  Future<void> deleteFile(String path) async {
    await _delete('/api/files', {'path': path});
  }

  Future<TextPreview> textPreview(String path) async {
    final body = await _get('/api/files/text', {'path': path});
    return TextPreview.fromJson(body);
  }

  String downloadUrl(String path) =>
      '${_uri('/api/files/download', {'path': path})}';

  String thumbnailUrl(String path, int size) =>
      '${_uri('/api/files/thumbnail', {'path': path, 'size': '$size'})}';

  String rawFileUrl(String path) => '${_uri('/api/files/raw', {'path': path})}';

  Map<String, String> get authHeader => {'Authorization': 'Bearer $token'};

  // Bulk
  Future<void> bulkDelete(List<String> paths) async {
    await _post('/api/files/bulk/delete', {'paths': paths});
  }

  Future<void> bulkMove(List<String> paths, String targetDir) async {
    await _post(
        '/api/files/bulk/move', {'paths': paths, 'target_dir': targetDir});
  }

  // Trash
  Future<List<TrashItem>> listTrash() async {
    final body = await _get('/api/trash');
    return (body['items'] as List<dynamic>)
        .map((e) => TrashItem.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<FileEntry> restoreTrash(String id) async {
    final body = await _post('/api/trash/$id/restore', {});
    return FileEntry.fromJson(body['entry'] as Map<String, dynamic>);
  }

  Future<void> deleteTrash(String id) async {
    await _delete('/api/trash/$id');
  }

  // Admin
  Future<AdminStats> adminStats() async {
    final body = await _get('/api/admin/stats');
    return AdminStats.fromJson(body);
  }

  Future<List<User>> listAdminUsers() async {
    final body = await _get('/api/admin/users');
    return (body['users'] as List<dynamic>)
        .map((e) => User.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<User> createAdminUser({
    required String username,
    required String password,
    required String homeRoot,
    bool isAdmin = false,
  }) async {
    final body = await _post('/api/admin/users', {
      'username': username,
      'password': password,
      'home_root': homeRoot,
      'is_admin': isAdmin,
      'disabled': false,
    });
    return User.fromJson(body['user'] as Map<String, dynamic>);
  }

  Future<User> updateAdminUser(
    int id, {
    String? username,
    String? homeRoot,
    bool? isAdmin,
    bool? disabled,
  }) async {
    final body = await _patch('/api/admin/users/$id', {
      if (username != null) 'username': username,
      if (homeRoot != null) 'home_root': homeRoot,
      if (isAdmin != null) 'is_admin': isAdmin,
      if (disabled != null) 'disabled': disabled,
    });
    return User.fromJson(body['user'] as Map<String, dynamic>);
  }

  Future<void> setAdminUserPassword(int id, String password) async {
    await _post('/api/admin/users/$id/password', {'password': password});
  }

  Future<AdminJob> startReindex() async {
    final body = await _post('/api/admin/jobs/reindex', {});
    return AdminJob.fromJson(body['job'] as Map<String, dynamic>);
  }

  Future<AdminJob> startPreviewWarmup() async {
    final body = await _post('/api/admin/jobs/preview-warmup', {});
    return AdminJob.fromJson(body['job'] as Map<String, dynamic>);
  }

  Future<AdminJob?> currentAdminJob() async {
    final body = await _get('/api/admin/jobs/current');
    final job = body['job'];
    return job != null ? AdminJob.fromJson(job as Map<String, dynamic>) : null;
  }

  Future<void> clearPreviewCache() async {
    await _delete('/api/admin/preview-cache');
  }
}
