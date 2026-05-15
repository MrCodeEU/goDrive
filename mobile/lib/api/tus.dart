import 'dart:convert';
import 'dart:io';
import 'package:http/http.dart' as http;
import 'client.dart';

typedef TusProgressCallback = void Function(int sent, int total);

class TusUpload {
  final String tusUrl;
  final String targetPath;
  final String filename;
  final int fileSize;

  const TusUpload({
    required this.tusUrl,
    required this.targetPath,
    required this.filename,
    required this.fileSize,
  });
}

class TusClient {
  final ApiClient api;

  const TusClient(this.api);

  // Create a new upload, returns the TUS URL (Location header).
  Future<String> create(String targetPath, String filename, int fileSize) async {
    final base = api.baseUrl.trimRight().replaceAll(RegExp(r'/$'), '');
    final uri = Uri.parse('$base/api/tus').replace(queryParameters: {'path': targetPath});

    final resp = await http.post(uri, headers: {
      ...api.authHeader,
      'Tus-Resumable': '1.0.0',
      'Upload-Length': '$fileSize',
      'Upload-Metadata': 'filename ${base64.encode(utf8.encode(filename))}',
    });

    if (resp.statusCode != 201) {
      String msg = 'Upload create failed';
      try {
        final body = jsonDecode(resp.body) as Map<String, dynamic>;
        msg = body['error'] as String? ?? msg;
      } catch (_) {}
      throw ApiException(resp.statusCode, msg);
    }

    final location = resp.headers['location'];
    if (location == null || location.isEmpty) {
      throw const ApiException(0, 'Upload endpoint did not return Location');
    }
    return location;
  }

  // Resume: HEAD to get current offset.
  Future<int> getOffset(String tusUrl) async {
    final uri = _resolveUrl(tusUrl);
    final resp = await http.head(uri, headers: {
      ...api.authHeader,
      'Tus-Resumable': '1.0.0',
    });
    if (resp.statusCode == 404) {
      throw const ApiException(404, 'upload_gone');
    }
    if (resp.statusCode != 204) {
      throw ApiException(resp.statusCode, 'HEAD failed');
    }
    return int.tryParse(resp.headers['upload-offset'] ?? '0') ?? 0;
  }

  // Upload from startOffset. Returns the final resolved path.
  Future<String?> patch(
    String tusUrl,
    File file,
    int startOffset,
    int fileSize, {
    TusProgressCallback? onProgress,
  }) async {
    final uri = _resolveUrl(tusUrl);

    // Stream file bytes from startOffset.
    final stream = file.openRead(startOffset);
    final req = http.StreamedRequest('PATCH', uri)
      ..headers.addAll({
        ...api.authHeader,
        'Content-Type': 'application/offset+octet-stream',
        'Tus-Resumable': '1.0.0',
        'Upload-Offset': '$startOffset',
        'Content-Length': '${fileSize - startOffset}',
      });

    int sent = startOffset;
    stream.listen(
      (List<int> chunk) {
        req.sink.add(chunk);
        sent += chunk.length;
        onProgress?.call(sent, fileSize);
      },
      onDone: () => req.sink.close(),
      onError: (Object e) => req.sink.addError(e),
      cancelOnError: true,
    );

    final resp = await http.Client().send(req);
    final respBody = await resp.stream.bytesToString();

    if (resp.statusCode >= 200 && resp.statusCode < 300) {
      return resp.headers['upload-final-path'];
    }
    String msg = 'Upload chunk failed';
    try {
      final body = jsonDecode(respBody) as Map<String, dynamic>;
      msg = body['error'] as String? ?? msg;
    } catch (_) {}
    throw ApiException(resp.statusCode, msg);
  }

  // Full upload: create → patch. Returns final resolved path.
  Future<String?> upload(
    File file,
    String targetPath, {
    TusProgressCallback? onProgress,
    void Function(String tusUrl)? onCreated,
  }) async {
    final filename = file.path.split('/').last;
    final fileSize = await file.length();

    final tusUrl = await create(targetPath, filename, fileSize);
    onCreated?.call(tusUrl);

    return patch(tusUrl, file, 0, fileSize, onProgress: onProgress);
  }

  // Resume an interrupted upload.
  Future<String?> resume(
    String tusUrl,
    File file, {
    TusProgressCallback? onProgress,
  }) async {
    final fileSize = await file.length();
    final offset = await getOffset(tusUrl);

    if (offset >= fileSize) {
      return null; // already complete
    }

    return patch(tusUrl, file, offset, fileSize, onProgress: onProgress);
  }

  Uri _resolveUrl(String tusUrl) {
    if (tusUrl.startsWith('http://') || tusUrl.startsWith('https://')) {
      return Uri.parse(tusUrl);
    }
    final base = api.baseUrl.trimRight().replaceAll(RegExp(r'/$'), '');
    return Uri.parse('$base$tusUrl');
  }
}
