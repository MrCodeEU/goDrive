import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:godrive/api/client.dart';
import 'package:godrive/api/tus.dart';

void main() {
  const baseUrl = 'https://drive.example.com';

  TusClient tusClient(MockClientHandler handler) => TusClient(
        ApiClient(baseUrl: baseUrl, token: 'tok'),
        httpClient: MockClient(handler),
      );

  // Helpers ---------------------------------------------------------------

  late Directory tmpDir;

  setUp(() async {
    tmpDir = await Directory.systemTemp.createTemp('tus_test_');
  });

  tearDown(() async {
    await tmpDir.delete(recursive: true);
  });

  Future<File> tmpFile(String name, String content) async {
    final f = File('${tmpDir.path}/$name');
    await f.writeAsString(content);
    return f;
  }

  // create ----------------------------------------------------------------

  group('create', () {
    test('POST returns Location header as tusUrl', () async {
      final tus = tusClient((req) async {
        expect(req.method, 'POST');
        expect(req.url.path, '/api/tus');
        expect(req.url.queryParameters['path'], '/uploads');
        expect(req.headers['Tus-Resumable'], '1.0.0');
        expect(req.headers['Upload-Length'], '13');
        return http.Response('', 201, headers: {'location': '/api/tus/abc123'});
      });

      final url = await tus.create('/uploads', 'hello.txt', 13);
      expect(url, '/api/tus/abc123');
    });

    test('non-201 response throws ApiException', () async {
      final tus = tusClient((_) async => http.Response(
          jsonEncode({'error': 'quota exceeded'}), 413,
          headers: {'content-type': 'application/json'}));

      expect(
        () => tus.create('/uploads', 'big.bin', 999999999),
        throwsA(
            isA<ApiException>().having((e) => e.statusCode, 'statusCode', 413)),
      );
    });

    test('missing Location header throws ApiException', () async {
      final tus = tusClient((_) async => http.Response('', 201));
      expect(
        () => tus.create('/uploads', 'file.txt', 5),
        throwsA(isA<ApiException>()),
      );
    });
  });

  // getOffset -------------------------------------------------------------

  group('getOffset', () {
    test('HEAD returns Upload-Offset', () async {
      final tus = tusClient((req) async {
        expect(req.method, 'HEAD');
        return http.Response('', 204, headers: {'upload-offset': '1024'});
      });

      final offset = await tus.getOffset('/api/tus/abc123');
      expect(offset, 1024);
    });

    test('404 response throws upload_gone ApiException', () async {
      final tus = tusClient((_) async => http.Response('', 404));
      expect(
        () => tus.getOffset('/api/tus/gone'),
        throwsA(isA<ApiException>()
            .having((e) => e.message, 'message', 'upload_gone')),
      );
    });

    test('missing offset header defaults to 0', () async {
      final tus = tusClient((_) async => http.Response('', 204));
      final offset = await tus.getOffset('/api/tus/abc123');
      expect(offset, 0);
    });
  });

  // patch -----------------------------------------------------------------

  group('patch', () {
    test('PATCH sends correct headers and returns final path', () async {
      final file = await tmpFile('hello.txt', 'hello world!');
      final fileSize = await file.length();

      final tus = tusClient((req) async {
        expect(req.method, 'PATCH');
        expect(req.headers['Content-Type'], 'application/offset+octet-stream');
        expect(req.headers['Upload-Offset'], '0');
        expect(req.headers['Tus-Resumable'], '1.0.0');
        return http.Response('', 204,
            headers: {'upload-final-path': '/uploads/hello.txt'});
      });

      final path = await tus.patch('/api/tus/abc123', file, 0, fileSize);
      expect(path, '/uploads/hello.txt');
    });

    test('resume from offset sends correct Upload-Offset header', () async {
      final file = await tmpFile('resume.txt', 'hello world!');
      final fileSize = await file.length();
      const offset = 6;

      final tus = tusClient((req) async {
        expect(req.headers['Upload-Offset'], '$offset');
        return http.Response('', 204,
            headers: {'upload-final-path': '/uploads/resume.txt'});
      });

      final path = await tus.patch('/api/tus/abc123', file, offset, fileSize);
      expect(path, '/uploads/resume.txt');
    });

    test('server error throws ApiException', () async {
      final file = await tmpFile('fail.txt', 'data');
      final tus = tusClient((_) async => http.Response(
          jsonEncode({'error': 'server error'}), 500,
          headers: {'content-type': 'application/json'}));

      expect(
        () => tus.patch('/api/tus/abc123', file, 0, 4),
        throwsA(
            isA<ApiException>().having((e) => e.statusCode, 'statusCode', 500)),
      );
    });
  });

  // upload (create + patch) -----------------------------------------------

  group('upload', () {
    test('creates upload then patches file', () async {
      final file = await tmpFile('doc.txt', 'hello');
      int callCount = 0;

      final tus = tusClient((req) async {
        callCount++;
        if (req.method == 'POST') {
          return http.Response('', 201,
              headers: {'location': '/api/tus/new123'});
        }
        // PATCH
        return http.Response('', 204,
            headers: {'upload-final-path': '/uploads/doc.txt'});
      });

      final path = await tus.upload(file, '/uploads');
      expect(path, '/uploads/doc.txt');
      expect(callCount, 2);
    });
  });

  // resume ----------------------------------------------------------------

  group('resume', () {
    test('resumes from existing offset', () async {
      final file = await tmpFile('partial.txt', 'hello world');
      const existingOffset = 5;

      final tus = tusClient((req) async {
        if (req.method == 'HEAD') {
          return http.Response('', 204,
              headers: {'upload-offset': '$existingOffset'});
        }
        expect(req.headers['Upload-Offset'], '$existingOffset');
        return http.Response('', 204,
            headers: {'upload-final-path': '/uploads/partial.txt'});
      });

      final path = await tus.resume('/api/tus/abc123', file);
      expect(path, '/uploads/partial.txt');
    });

    test('returns null when already complete (offset >= fileSize)', () async {
      final file = await tmpFile('done.txt', 'hello');
      final fileSize = await file.length();

      final tus = tusClient((_) async =>
          http.Response('', 204, headers: {'upload-offset': '$fileSize'}));

      final path = await tus.resume('/api/tus/abc123', file);
      expect(path, isNull);
    });

    test('upload_gone on HEAD creates new upload', () async {
      final file = await tmpFile('retry.txt', 'data');
      int headCount = 0;

      final tus = tusClient((req) async {
        if (req.method == 'HEAD') {
          headCount++;
          return http.Response('', 404);
        }
        // After upload_gone, caller should do a fresh upload.
        // resume() itself doesn't create a new upload — it throws.
        return http.Response('', 204);
      });

      await expectLater(
        tus.resume('/api/tus/gone', file),
        throwsA(isA<ApiException>()
            .having((e) => e.message, 'message', 'upload_gone')),
      );
      expect(headCount, 1);
    });
  });
}
