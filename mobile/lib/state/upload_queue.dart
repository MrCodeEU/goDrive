import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:wakelock_plus/wakelock_plus.dart';
import '../api/background_uploader.dart';
import '../api/client.dart';
import '../api/tus.dart';

const _prefKey = 'godrive_upload_queue';
const _maxConcurrent = 3;

enum UploadStatus { queued, uploading, background, done, error, interrupted }

class UploadItem {
  final String id;
  final File? file;
  final String? filePath;
  final String name;
  final int fileSize;
  final String targetPath;
  UploadStatus status;
  double progress;
  String? error;
  String? finalPath;
  String? tusUrl;

  UploadItem({
    required this.id,
    required this.file,
    this.filePath,
    required this.name,
    required this.fileSize,
    required this.targetPath,
    this.status = UploadStatus.queued,
    this.progress = 0,
    this.error,
    this.finalPath,
    this.tusUrl,
  });

  // JSON for persistence. The stored file path can be reopened only while the
  // platform keeps that picker path valid.
  Map<String, dynamic> toJson() => {
        'id': id,
        'file_path': filePath ?? file?.path,
        'name': name,
        'file_size': fileSize,
        'target_path': targetPath,
        'status': switch (status) {
          UploadStatus.done => 'done',
          UploadStatus.error => 'error',
          UploadStatus.interrupted => 'interrupted',
          UploadStatus.background => 'background',
          _ => 'interrupted',
        },
        'progress': progress,
        'final_path': finalPath,
        'tus_url': tusUrl,
        'error': error,
      };

  factory UploadItem.fromJson(Map<String, dynamic> j) {
    final filePath = j['file_path'] as String?;
    final file =
        filePath != null && File(filePath).existsSync() ? File(filePath) : null;
    final status = switch (j['status'] as String?) {
      'done' => UploadStatus.done,
      'error' => UploadStatus.error,
      'background' => UploadStatus.background,
      _ => UploadStatus.interrupted,
    };
    return UploadItem(
      id: j['id'] as String,
      file: file,
      filePath: filePath,
      name: j['name'] as String,
      fileSize: j['file_size'] as int,
      targetPath: j['target_path'] as String,
      status: status,
      finalPath: j['final_path'] as String?,
      tusUrl: j['tus_url'] as String?,
      error: j['error'] as String?,
      progress: (j['progress'] as num?)?.toDouble() ??
          (status == UploadStatus.done ? 1.0 : 0),
    );
  }
}

class UploadQueue extends ChangeNotifier {
  final List<UploadItem> _items = [];
  bool _running = false;

  List<UploadItem> get items => List.unmodifiable(_items);

  int get activeCount => _items
      .where((i) =>
          i.status == UploadStatus.queued ||
          i.status == UploadStatus.uploading ||
          i.status == UploadStatus.background)
      .length;

  bool get hasActive => activeCount > 0;

  Future<void> init() async {
    try {
      await const BackgroundUploader().refresh();
    } catch (_) {
      // Native background reconciliation is best-effort.
    }
    await refreshPersisted();
  }

