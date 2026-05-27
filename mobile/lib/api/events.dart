import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'client.dart';

enum FileEventConnectionStatus { disconnected, connecting, connected }

class FileEvent {
  final String id;
  final String event;
  final DateTime timestamp;
  final Map<String, dynamic> data;

  const FileEvent({
    required this.id,
    required this.event,
    required this.timestamp,
    required this.data,
  });

  factory FileEvent.fromJson(Map<String, dynamic> json) => FileEvent(
        id: json['id'] as String? ?? '',
        event: json['event'] as String? ?? '',
        timestamp: DateTime.tryParse(json['timestamp'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
        data: (json['data'] as Map<String, dynamic>?) ?? const {},
      );
}

class FileEventService {
  final ApiClient client;
  final http.Client _http;
  final _events = StreamController<FileEvent>.broadcast();
  final _status = StreamController<FileEventConnectionStatus>.broadcast();
  bool _running = false;
  bool _disposed = false;
  int _reconnectAttempt = 0;
  StreamSubscription<String>? _lineSubscription;

  FileEventService({required this.client, http.Client? httpClient})
      : _http = httpClient ?? client.httpClient;

  Stream<FileEvent> get events => _events.stream;
  Stream<FileEventConnectionStatus> get status => _status.stream;

  void start() {
    if (_running || _disposed) return;
    _running = true;
    _reconnectAttempt = 0;
    unawaited(_connectLoop());
  }

  Future<void> stop() async {
    _running = false;
    await _lineSubscription?.cancel();
    _lineSubscription = null;
    if (!_disposed) {
      _status.add(FileEventConnectionStatus.disconnected);
    }
  }

  Future<void> dispose() async {
    await stop();
    _disposed = true;
    _http.close();
    await _events.close();
    await _status.close();
  }

  Future<void> _connectLoop() async {
    while (_running && !_disposed) {
      _status.add(FileEventConnectionStatus.connecting);
      try {
        await _connectOnce();
      } catch (_) {
        // Live refresh is opportunistic; manual refresh remains available.
      }
      if (!_running || _disposed) break;
      _status.add(FileEventConnectionStatus.disconnected);
      await Future<void>.delayed(_nextReconnectDelay());
    }
  }

  Future<void> _connectOnce() async {
    final request = http.Request('GET', client.eventsUri())
      ..headers.addAll(client.authHeader)
      ..headers['Accept'] = 'text/event-stream';
    final response = await _http.send(request);
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw http.ClientException(
        'event stream failed with ${response.statusCode}',
        client.eventsUri(),
      );
    }

    _reconnectAttempt = 0;
    _status.add(FileEventConnectionStatus.connected);
    final completer = Completer<void>();
    final parser = _SSEParser((event) {
      if (!_events.isClosed) _events.add(event);
    });
    _lineSubscription = response.stream
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen(
          parser.addLine,
          onError: completer.completeError,
          onDone: completer.complete,
          cancelOnError: true,
        );
    await completer.future;
  }

  Duration _nextReconnectDelay() {
    _reconnectAttempt++;
    final seconds = switch (_reconnectAttempt) {
      1 => 1,
      2 => 2,
      3 => 5,
      _ => 10,
    };
    return Duration(seconds: seconds);
  }
}

class _SSEParser {
  final void Function(FileEvent event) onEvent;
  final List<String> _data = [];

  _SSEParser(this.onEvent);

  void addLine(String line) {
    if (line.isEmpty) {
      _dispatch();
      return;
    }
    if (line.startsWith(':')) return;
    if (line.startsWith('data:')) {
      _data.add(line.substring(5).trimLeft());
    }
  }

  void _dispatch() {
    if (_data.isEmpty) return;
    final raw = _data.join('\n');
    _data.clear();
    try {
      onEvent(FileEvent.fromJson(jsonDecode(raw) as Map<String, dynamic>));
    } catch (_) {
      // Ignore malformed events and keep the stream alive.
    }
  }
}
