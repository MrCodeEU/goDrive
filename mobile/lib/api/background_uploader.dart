import 'dart:io';

import 'package:flutter/services.dart';

import 'client.dart';

class BackgroundUploadRequest {
  final String id;
  final String filePath;
  final String filename;
  final int fileSize;
  final String targetPath;
  final String? tusUrl;

  const BackgroundUploadRequest({
    required this.id,
    required this.filePath,
    required this.filename,
    required this.fileSize,
    required this.targetPath,
    this.tusUrl,
  });
}

class BackgroundUploader {
  static const _channel = MethodChannel('godrive/background_uploads');

  const BackgroundUploader();

  Future<bool> get isSupported async {
    if (!Platform.isAndroid && !Platform.isIOS) return false;
    return await _channel.invokeMethod<bool>('isSupported') ?? false;
  }

  Future<void> refresh() async {
    if (!Platform.isAndroid && !Platform.isIOS) return;
    await _channel.invokeMethod<void>('refreshUploads');
  }

  Future<void> start(ApiClient api, BackgroundUploadRequest request) async {
    if (!Platform.isAndroid && !Platform.isIOS) {
      throw const ApiException(
          0, 'Background upload is only available on Android and iOS');
    }

    if (Platform.isAndroid) {
      final granted =
          await _channel.invokeMethod<bool>('ensureNotificationPermission') ??
              false;
      if (!granted) {
        throw const ApiException(
            0, 'Notification permission is required for background upload');
      }
    }

    await _channel.invokeMethod<void>('startUpload', {
      'id': request.id,
      'filePath': request.filePath,
      'filename': request.filename,
      'fileSize': request.fileSize,
      'targetPath': request.targetPath,
      'tusUrl': request.tusUrl,
      'baseUrl': api.baseUrl,
      'token': api.token,
    });
  }
}