  Future<void> refreshPersisted() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final raw = prefs.getString(_prefKey);
      if (raw != null) {
        try {
          final list = jsonDecode(raw) as List<dynamic>;
          final restored = <UploadItem>[];
          for (final j in list) {
            restored.add(UploadItem.fromJson(j as Map<String, dynamic>));
          }
          _mergePersisted(restored);
          notifyListeners();
        } catch (_) {}
      }
    } catch (_) {
      // SharedPreferences unavailable — start with empty queue.
    }
  }

  void _mergePersisted(List<UploadItem> persisted) {
    final byID = {for (final item in _items) item.id: item};
    for (final restored in persisted) {
      final current = byID[restored.id];
      if (current == null) {
        _items.add(restored);
        continue;
      }
      current.status = restored.status;
      current.progress = restored.progress;
      current.error = restored.error;
      current.finalPath = restored.finalPath;
      current.tusUrl = restored.tusUrl;
    }
  }

  Future<void> _persist() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final persisted = _items.where((i) => i.status != UploadStatus.queued);
      if (persisted.isEmpty) {
        await prefs.remove(_prefKey);
      } else {
        await prefs.setString(
            _prefKey, jsonEncode(persisted.map((i) => i.toJson()).toList()));
      }
    } catch (_) {
      // Persistence failure must not interrupt active uploads.
    }
  }

  void enqueue(List<File> files, String targetPath) {
    for (final file in files) {
      _items.insert(
          0,
          UploadItem(
            id: '${DateTime.now().microsecondsSinceEpoch}_${file.path.hashCode}',
            file: file,
            filePath: file.path,
            name: file.path.split('/').last,
            fileSize: file.lengthSync(),
            targetPath: targetPath,
          ));
    }
    notifyListeners();
  }

  Future<void> processQueue(TusClient tus,
      {void Function(String path)? onComplete}) async {
    if (_running) return;
    _running = true;

    final queued =
        _items.where((i) => i.status == UploadStatus.queued).toList();
    if (queued.isEmpty) {
      _running = false;
      return;
    }
    var idx = 0;

    try {
      await WakelockPlus.enable();

      Future<void> worker() async {
        while (idx < queued.length) {
          final item = queued[idx++];
          if (item.file == null) {
            item.status = UploadStatus.interrupted;
            item.error = 'File is no longer available on this device';
            notifyListeners();
            await _persist();
            continue;
          }
          await _uploadOne(item, tus, onComplete: onComplete);
        }
      }

      final workers = List.generate(
          _maxConcurrent.clamp(1, queued.length.clamp(1, _maxConcurrent)),
          (_) => worker());
      await Future.wait(workers);
    } finally {
      await WakelockPlus.disable();
      _running = false;
      await _persist();
    }
  }

  Future<void> retry(UploadItem item, TusClient tus,
      {void Function(String path)? onComplete}) async {
    if (_running) return;
    if (item.file == null) {
      item.status = UploadStatus.interrupted;
      item.error = 'File is no longer available on this device';
      notifyListeners();
      await _persist();
      return;
    }
    item.status = UploadStatus.queued;
    item.error = null;
    item.progress = 0;
    notifyListeners();
    _running = true;
    try {
      await WakelockPlus.enable();
      await _uploadOne(item, tus, onComplete: onComplete);
    } finally {
      await WakelockPlus.disable();
      _running = false;
      await _persist();
    }
  }

  Future<void> startBackgroundUpload(
    UploadItem item,
    ApiClient api, {
    BackgroundUploader uploader = const BackgroundUploader(),
  }) async {
    final path = item.filePath ?? item.file?.path;
    if (path == null || item.file == null) {
      item.status = UploadStatus.interrupted;
      item.error = 'File is no longer available on this device';
      notifyListeners();
      await _persist();
      return;
    }

    item.status = UploadStatus.background;
    item.error = null;
    notifyListeners();
    await _persist();

    try {
      await uploader.start(
        api,
        BackgroundUploadRequest(
          id: item.id,
          filePath: path,
          filename: item.name,
          fileSize: item.fileSize,
          targetPath: item.targetPath,
          tusUrl: item.tusUrl,
        ),
      );
    } catch (e) {
      item.status = UploadStatus.error;
      item.error = e.toString();
      notifyListeners();
      await _persist();
    }
  }

  Future<void> _uploadOne(UploadItem item, TusClient tus,
      {void Function(String path)? onComplete}) async {
    item.status = UploadStatus.uploading;
    notifyListeners();

    try {
      String? finalPath;

      if (item.tusUrl != null) {
        try {
          finalPath = await tus.resume(item.tusUrl!, item.file!,
              onProgress: (sent, total) {
            item.progress = total > 0 ? sent / total : 0;
            notifyListeners();
          });
        } on ApiException catch (e) {
          if (e.message == 'upload_gone') {
            item.tusUrl = null;
          } else {
            rethrow;
          }
        }
      }

      if (finalPath == null && item.tusUrl == null) {
        finalPath = await tus.upload(
          item.file!,
          item.targetPath,
          onProgress: (sent, total) {
            item.progress = total > 0 ? sent / total : 0;
            notifyListeners();
          },
          onCreated: (url) {
            item.tusUrl = url;
            unawaited(_persist());
          },
        );
      }

      item.status = UploadStatus.done;
      item.progress = 1.0;
      item.finalPath = finalPath ?? '${item.targetPath}/${item.name}';
      onComplete?.call(item.targetPath);
    } catch (e) {
      item.status = UploadStatus.error;
      item.error = e.toString();
    }
    notifyListeners();
    await _persist();
  }

  void clearCompleted() {
    _items.removeWhere((i) =>
        i.status == UploadStatus.done || i.status == UploadStatus.interrupted);
    notifyListeners();
    unawaited(_persist());
  }

  void remove(UploadItem item) {
    _items.remove(item);
    notifyListeners();
    unawaited(_persist());
  }
}
