import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'state/auth_state.dart';
import 'state/upload_queue.dart';
import 'screens/login_screen.dart';
import 'screens/files_screen.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(
    MultiProvider(
      providers: [
        ChangeNotifierProvider(create: (_) => AuthState()..init()),
        ChangeNotifierProvider(create: (_) => UploadQueue()..init()),
      ],
      child: const GoDriveApp(),
    ),
  );
}

class GoDriveApp extends StatelessWidget {
  const GoDriveApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'goDrive',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF006C68),
          brightness: Brightness.light,
        ),
        useMaterial3: true,
        appBarTheme: const AppBarTheme(
          centerTitle: false,
          elevation: 0,
        ),
      ),
      darkTheme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF006C68),
          brightness: Brightness.dark,
        ),
        useMaterial3: true,
        appBarTheme: const AppBarTheme(
          centerTitle: false,
          elevation: 0,
        ),
      ),
      home: const _Root(),
    );
  }
}

class _Root extends StatelessWidget {
  const _Root();

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthState>();
    if (auth.loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (!auth.loggedIn) {
      return const LoginScreen();
    }
    return const FilesScreen(path: '/');
  }
}
