# goDrive Mobile

Flutter app for goDrive — iOS and Android.

## Setup

This repo pins the local toolchain through `mise`:

```sh
mise install
mise exec -- go version
mise exec -- node --version
mise exec -- npm --version
mise exec -- java -version
mise exec -- flutter --version
```

Pinned tools are Go 1.26.2, Node 22, Java 21, and Flutter 3.44.0. npm is provided by the pinned Node runtime.

### First-time scaffold

The `mobile/` directory includes the Flutter platform scaffold. If a checkout is missing generated platform files, regenerate them with:

```sh
# From the repo root
flutter create --project-name godrive --org com.example --platforms android,ios mobile
```

This writes the missing `android/build.gradle.kts`, iOS Xcode project, etc. without touching existing source files (Flutter merges, not overwrites).

Then install dependencies:

```sh
make mobile-install
# or: flutter pub get --directory mobile
```

### Development

```sh
# Start emulator + backend + flutter run in one command
make mobile-dev

# Flutter run only (emulator + backend already running)
make mobile-run

# Build debug APK
make mobile-build-android
# APK: mobile/build/app/outputs/flutter-apk/app-debug.apk
```

Backend must listen on `0.0.0.0:8121` (set in `.env`). App login URL:
```
http://10.0.2.2:8121
```
(`10.0.2.2` = host's `127.0.0.1` from inside Android emulator)

### Android notes

- Impeller is disabled via `AndroidManifest.xml` (`EnableImpeller = false`) — it crashes on x86_64 emulators. Remove that entry for real device testing if desired.
- Gradle 9.0 is required for Java 25 compatibility.
- Debug builds allow cleartext HTTP to any host via `android/app/src/debug/res/xml/network_security_config.xml`.
- Release builds enforce HTTPS. Use a Caddy/nginx reverse proxy with TLS in production.
- Release builds require a real Android upload keystore. For local builds, copy `android/key.properties.example` to `android/key.properties` and point it at your keystore. In GitHub Actions, set `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, and `ANDROID_KEY_PASSWORD`.
- The `Release` workflow can run without Android signing secrets by generating a temporary dry-run keystore. Those dry-run APK/AAB artifacts are only for validating the pipeline and must not be shipped.

### iOS notes

- `NSAllowsLocalNetworking = true` in `Info.plist` allows HTTP to `.local` hostnames for LAN dev.
- Remove that key and use HTTPS in production.
- Requires Xcode + Apple Developer account (free account can sideload for personal use).
- The `Release` workflow builds an unsigned iOS dry-run IPA on GitHub-hosted macOS runners. TestFlight upload still needs a signed IPA path with Apple distribution certificates and provisioning profiles.

## Release Pipeline

Existing development workflows stay unchanged:

- `Mobile` builds and uploads the Android debug APK for pull requests and `main`.
- `iOS` remains the manual unsigned iOS build used for device sideload experiments.

The separate `Release` workflow runs on `v*` tags or manually:

- Android always builds a release `.aab` and `.apk`.
- Without Android signing secrets, it uses a temporary dry-run keystore and uploads workflow artifacts only.
- With Android signing secrets, the artifacts are signed with the configured upload key.
- On tag builds, Android artifacts are attached to the GitHub Release.
- Google Play upload is opt-in through `workflow_dispatch` with `upload_google_play=true`; it uploads the `.aab` to the internal track as a draft release.
- iOS currently validates the unsigned build/package path and uploads an unsigned dry-run IPA artifact. Signed TestFlight upload is intentionally blocked until Apple certificates and provisioning profiles are configured.

## Features

| Feature | Status |
|---|---|
| Login with server URL + credentials | ✅ |
| File browser (list view) | ✅ |
| File browser (grid view with thumbnails) | ✅ |
| Breadcrumb navigation | ✅ |
| Folder pagination with load-more | ✅ |
| Search (indexed filename/path) | ✅ |
| Create folder | ✅ |
| Rename file/folder | ✅ |
| Move to folder (folder picker) | ✅ |
| Delete to trash | ✅ |
| Trash list, restore, permanent delete | ✅ |
| Download (via url_launcher) | ✅ |
| Open externally | ✅ |
| Image viewer (pinch-zoom, prev/next, metadata) | ✅ |
| In-app video player (chewie + video_player) | ✅ |
| PDF viewer (url_launcher → system viewer) | ✅ |
| Text/Markdown preview | ✅ |
| Upload from file picker | ✅ |
| Upload from camera | ✅ |
| Upload from photo library (image_picker) | ✅ |
| Upload queue with TUS resume | ✅ |
| Upload progress per file | ✅ |
| Retry failed/interrupted uploads | ✅ |
| Queue persistence across app restarts | ✅ (metadata only — file handles lost) |
| Wakelock during active uploads | ✅ |
| Admin: system stats | ✅ |
| Admin: reindex + preview warmup jobs | ✅ |
| Admin: user management (create/edit/disable) | ✅ |
| Admin: webhook management | ✅ |
| Background upload | ✅ Android foreground service, iOS background URLSession |
| Offline support | ❌ out of scope |

## Architecture

| Path | Role |
|---|---|
| `lib/main.dart` | App root, MultiProvider (AuthState, UploadQueue), theme |
| `lib/api/client.dart` | Typed HTTP client — all REST endpoints |
| `lib/api/tus.dart` | TUS upload client with HEAD-based resume |
| `lib/api/models.dart` | Dart models: FileEntry, User, TrashItem, AdminJob, AdminStats, Webhook... |
| `lib/state/auth_state.dart` | Login, logout, session restore |
| `lib/state/upload_queue.dart` | Upload queue, 3 concurrent workers, SharedPreferences persistence |
| `lib/storage/session.dart` | FlutterSecureStorage token, SharedPreferences base URL |
| `lib/screens/login_screen.dart` | URL + credentials form |
| `lib/screens/files_screen.dart` | Full file browser (list/grid, search, upload, trash, admin navigation) |
| `lib/screens/image_viewer_screen.dart` | PhotoViewGallery, prev/next, zoom, metadata overlay |
| `lib/screens/video_player_screen.dart` | chewie + VideoPlayerController.networkUrl with auth headers |
| `lib/screens/admin_screen.dart` | System stats, job runner, user/webhook CRUD |
| `lib/widgets/breadcrumb_bar.dart` | Horizontal scrollable path segments |
| `lib/widgets/file_tile.dart` | List row with CachedNetworkImage thumbnail (auth headers) |
| `lib/widgets/upload_queue_sheet.dart` | Draggable bottom sheet with per-file progress |
| `android/app/src/main/kotlin/com/example/godrive/BackgroundUploadService.kt` | Android foreground-service TUS uploader |
| `ios/Runner/AppDelegate.swift` | iOS background URLSession TUS uploader bridge |

## Auth

Bearer token auth — no CSRF needed on mobile.
Token stored in `FlutterSecureStorage` with Android `EncryptedSharedPreferences`.
Server URL stored in `SharedPreferences`.

**Important:** `AuthState.init()` and `UploadQueue.init()` catch ALL exceptions — `FlutterSecureStorage` and `SharedPreferences` can throw on first run (Android keystore not initialized), which would crash the Dart VM if uncaught.

## Upload Flow

1. User taps FAB → chooses source (photo library / camera / files).
2. Files enqueued in `UploadQueue`.
3. Up to 3 concurrent TUS uploads.
4. `wakelock_plus` keeps screen on while any upload is active.
5. TUS URL stored per item; retry does HEAD → resume from offset.
6. `done`/`interrupted` items survive app restart (file paths lost, metadata kept).
7. Android users can move a queued/failed/interrupted item to a foreground service; the app prompts for notification permission when needed, the service shows progress, and the upload sheet refreshes persisted progress/completion while open.
8. iOS users can move a queued/failed/interrupted item to a native background `URLSession` upload. The native bridge creates/resumes the TUS upload, schedules the PATCH as a background upload task, and writes progress/completion back into the persisted queue.
