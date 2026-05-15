import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:godrive/main.dart';
import 'package:godrive/state/auth_state.dart';
import 'package:godrive/state/upload_queue.dart';

void main() {
  testWidgets('shows login screen when signed out',
      (WidgetTester tester) async {
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

    expect(find.byIcon(Icons.folder_open_rounded), findsOneWidget);
    expect(find.text('goDrive'), findsOneWidget);
    expect(find.text('Server URL'), findsOneWidget);
    expect(find.text('Sign in'), findsOneWidget);
  });
}
