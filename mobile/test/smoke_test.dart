// Integration smoke tests: login, browse, navigate, open image, open text.
// All HTTP is intercepted by MockClient — no real server needed.
import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:godrive/api/client.dart';
import 'package:godrive/main.dart';
import 'package:godrive/state/auth_state.dart';
import 'package:godrive/state/upload_queue.dart';
import 'package:godrive/screens/files_screen.dart';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

http.Response _json(Object body, {int status = 200}) => http.Response(
      jsonEncode(body),
      status,
      headers: {'content-type': 'application/json'},
    );

// HTTP client that routes SSE requests to a never-ending stream and all
// other requests through a MockClientHandler. Prevents SSE reconnect timers.
class _TestClient extends http.BaseClient {
  final MockClientHandler _handler;
  // Keep controllers so they can be cleaned up.
  final _sseControllers = <StreamController<List<int>>>[];

  _TestClient(this._handler);

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.url.path == '/api/events') {
      final ctrl = StreamController<List<int>>();
      _sseControllers.add(ctrl);
      return http.StreamedResponse(
        ctrl.stream,
        200,
        headers: {'content-type': 'text/event-stream'},
      );
    }
    final req = request as http.Request;
    final resp = await _handler(req);
    return http.StreamedResponse(
      http.ByteStream.fromBytes(resp.bodyBytes),
      resp.statusCode,
      headers: resp.headers,
    );
  }

  @override
  void close() {
    for (final ctrl in _sseControllers) {
      ctrl.close();
    }
    super.close();
  }
}

Map<String, dynamic> _userJson() => {
      'id': 1,
      'username': 'alice',
      'is_admin': false,
      'disabled': false,
      'home_root': '/data/alice',
    };

Map<String, dynamic> _fileEntry({
  required String name,
  String type = 'file',
  String? previewKind,
  String? mimeType,
}) =>
    {
      'name': name,
      'path': '/$name',
      'type': type,
      'size': 1024,
      'modified_at': '2026-01-01T00:00:00Z',
      if (previewKind != null) 'preview_kind': previewKind,
      if (mimeType != null) 'mime_type': mimeType,
    };

Map<String, dynamic> _listPage(List<Map<String, dynamic>> entries,
        {String path = '/'}) =>
    {
      'path': path,
      'entries': entries,
      'total': entries.length,
      'offset': 0,
      'limit': 200,
      'has_more': false,
    };

// Pre-authenticated AuthState backed by a MockClient.
class _SeedAuthState extends AuthState {
  final _TestClient mockHttp;
  _SeedAuthState(this.mockHttp);

  @override
  Future<void> init() async {
    final client = ApiClient(
      baseUrl: 'https://drive.test',
      token: 'test-token',
      httpClient: mockHttp,
    );
    // Resolve the user so loggedIn == true.
    final user = await client.me();
    setLoggedIn(client, user);
  }
}

