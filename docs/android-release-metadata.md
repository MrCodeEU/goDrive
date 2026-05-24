# Android Release Metadata

## Package

- Application ID: `eu.mljr.godrive`
- Android namespace: `eu.mljr.godrive`
- App label: `goDrive`
- Release network policy: HTTPS-only through `network_security_config.xml`
- Debug network policy: cleartext HTTP allowed for local/LAN development

The application ID is the permanent Play Store package name. Do not change it after the first uploaded Play artifact.

## Versioning

Flutter source version lives in `mobile/pubspec.yaml`.

```yaml
version: 0.1.0+1
```

Release workflow behavior:

- Tag builds named `vX.Y.Z` use Android `versionName=X.Y.Z`.
- Manual release builds use the version name from `mobile/pubspec.yaml`.
- Android `versionCode` uses `GITHUB_RUN_NUMBER` so Play uploads remain monotonically increasing across release workflow runs.

Before creating a public release tag, update `mobile/pubspec.yaml` to the intended version and tag the same version, for example `v0.1.0`.

## Launcher And Splash

Android launcher metadata is repo-owned:

- App icon: `@mipmap/ic_launcher`
- Round icon: `@mipmap/ic_launcher_round`
- Adaptive icon foreground: `@drawable/ic_launcher_foreground`
- Adaptive icon background: `@color/godrive_icon_background`
- Splash background: `@color/godrive_splash_background`
- Splash icon: `@drawable/godrive_splash_icon`

The current icon is a simple first-party goDrive mark so Play testing does not ship the default Flutter icon. Replace it later only if the final brand changes.

## Permissions Review

Declared production permissions:

- `INTERNET`: required for self-hosted server access.
- `FOREGROUND_SERVICE` and `FOREGROUND_SERVICE_DATA_SYNC`: required for foreground upload work on modern Android.
- `POST_NOTIFICATIONS`: required to show foreground upload notifications on Android 13+.
- `READ_EXTERNAL_STORAGE` with `maxSdkVersion=32`: compatibility for file/media selection on older Android versions.
- `READ_MEDIA_IMAGES` and `READ_MEDIA_VIDEO`: media picker/access on Android 13+.

The app does not request broad storage management. It uses platform file and media pickers rather than direct full-device filesystem access.

## Play Listing Draft

Short description:

```text
Self-hosted file manager with uploads, previews, search, and mobile access.
```

Full description draft:

```text
goDrive is a self-hosted file manager for private servers. Connect the app to your own goDrive instance to browse files, upload photos and documents, preview common media, resume interrupted uploads, and manage your personal storage from Android.

goDrive is designed for small private deployments where you control the server and data location. It does not provide hosted cloud storage by itself.
```

Store asset checklist:

- 512 x 512 Play icon.
- 1024 x 500 feature graphic.
- Phone screenshots covering login, files, preview, upload queue, and admin where appropriate.
- Privacy policy URL.
- Data safety answers.
