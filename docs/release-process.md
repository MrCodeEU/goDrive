# Release Process

This document describes the normal goDrive release flow. Store-specific Android and iOS onboarding details live in `docs/mobile-store-release.md`.

## Versioning

goDrive uses semantic version tags:

```text
vMAJOR.MINOR.PATCH
```

Before `v1.0.0`, minor versions may still include breaking changes. Document those clearly in `CHANGELOG.md` and release notes.

## Changelog Rules

Every user-facing change should update `CHANGELOG.md` under `[Unreleased]`.

Use these sections when they apply:

- `Added` for new features.
- `Changed` for behavior changes.
- `Deprecated` for supported features that will be removed later.
- `Removed` for removed features.
- `Fixed` for bug fixes.
- `Security` for vulnerability fixes and security hardening.

Internal refactors, test-only changes, and formatting-only changes do not need changelog entries unless they affect users, deployers, contributors, or release operators.

## Pre-Release Checklist

1. Confirm the working tree is clean.
2. Review `todo.md` for release blockers.
3. Run the local quality gate that fits the release scope:

   ```sh
   make check
   make security
   make api-contract
   ```

4. Run targeted mobile checks:

   ```sh
   make mobile-test
   flutter analyze
   ```

5. Run local `act` checks for changed Linux workflows where practical.
6. Run the manual test plan in `docs/manual-test-plan.md` against disposable data.
7. Verify Docker deployment and preview tooling in a fresh container.
8. Verify the demo deployment path if demo-facing behavior changed.
9. Confirm no secrets, local databases, generated scratch files, or private paths are committed.
10. Confirm `CHANGELOG.md` has the final release notes.

## Cutting a Release

1. Move relevant `CHANGELOG.md` entries from `[Unreleased]` into a dated release section:

   ```md
   ## [0.1.0] - 2026-05-24
   ```

2. Leave a fresh empty `[Unreleased]` section at the top.
3. Commit the changelog update.
4. Create and push an annotated tag:

   ```sh
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

5. The release workflow builds Android `.aab` and `.apk` artifacts and creates or updates the GitHub Release for tag pushes.
6. Review generated release notes and attach any missing context from `CHANGELOG.md`.

## Android Release

For tag builds, Android `versionName` is derived from the tag without the leading `v`. The Android `versionCode` is the GitHub run number.

The release workflow always builds:

- `.aab` for Play Store upload.
- `.apk` for direct GitHub Release download.

Google Play upload is manual and opt-in through `workflow_dispatch` with `upload_google_play=true`. The workflow uploads to the internal track as a draft when credentials exist.

## iOS Release

iOS release automation is intentionally manual until signing is fully configured.

Current workflow support:

- unsigned iOS dry-run build through `workflow_dispatch`
- no automatic TestFlight upload until signed IPA export is implemented

Use `docs/mobile-store-release.md` for Apple Developer, App Store Connect, provisioning profile, and TestFlight setup.

## Post-Release

After publishing:

1. Verify the GitHub Release artifacts.
2. Pull and smoke-test the published Docker image.
3. Confirm the demo instance deploys and resets correctly.
4. For Android, promote the internal-track draft only after device testing.
5. For iOS, upload to TestFlight only after signed export and real-device validation.
6. Add follow-up tasks to `todo.md` for anything deferred.