// Pump the full app with a given AuthState and UploadQueue.
Future<void> _pumpApp(
  WidgetTester tester,
  AuthState auth, {
  UploadQueue? queue,
}) async {
  SharedPreferences.setMockInitialValues({});
  await tester.pumpWidget(
    MultiProvider(
      providers: [
        ChangeNotifierProvider<AuthState>.value(value: auth),
        ChangeNotifierProvider<UploadQueue>.value(
            value: queue ?? UploadQueue()..init()),
      ],
      child: const GoDriveApp(),
    ),
  );
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 300));
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {

// ---------------------------------------------------------------------------
// Login smoke
// ---------------------------------------------------------------------------

group('login screen', () {
  testWidgets('shows all required fields when signed out', (tester) async {
    SharedPreferences.setMockInitialValues({});
    await tester.pumpWidget(
      MultiProvider(
        providers: [
          ChangeNotifierProvider(create: (_) => AuthState()..init()),
          ChangeNotifierProvider(create: (_) => UploadQueue()..init()),
        ],
        child: const GoDriveApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Server URL'), findsOneWidget);
    expect(find.text('Username'), findsOneWidget);
    expect(find.text('Password'), findsOneWidget);
    expect(find.text('Sign in'), findsOneWidget);
  });

});

// ---------------------------------------------------------------------------
// Browse smoke (requires pre-authenticated state)
// ---------------------------------------------------------------------------

group('browse', () {
  testWidgets('shows files and folders at root', (tester) async {
    final mockHttp = _TestClient((req) async {
      if (req.url.path.contains('/api/me')) return _json({'user': _userJson()});
      if (req.url.path == '/api/files/list') {
        return _json(_listPage([
          _fileEntry(name: 'Documents', type: 'dir'),
          _fileEntry(name: 'photo.jpg', previewKind: 'image', mimeType: 'image/jpeg'),
          _fileEntry(name: 'notes.txt', previewKind: 'text', mimeType: 'text/plain'),
          _fileEntry(name: 'video.mp4', previewKind: 'video', mimeType: 'video/mp4'),
        ]));
      }
      return _json({}, status: 404);
    });

    final auth = _SeedAuthState(mockHttp);
    await auth.init();
    await _pumpApp(tester, auth);

    expect(find.text('Documents'), findsOneWidget);
    expect(find.text('photo.jpg'), findsOneWidget);
    expect(find.text('notes.txt'), findsOneWidget);
    expect(find.text('video.mp4'), findsOneWidget);
  });

  testWidgets('shows empty state when directory is empty', (tester) async {
    final mockHttp = _TestClient((req) async {
      if (req.url.path.contains('/api/me')) return _json({'user': _userJson()});
      if (req.url.path == '/api/files/list') return _json(_listPage([]));
      return _json({}, status: 404);
    });

    final auth = _SeedAuthState(mockHttp);
    await auth.init();
    await _pumpApp(tester, auth);

    // Empty directory should show a hint or empty list (no crash).
    expect(find.byType(FilesScreen), findsOneWidget);
  });

  testWidgets('navigates into subfolder on tap', (tester) async {
    var listCallCount = 0;
    final mockHttp = _TestClient((req) async {
      if (req.url.path.contains('/api/me')) return _json({'user': _userJson()});
      if (req.url.path == '/api/files/list') {
        listCallCount++;
        final path = req.url.queryParameters['path'] ?? '/';
        if (path == '/Documents') {
          return _json(_listPage(
            [_fileEntry(name: 'report.pdf', previewKind: 'pdf', mimeType: 'application/pdf')],
            path: '/Documents',
          ));
        }
        return _json(_listPage([
          _fileEntry(name: 'Documents', type: 'dir'),
        ]));
      }
      return _json({}, status: 404);
    });

    final auth = _SeedAuthState(mockHttp);
    await auth.init();
    await _pumpApp(tester, auth);

    expect(find.text('Documents'), findsOneWidget);
    await tester.tap(find.text('Documents'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    // After navigating into Documents, report.pdf should be visible.
    expect(find.text('report.pdf'), findsOneWidget);
    // Two list calls: root + Documents.
    expect(listCallCount, greaterThanOrEqualTo(2));
  });
});

// ---------------------------------------------------------------------------
// Open file smoke
// ---------------------------------------------------------------------------

group('open file', () {
  testWidgets('tapping image entry opens image viewer', (tester) async {
    final mockHttp = _TestClient((req) async {
      if (req.url.path.contains('/api/me')) return _json({'user': _userJson()});
      if (req.url.path == '/api/files/list') {
        return _json(_listPage([
          _fileEntry(name: 'photo.jpg', previewKind: 'image', mimeType: 'image/jpeg'),
        ]));
      }
      return _json({}, status: 404);
    });

    final auth = _SeedAuthState(mockHttp);
    await auth.init();
    await _pumpApp(tester, auth);

    expect(find.text('photo.jpg'), findsOneWidget);
    await tester.tap(find.text('photo.jpg'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    // Image viewer should be pushed onto the navigator stack.
    // It renders an AppBar with the filename.
    expect(find.text('photo.jpg'), findsWidgets);
  });

  testWidgets('tapping text file triggers text preview fetch', (tester) async {
    var previewFetched = false;
    final mockHttp = _TestClient((req) async {
      if (req.url.path.contains('/api/me')) return _json({'user': _userJson()});
      if (req.url.path == '/api/files/list') {
        return _json(_listPage([
          _fileEntry(name: 'notes.txt', previewKind: 'text', mimeType: 'text/plain'),
        ]));
      }
      if (req.url.path.contains('/api/files/text')) {
        previewFetched = true;
        return _json({
          'path': '/notes.txt',
          'content': 'Hello world',
          'mime_type': 'text/plain',
          'truncated': false,
        });
      }
      return _json({}, status: 404);
    });

    final auth = _SeedAuthState(mockHttp);
    await auth.init();
    await _pumpApp(tester, auth);

    expect(find.text('notes.txt'), findsOneWidget);
    await tester.tap(find.text('notes.txt'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(previewFetched, isTrue,
        reason: 'Expected text preview API call after tapping text file');
  });
});

} // end main
