import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:godrive/api/client.dart';

void main() {
  const baseUrl = 'https://drive.example.com';

  // Helpers ---------------------------------------------------------------

  ApiClient client(MockClientHandler handler, {String token = 'tok'}) =>
      ApiClient(
        baseUrl: baseUrl,
        token: token,
        httpClient: MockClient(handler),
      );

  http.Response jsonResp(Object body, {int status = 200}) =>
      http.Response(jsonEncode(body), status,
          headers: {'content-type': 'application/json'});

  Map<String, dynamic> userJson({int id = 1, bool isAdmin = false}) => {
        'id': id,
        'username': 'alice',
        'is_admin': isAdmin,
        'disabled': false,
        'home_root': '/data/alice',
      };

  // Login -----------------------------------------------------------------

  group('login', () {
    test('success returns token and user', () async {
      final (token, user) = await ApiClient.login(
        baseUrl,
        'alice',
        'secret',
        httpClient: MockClient((_) async => jsonResp({
              'token': 'abc123',
              'user': userJson(),
            })),
      );
      expect(token, 'abc123');
      expect(user.username, 'alice');
      expect(user.isAdmin, false);
    });

    test('wrong credentials throws ApiException 401', () async {
      expect(
        () => ApiClient.login(
          baseUrl,
          'alice',
          'wrong',
          httpClient: MockClient((_) async =>
              jsonResp({'error': 'Invalid credentials'}, status: 401)),
        ),
        throwsA(isA<ApiException>()
            .having((e) => e.statusCode, 'statusCode', 401)
            .having((e) => e.message, 'message', 'Invalid credentials')),
      );
    });

    test('network error propagates', () async {
      expect(
        () => ApiClient.login(
          baseUrl,
          'alice',
          'secret',
          httpClient: MockClient(
              (_) async => throw http.ClientException('unreachable')),
        ),
        throwsA(isA<http.ClientException>()),
      );
    });
  });

  // Browse ----------------------------------------------------------------

  group('listFiles', () {
    test('parses entries and metadata', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((_) async => jsonResp({
            'path': '/docs',
            'entries': [
              {
                'name': 'report.pdf',
                'path': '/docs/report.pdf',
                'type': 'file',
                'size': 1024,
                'modified_at': now,
                'mime_type': 'application/pdf',
                'preview_kind': 'pdf',
              },
              {
                'name': 'images',
                'path': '/docs/images',
                'type': 'dir',
                'size': 0,
                'modified_at': now,
              },
            ],
            'total': 2,
            'offset': 0,
            'limit': 500,
            'has_more': false,
          }));

      final page = await api.listFiles('/docs');
      expect(page.path, '/docs');
      expect(page.entries.length, 2);
      expect(page.entries[0].name, 'report.pdf');
      expect(page.entries[0].isDir, false);
      expect(page.entries[0].previewKind, 'pdf');
      expect(page.entries[1].isDir, true);
      expect(page.total, 2);
      expect(page.hasMore, false);
    });

    test('empty directory returns empty entries', () async {
      final api = client((_) async => jsonResp({
            'path': '/empty',
            'entries': [],
            'total': 0,
            'offset': 0,
            'limit': 500,
            'has_more': false,
          }));

      final page = await api.listFiles('/empty');
      expect(page.entries, isEmpty);
    });

    test('server error throws ApiException', () async {
      final api =
          client((_) async => jsonResp({'error': 'Not found'}, status: 404));
      expect(
        () => api.listFiles('/missing'),
        throwsA(
            isA<ApiException>().having((e) => e.statusCode, 'statusCode', 404)),
      );
    });
  });

  group('searchFiles', () {
    test('returns matching entries', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((req) async {
        expect(req.url.queryParameters['q'], 'report');
        return jsonResp({
          'entries': [
            {
              'name': 'report.pdf',
              'path': '/docs/report.pdf',
              'type': 'file',
              'size': 512,
              'modified_at': now,
            }
          ]
        });
      });

      final results = await api.searchFiles('report');
      expect(results.length, 1);
      expect(results[0].name, 'report.pdf');
    });

    test('empty query returns empty list', () async {
      final api = client((_) async => jsonResp({'entries': []}));
      final results = await api.searchFiles('');
      expect(results, isEmpty);
    });
  });

  // Text / image / video preview ------------------------------------------

  group('text preview', () {
    test('parses text preview response', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((req) async {
        expect(req.url.queryParameters['path'], '/notes/todo.md');
        return jsonResp({
          'path': '/notes/todo.md',
          'name': 'todo.md',
          'content': '# Todo\n- item one',
          'truncated': false,
          'size': 18,
          'max_bytes': 262144,
          'mime_type': 'text/markdown',
          'modified_at': now,
        });
      });

      final preview = await api.textPreview('/notes/todo.md');
      expect(preview.name, 'todo.md');
      expect(preview.content, contains('item one'));
      expect(preview.truncated, false);
      expect(preview.mimeType, 'text/markdown');
    });

    test('truncated flag set for large files', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((_) async => jsonResp({
            'path': '/big.log',
            'name': 'big.log',
            'content': 'first 256kb...',
            'truncated': true,
            'size': 1048576,
            'max_bytes': 262144,
            'mime_type': 'text/plain',
            'modified_at': now,
          }));

      final preview = await api.textPreview('/big.log');
      expect(preview.truncated, true);
    });
  });

  group('URL helpers', () {
    test('thumbnailUrl encodes path and size', () {
      final api = ApiClient(baseUrl: baseUrl, token: 'tok');
      final url = api.thumbnailUrl('/photos/img.jpg', 256);
      expect(url, contains('/api/files/thumbnail'));
      expect(url, contains('size=256'));
    });

    test('downloadUrl encodes path', () {
      final api = ApiClient(baseUrl: baseUrl, token: 'tok');
      final url = api.downloadUrl('/docs/file.pdf');
      expect(url, contains('/api/files/download'));
      expect(url, contains('file.pdf'));
    });

    test('rawFileUrl encodes path', () {
      final api = ApiClient(baseUrl: baseUrl, token: 'tok');
      final url = api.rawFileUrl('/photos/img.jpg');
      expect(url, contains('/api/files/raw'));
    });
  });

  // Trash -----------------------------------------------------------------

  group('trash', () {
    test('listTrash parses items', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((_) async => jsonResp({
            'items': [
              {
                'id': 'trash-1',
                'user_id': 1,
                'original_path': '/docs/old.txt',
                'original_name': 'old.txt',
                'is_dir': false,
                'size': 100,
                'deleted_at': now,
              }
            ]
          }));

      final items = await api.listTrash();
      expect(items.length, 1);
      expect(items[0].id, 'trash-1');
      expect(items[0].originalName, 'old.txt');
    });

    test('restoreTrash returns restored entry', () async {
      final now = DateTime.now().toUtc().toIso8601String();
      final api = client((req) async {
        expect(req.url.path, contains('/api/trash/trash-1/restore'));
        return jsonResp({
          'entry': {
            'name': 'old.txt',
            'path': '/docs/old.txt',
            'type': 'file',
            'size': 100,
            'modified_at': now,
          }
        });
      });

      final entry = await api.restoreTrash('trash-1');
      expect(entry.name, 'old.txt');
      expect(entry.path, '/docs/old.txt');
    });

    test('deleteTrash sends DELETE to correct path', () async {
      bool called = false;
      final api = client((req) async {
        expect(req.method, 'DELETE');
        expect(req.url.path, contains('/api/trash/trash-1'));
        called = true;
        return http.Response('', 204);
      });

      await api.deleteTrash('trash-1');
      expect(called, true);
    });
  });

  // Admin smoke -----------------------------------------------------------

  group('adminStats', () {
    test('parses stats response', () async {
      final api = client((_) async => jsonResp({
            'index': {
              'indexed_files': 42,
              'indexed_directories': 5,
              'indexed_bytes': 1024
            },
            'users': {'total': 2, 'disabled': 0},
            'trash': {'items': 1, 'bytes': 512},
            'preview_cache': {'files': 10, 'bytes': 2048},
            'preview': {
              'workers': 4,
              'sizes': [256, 512],
              'tools': []
            },
            'watcher': {'enabled': true, 'roots': 2},
            'reconciliation': {'enabled': true, 'interval': '5m'},
          }));

      final stats = await api.adminStats();
      expect(stats.indexedFiles, 42);
      expect(stats.totalUsers, 2);
      expect(stats.watcherEnabled, true);
    });
  });
}
