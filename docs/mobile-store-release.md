# Mobile Store Release Runbook

This runbook describes the path from the current dry-run GitHub Actions release flow to first internal testing builds on Google Play and TestFlight.

## Current Automation State

- Existing development workflows are unchanged.
- `.github/workflows/release.yml` runs on `v*` tags or manual dispatch.
- Android release builds produce both:
  - Play Store artifact: `.aab`
  - GitHub Release sideload artifact: `.apk`
- Android release builds are dry-runnable without production credentials by generating a temporary CI keystore.
- Google Play upload is opt-in with `workflow_dispatch` input `upload_google_play=true`.
- iOS release validation currently builds and packages an unsigned dry-run IPA.
- TestFlight upload is deliberately blocked until signed IPA export is implemented with real Apple distribution certificates and provisioning profiles.

## Android: First Internal Testing Build

### 1. Create the app in Play Console

1. Create a Google Play Developer account.
2. Create a new app.
3. Use package name `eu.mljr.godrive`.
4. Choose app or game, free or paid, and declarations.
5. Complete the required dashboard setup far enough that an internal testing release can be created.

The package name is permanent after upload. Do not upload a test bundle under `eu.mljr.godrive` unless that is the final package name.

### 2. Create the Android upload keystore

Create one long-lived upload keystore and store it offline:

```sh
keytool -genkeypair \
  -v \
  -keystore godrive-upload.jks \
  -storetype JKS \
  -keyalg RSA \
  -keysize 4096 \
  -validity 10000 \
  -alias upload
```

Back up:

- `godrive-upload.jks`
- keystore password
- key alias
- key password

Losing the upload key is recoverable through Play Console key reset, but it is still a release blocker.

### 3. Add GitHub Android secrets

Base64 encode the keystore:

```sh
base64 -w0 godrive-upload.jks
```

Add repository secrets:

- `ANDROID_KEYSTORE_BASE64`
- `ANDROID_KEYSTORE_PASSWORD`
- `ANDROID_KEY_ALIAS`
- `ANDROID_KEY_PASSWORD`

### 4. Build once without uploading

Run GitHub Actions manually:

- Workflow: `Release`
- `upload_google_play=false`
- `upload_testflight=false`

Expected result:

- `godrive-android-release` workflow artifact exists.
- `.aab` and `.apk` are produced.
- No Play Store upload happens.

### 5. Upload first AAB manually

For the very first Play setup, upload the `.aab` manually in Play Console to internal testing.

Use Play App Signing when prompted. Prefer Google-managed app signing key and keep your local keystore as the upload key.

### 6. Create an internal testing track

1. Create an internal tester email list.
2. Create an internal testing release.
3. Add the uploaded AAB.
4. Fill release notes.
5. Roll out to internal testing.
6. Install through the internal testing link on a real Android device.

### 7. Enable automated Google Play upload

After the first manual upload works:

1. Create a Google Cloud service account for Play Developer API access.
2. Link it in Play Console under API access.
3. Grant only the permissions required to upload releases for this app.
4. Download the JSON key.
5. Add it as GitHub secret `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON`.
6. Run `Release` manually with `upload_google_play=true`.

The workflow uploads to the `internal` track with `status: draft`, so the final rollout remains a Play Console decision.

## iOS: First TestFlight Build

### 1. Enroll and create app identity

1. Enroll in the Apple Developer Program.
2. In Apple Developer, create App ID `eu.mljr.godrive`.
3. Create App Group `group.eu.mljr.godrive`.
4. Enable required capabilities for the app and share extension.
5. Create extension App ID `eu.mljr.godrive.ShareExtension`.

The Runner app and Share Extension need separate provisioning coverage.

### 2. Create the app in App Store Connect

1. Create a new iOS app.
2. Use bundle ID `eu.mljr.godrive`.
3. Set SKU, name, primary language, and availability.
4. Complete the minimum TestFlight metadata.

### 3. Prepare signing assets

Create:

- Apple Distribution certificate.
- App Store provisioning profile for `eu.mljr.godrive`.
- App Store provisioning profile for `eu.mljr.godrive.ShareExtension`.

Export the distribution certificate as password-protected `.p12`.

GitHub secrets that will be needed for signed IPA export:

- `IOS_DISTRIBUTION_CERTIFICATE_BASE64`
- `IOS_DISTRIBUTION_CERTIFICATE_PASSWORD`
- `IOS_RUNNER_PROVISIONING_PROFILE_BASE64`
- `IOS_SHARE_EXTENSION_PROVISIONING_PROFILE_BASE64`
- `APPLE_TEAM_ID`
- `IOS_RUNNER_PROVISIONING_PROFILE_NAME`
- `IOS_SHARE_EXTENSION_PROVISIONING_PROFILE_NAME`

### 4. Create App Store Connect API key

Create an App Store Connect API key with the least role that can upload TestFlight builds.

GitHub secrets:

- `APPSTORE_API_PRIVATE_KEY`
- `APPSTORE_API_KEY_ID`
- `APPSTORE_ISSUER_ID`

### 5. Implement signed IPA export

The remaining CI work is:

1. Import the distribution certificate into a temporary macOS keychain.
2. Install both provisioning profiles.
3. Build the Runner app and Share Extension with manual signing.
4. Embed `ShareExtension.appex` into `Runner.app`.
5. Export a signed App Store Connect IPA.
6. Upload with `Apple-Actions/upload-testflight-build@v3`.

Until this is implemented, the `Release` workflow intentionally produces only an unsigned iOS dry-run artifact.

### 6. TestFlight internal testing

1. Upload the signed IPA.
2. Wait for App Store Connect processing.
3. Create an internal tester group.
4. Add up to 100 internal testers.
5. Attach the processed build.
6. Install through TestFlight on a real iPhone.

External TestFlight testing and App Store release require Apple review and more complete app metadata.

## Store Listing Materials

Prepare before requesting public review:

- App name and subtitle.
- Short and full descriptions.
- Support URL.
- Privacy policy URL.
- Screenshots for required device classes.
- App icon review.
- Android feature graphic if needed.
- Category.
- Age rating answers.
- Play Data Safety answers.
- Apple App Privacy answers.
- Test credentials or reviewer instructions for a self-hosted server.
- Clear explanation that users connect to their own goDrive server.

## Pre-Release Technical Gates

Before the first public release:

- Run backend, web, mobile, and security checks.
- Run Android internal testing on physical devices.
- Run iOS TestFlight on physical devices.
- Test background uploads with large files and interrupted networks.
- Test reverse-proxy HTTPS deployment.
- Test WebDAV against intended clients.
- Verify no secrets, local databases, or build artifacts are committed.
- Confirm `SECURITY.md`, license, and contribution docs are present.
- Confirm demo instance policy and reset mechanism are decided.
