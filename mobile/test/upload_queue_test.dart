import 'dart:convert';
import 'dart:io';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:godrive/api/client.dart';
import 'package:godrive/api/tus.dart';
import 'package:godrive/state/upload_queue.dart';

// Fake TusClient — controls upload outcomes without HTTP.
class _FakeTus extends TusClient {
  final Future<String?> Function(File file, String targetPath)? onUpload;
  final Future<String?> Function(String tusUrl, File file)? onResume;
  final Exception? throwOn;

  _FakeTus({this.onUpload, this.onResume, this.throwOn})
      : super(ApiClient(baseUrl: 'http://localhost', token: 'tok'));

  @override
  Future<String?> upload(
    File file,
    String targetPath, {
    TusProgressCallback? onProgress,
    void Function(String tusUrl)? onCreated,
  }) async {
    if (throwOn != null) throw throwOn!;
    return onUpload?.call(file, targetPath) ??
        '$targetPath/${file.path.split('/').last}';
  }

  @override
  Future<String?> resume(
    String tusUrl,
    File file, {
    TusProgressCallback? onProgress,
  }) async {
    if (throwOn != null) throw throwOn!;
    return onResume?.call(tusUrl, file) ?? file.path.split('/').last;
  }
}

void main() {
  setUpAll(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    // Wakelock uses a Pigeon BasicMessageChannel. Return [null] = void success.
    const codec = StandardMessageCodec();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMessageHandler(
      'dev.flutter.pigeon.wakelock_plus_platform_interface.WakelockPlusApi.toggle',
      (_) async => codec.encodeMessage([null]),
    );
  });

  late Directory tmpDir;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    tmpDir = await Directory.systemTemp.createTemp('queue_test_');
  });

  tearDown(() async {
    await tmpDir.delete(recursive: true);
  });

  Future<File> tmpFile(String name, [String content = 'data']) async {
    final f = File('${tmpDir.path}/$name');
    await f.writeAsString(content);
    return f;
  }

  // Enqueue ---------------------------------------------------------------

  group('enqueue', () {
    test('adds items to queue with queued status', () async {
      final queue = UploadQueue(retryDelays: []);
      final files = [await tmpFile('a.txt'), await tmpFile('b.txt')];

      queue.enqueue(files, '/uploads');

      expect(queue.items.length, 2);
      expect(queue.items.every((i) => i.status == UploadStatus.queued), true);
      expect(queue.hasActive, true);
    });

    test('items get correct names and paths', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('photo.jpg');

      queue.enqueue([file], '/photos');

      expect(queue.items.first.name, 'photo.jpg');
      expect(queue.items.first.targetPath, '/photos');
    });
  });

  // processQueue ----------------------------------------------------------

  group('processQueue', () {
    test('successful upload marks item done', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('doc.txt');
      queue.enqueue([file], '/docs');

      final tus = _FakeTus();
      await queue.processQueue(tus);

      expect(queue.items.first.status, UploadStatus.done);
      expect(queue.items.first.progress, 1.0);
    });

    test('failed upload marks item error after retries', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('fail.txt');
      queue.enqueue([file], '/docs');

      final tus = _FakeTus(throwOn: const ApiException(500, 'server error'));
      await queue.processQueue(tus);

      final item = queue.items.first;
      expect(item.status, UploadStatus.error);
      expect(item.error, contains('server error'));
    });

    test('skips item when file is gone', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('gone.txt');
      queue.enqueue([file], '/docs');
      // Delete the file before processing
      await file.delete();

      final tus = _FakeTus();
      await queue.processQueue(tus);

      final item = queue.items.first;
      expect(item.status, UploadStatus.interrupted);
      expect(item.error, contains('no longer available'));
    });

    test('upload_gone retries as fresh upload', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('resume.txt');
      // Access private _items via processQueue — enqueue manually via public API instead
      queue.enqueue([file], '/docs');
      // Patch tusUrl onto the enqueued item
      queue.items.first.tusUrl = '/api/tus/stale';

      bool resumeCalled = false;
      bool uploadCalled = false;
      final tus = _FakeTus(
        onResume: (tusUrl, f) async {
          resumeCalled = true;
          throw const ApiException(404, 'upload_gone');
        },
        onUpload: (f, path) async {
          uploadCalled = true;
          return '$path/resume.txt';
        },
      );

      await queue.processQueue(tus);

      expect(resumeCalled, true);
      expect(uploadCalled, true);
      expect(queue.items.first.status, UploadStatus.done);
    });

    test('processes multiple items concurrently', () async {
      final queue = UploadQueue(retryDelays: []);
      final files = [
        await tmpFile('f1.txt'),
        await tmpFile('f2.txt'),
        await tmpFile('f3.txt'),
        await tmpFile('f4.txt'),
      ];
      queue.enqueue(files, '/docs');

      final tus = _FakeTus();
      await queue.processQueue(tus);

      expect(queue.items.where((i) => i.status == UploadStatus.done).length, 4);
    });

    test('second processQueue call is no-op while first runs', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('slow.txt');
      queue.enqueue([file], '/docs');

      final tus = _FakeTus();
      // Start two concurrent calls — only the first should run
      await Future.wait([
        queue.processQueue(tus),
        queue.processQueue(tus),
      ]);

      expect(queue.items.length, 1);
      expect(queue.items.first.status, UploadStatus.done);
    });
  });

  // Persistence -----------------------------------------------------------

  group('persist and restore', () {
    test('persists done items and restores them', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('save.txt');
      queue.enqueue([file], '/docs');

      final tus = _FakeTus();
      await queue.processQueue(tus);

      // Restore in a fresh queue
      final queue2 = UploadQueue();
      await queue2.refreshPersisted();

      expect(queue2.items.length, 1);
      expect(queue2.items.first.status, UploadStatus.done);
      expect(queue2.items.first.name, 'save.txt');

      final prefs = await SharedPreferences.getInstance();
      final raw = prefs.getString('godrive_upload_queue');
      final stored = jsonDecode(raw!) as Map<String, dynamic>;
      expect(stored['version'], 1);
      expect(stored['items'], isA<List<dynamic>>());
    });

    test('restores legacy array queue payloads', () async {
      final file = await tmpFile('legacy.txt');
      SharedPreferences.setMockInitialValues({
        'godrive_upload_queue': jsonEncode([
          {
            'id': 'legacy-1',
            'file_path': file.path,
            'name': 'legacy.txt',
            'file_size': await file.length(),
            'target_path': '/docs',
            'status': 'done',
            'progress': 1.0,
            'final_path': '/docs/legacy.txt',
          }
        ]),
      });

      final queue = UploadQueue();
      await queue.refreshPersisted();

      expect(queue.items.length, 1);
      expect(queue.items.first.id, 'legacy-1');
      expect(queue.items.first.status, UploadStatus.done);
      expect(queue.items.first.finalPath, '/docs/legacy.txt');
    });

    test('queued-only items are not persisted', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('unsaved.txt');
      queue.enqueue([file], '/docs');
      // Don't call processQueue — item stays queued

      final queue2 = UploadQueue();
      await queue2.refreshPersisted();

      expect(queue2.items, isEmpty);
    });

    test('error items are persisted with error message', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('err.txt');
      queue.enqueue([file], '/docs');

      final tus = _FakeTus(throwOn: const ApiException(503, 'unavailable'));
      await queue.processQueue(tus);

      final queue2 = UploadQueue();
      await queue2.refreshPersisted();

      expect(queue2.items.first.status, UploadStatus.error);
      expect(queue2.items.first.error, contains('unavailable'));
    });
  });

  // clearCompleted --------------------------------------------------------

  group('clearCompleted', () {
    test('removes done and interrupted items', () async {
      final queue = UploadQueue(retryDelays: []);
      final files = [await tmpFile('done.txt'), await tmpFile('active.txt')];
      queue.enqueue(files, '/docs');

      final tus = _FakeTus(
        onUpload: (f, path) async {
          if (f.path.contains('done.txt')) return '$path/done.txt';
          throw const ApiException(503, 'fail');
        },
      );
      await queue.processQueue(tus);

      // one done, one error
      queue.clearCompleted();

      // error items are NOT cleared by clearCompleted (only done + interrupted)
      expect(queue.items.where((i) => i.status == UploadStatus.done), isEmpty);
    });
  });

  // Retry -----------------------------------------------------------------

  group('retry', () {
    test('retries an error item successfully', () async {
      final queue = UploadQueue(retryDelays: []);
      final file = await tmpFile('retry.txt');
      queue.enqueue([file], '/docs');

      // First attempt fails
      await queue.processQueue(
          _FakeTus(throwOn: const ApiException(500, 'server error')));
      expect(queue.items.first.status, UploadStatus.error);

      // Retry succeeds
      await queue.retry(queue.items.first, _FakeTus());
      expect(queue.items.first.status, UploadStatus.done);
    });
  });
}
