# Mobile Apps

Flutter apps for Android and iOS with feature parity on core flows.

## Features

- File browser: list and grid views
- Image viewer and in-app video player
- TUS upload queue with resume
- Wakelock held during active uploads
- Android foreground service for background uploads
- iOS background URLSession for selected file uploads
- Share sheet integration (send files to goDrive from other apps)
- Admin screen: stats, reindex/warmup jobs, user and webhook management

## Android Setup

```bash
make mobile-install       # flutter pub get
make mobile-dev           # start emulator + backend + flutter run
make mobile-build-android # build debug APK
```

The emulator reaches the backend at `http://10.0.2.2:8121` (Android maps `10.0.2.2` → host `127.0.0.1`).

## iOS Setup

iOS builds run on GitHub Actions (`macos-latest`). [xtool](https://xtool.sh) signs and installs from Linux without Xcode.

**One-time setup (Fedora Atomic):**

```bash
rpm-ostree install usbmuxd && systemctl reboot  # skip if already installed
make xtool-setup    # download xtool AppImage, add USB udev rule
make xtool-auth     # Apple ID login
make ios-devices    # verify device appears
```

**Dev loop:**

```bash
make ios-deploy    # push to ios-dev branch → CI builds → xtool installs (~8-12 min)
make ios-refresh   # re-sign when 7-day free cert expires (no rebuild)
```

## Connecting to your server

Enter your goDrive server URL and credentials on the login screen. Use `https://` in production — the app enforces HTTPS for non-localhost addresses.
