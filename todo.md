# goDrive Release TODO

This list tracks the remaining work before goDrive is ready to publish as an open source project and submit mobile apps to the stores.

## Critical Security

- [x] Replace or wrap WebDAV filesystem access so it uses goDrive's safe path resolver instead of `webdav.Dir(user.HomeRoot)`.
  - Current risk: symlinks inside a user root can expose paths outside the user root if OS permissions allow.
  - Add regression tests for WebDAV read/write/list/delete through symlinks that point outside the user root.
- [x] Add WebDAV-specific authentication hardening.
  - Add rate limiting for Basic Auth attempts.
  - Decide whether cookie-authenticated WebDAV mutating methods should require CSRF or whether WebDAV should only allow Basic/Bearer auth.
  - Document the supported auth modes for Finder, iOS Files, rclone, and browser clients.

## Security Hardening

- [x] Add webhook egress controls.
  - Validate webhook URLs.
  - Default to HTTPS-only for production.
  - Block private, loopback, link-local, multicast, and metadata-service IP ranges unless explicitly allowed.
  - Add a strict redirect policy.
  - Consider a deployment allowlist for trusted webhook destinations.
- [x] Fix or remove the ineffective `WriteContent` root re-check.
  - The caller validates today, but the helper currently ignores a failed defensive check.
- [x] Add HSTS when `GODRIVE_COOKIE_SECURE=true` or behind configured HTTPS.
- [x] Revisit cookie policy.
  - Consider `SameSite=Strict` for browser cookies if it does not break expected workflows.
  - Keep `HttpOnly` session cookies and non-HttpOnly CSRF token behavior documented.
- [x] Add security-focused automated checks.
  - `govulncheck`.
  - npm audit or OSV scan for web dependencies.
  - Flutter/Dart dependency audit.
  - Docker image scan.
  - GitHub Dependabot or Renovate.
- [ ] Reduce Docker image vulnerability noise.
  - Consider an additional minimal/no-preview production image only if it does not create divergent feature behavior.
  - Keep container vulnerability reports visible while avoiding unrelated base-image CVEs blocking every Dependabot PR.
- [x] Add a public `SECURITY.md`.
  - Include supported versions.
  - Include vulnerability disclosure process.
  - State intended deployment model: small private self-hosted instance, not public multi-tenant SaaS.
- [x] Expand security regression tests.
  - WebDAV symlink escape.
  - WebDAV mutating methods and auth modes.
  - Webhook URL validation and blocked egress targets.
  - Text edit confinement.
  - Admin-only API key and webhook actions.

## Platform Feature Parity

- [x] Bring mobile admin closer to web parity.
  - [x] API key management.
  - [x] Webhook management.
  - [x] Admin job cancellation.
  - [x] Scoped reindex inputs by user/path.
  - [x] Preview tool/status visibility if useful.
- [x] Bring mobile file features closer to web parity.
  - [x] EXIF/GPS display.
  - [x] Trash thumbnails.
  - [x] Bulk ZIP download or a clear mobile alternative.
    - Mobile keeps the current external-handler download flow for selected files instead of in-app ZIP handling.
  - [x] Text editing or explicitly mark web-only.
  - [x] 3D preview strategy: native/in-app viewer, webview, or explicitly external-only.
    - Mobile opens 3D files externally for now.
- [x] Add live refresh on mobile.
  - Use SSE or another lightweight event mechanism so uploads/external filesystem changes update open folders without manual refresh.
- [x] Decide feature support matrix.
  - Document which features are supported on web, Android, iOS, WebDAV, and CLI.
  - Mark intentional platform differences instead of letting them look unfinished.

## Mobile Release Readiness

- [x] Configure real Android release signing.
  - [x] Replace debug signing in `mobile/android/app/build.gradle.kts`.
  - [x] Add secure CI secret handling for keystore material.
  - [x] Keep release workflow dry-runnable without production credentials via a temporary CI keystore.
- [ ] Finalize Android package metadata.
  - [x] Confirm application ID: `eu.mljr.godrive`.
  - [x] App label: `goDrive`.
  - [x] Icons and adaptive icon.
  - [x] Splash/launch screen.
  - [x] Versioning policy.
  - [x] Permissions review.
  - [ ] Play Store listing screenshots/graphics.
  - [x] Play Store listing copy draft.
  - Release/store onboarding runbook: `docs/mobile-store-release.md`.
- [ ] Harden Android background uploads on physical devices.
  - Large files.
  - Large batches.
  - Network interruption and retry.
  - App backgrounding.
  - App kill.
  - Device reboot.
  - Notification permission denied.
- [ ] Prepare iOS production configuration.
  - Remove development-only ATS local networking unless intentionally supported.
  - Configure signing, bundle ID, capabilities, and App Group IDs for production.
  - Confirm share extension packaging in release builds.
  - Add signed IPA export once Apple distribution certificates and provisioning profiles exist.
- [ ] Harden iOS background uploads on physical devices.
  - Large files.
  - Large batches.
  - Lock screen behavior.
  - App swipe-away behavior.
  - Device reboot.
  - Network interruption and retry.
  - Stale background task reconciliation.
- [ ] Create store privacy materials.
  - Privacy policy.
  - Data safety answers for Play Store.
  - App Privacy answers for App Store.
  - Explain self-hosted server URL, auth token storage, uploads, and optional camera/photo/file access.
- [ ] Add release CI for mobile.
  - [x] Android release build.
  - [x] Android GitHub Release artifact upload on `v*` tags.
  - [x] Android Google Play internal-track upload gate, skipped unless explicitly enabled and credentials exist.
  - [x] iOS unsigned build/package validation.
  - [ ] iOS signed archive/export.
  - [ ] iOS TestFlight upload gate after signing is configured.
  - [x] Artifact retention and signing strategy for current dry-run workflows.

## Architecture And Maintainability

- [ ] Centralize the API contract.
  - [x] Add OpenAPI or another schema source of truth.
  - [x] Add route coverage validation so implemented API routes stay documented.
  - Generate or validate web TypeScript types and Flutter models against it.
  - Reduce drift between backend handlers, web API client, Flutter API client, Kotlin service, and Swift uploader.
- [ ] Version and isolate the mobile upload queue schema.
  - Avoid manual JSON shape duplication across Dart, Kotlin, and Swift.
  - Add migration handling for queued items from older app versions.
- [ ] Migrate Android project/plugins to Flutter built-in Kotlin DSL when dependencies support it.
  - Current Flutter builds auto-pin `android.builtInKotlin=false` and warn that the legacy Kotlin Gradle plugin path will fail in a future Flutter version.
- [ ] Code-split heavy web features.
  - Lazy-load 3D viewer and `3MFLoader`.
  - Lazy-load CodeMirror/editor bundles.
  - Re-check Vite chunk warnings after splitting.
- [ ] Decide the long-term WebDAV position.
  - Fully supported and indexed.
  - Supported but explicitly direct-filesystem only.
  - Disabled by default behind an environment flag.
- [ ] Consider a Nextcloud-compatible preview/capabilities layer.
  - Keep generic `/dav` focused on file operations.
  - Add OCS capability discovery and Nextcloud-style preview endpoints only if client compatibility justifies it.
- [ ] Improve indexing integration for WebDAV writes if WebDAV remains supported.
  - Refresh index after WebDAV create/update/move/delete, or document watcher/reconciliation delay.
- [ ] Review admin home-root management.
  - Add guardrails for dangerous roots.
  - Consider requiring roots under configured data root unless explicitly allowed.
- [ ] Add structured release notes and changelog process.

## Testing And Quality Gates

- [ ] Keep `make check` green.
  - Go fmt/vet/lint/tests.
  - Web type-check/tests/build.
  - Mobile analyze/tests.
- [x] Reduce hosted GitHub Actions minute usage.
  - Local-first workflow validation with `act`.
  - Scheduled/manual security scans.
  - Manual CI Docker build gate.
  - Tag/manual production image publishing.
  - Manual-only iOS dry-run builds.
- [x] Fix or update mobile Makefile targets if local Flutter no longer supports `--directory`.
  - Prefer `cd mobile && flutter test`.
  - Prefer `cd mobile && flutter analyze`.
- [ ] Add end-to-end browser tests for the main web workflows.
  - Login/logout.
  - Upload.
  - Browse/search.
  - Preview.
  - Trash restore/delete.
  - Admin jobs.
- [ ] Add mobile integration smoke tests where practical.
  - Login.
  - Browse.
  - Upload.
  - Open image/video/text.
- [ ] Run and record the manual release test plan against disposable data.
- [ ] Run 400k-file indexing and browsing performance smoke tests.
- [ ] Verify preview tool behavior in Docker and bare-metal installs.

## Open Source Readiness

- [ ] Add a license.
- [ ] Add `CONTRIBUTING.md`.
- [ ] Add issue templates.
- [ ] Add pull request template.
- [ ] Add `CODE_OF_CONDUCT.md` if desired.
- [ ] Clean repository artifacts before publishing.
  - No build outputs.
  - No local IDE files.
  - No generated temporary files.
  - No secrets or local `.env`.
- [ ] Review README for public users.
  - Quick start.
  - Docker deployment.
  - Reverse proxy/TLS guidance.
  - Backup/restore guidance.
  - Security caveats.
  - Mobile setup.
  - Troubleshooting.
- [ ] Add screenshots or short demo media for GitHub and app stores.
- [ ] Add architecture documentation.
  - Filesystem source of truth.
  - SQLite metadata.
  - Upload flow.
  - Preview generation.
  - Watcher/reconciliation.
  - Mobile background uploads.

## Web UI Responsiveness

- [ ] Treat the web app as desktop-first until a responsive pass is complete.
- [ ] Redesign the web app visually while keeping it dense, practical, and file-manager focused.
- [ ] Add a mobile navigation model for the sidebar, path bar, search, uploads, trash, and admin entry points.
- [ ] Make file grid/list/masonry views usable on phone widths without text overlap or horizontal scrolling.
- [ ] Rework action toolbars into touch-friendly grouped controls.
- [ ] Make preview, info, trash, admin, upload queue, and modal dialogs fit small screens.
- [ ] Add viewport checks for phone, tablet, and desktop to the release gate.

## Demo Instance

- [x] Design a public demo instance strategy.
  - [x] Always up to date with `main` or latest release.
  - [x] Securely isolated from production and developer machines.
  - [x] Uses disposable demo data only.
  - [x] Automatically resets to a known state on container restart.
  - [ ] Automatically resets on a schedule.
  - [x] Blocks or constrains dangerous features such as webhooks, WebDAV writes, admin root editing, API key creation, or arbitrary uploads if needed.
  - [x] Has upload size/type limits.
  - [x] Has rate limits and abuse protection.
  - [x] Runs behind HTTPS.
  - [x] Has health checks.
  - [x] Has clear public demo credentials with no sensitive data.
  - [ ] Has monitoring.
  - [x] Has CI/CD deployment from trusted branches only.
  - [x] Can be destroyed and recreated from code and seed data.
- [x] Decide demo hosting.
  - Home lab via `MrCodeEU/homelab-automation` repository dispatch.
- [x] Add demo seed data generation.
  - Expanded representative folder tree.
  - Images, reports, notes, structured data, and project/design samples.
  - No copyrighted/private content.
- [x] Expand demo seed data further.
  - [x] More varied image-like samples.
  - [x] Optional seeded Picsum JPEG photos for real raster-image preview coverage.
  - [x] More nested folders.
  - [x] More realistic documents, CSV, JSON, Markdown, code snippets, and release/store assets.
  - [x] Office/PDF/video fixtures for full preview pipeline coverage.
  - [x] Simple generated 3D OBJ fixtures.
  - [x] Tunable generation counts for image/model/deep-folder volume.
- [x] Keep the demo image on the full production preview toolchain.
  - Includes LibreOffice, ffmpeg, poppler, and libvips so the public demo exercises the real preview path.
- [x] Add limited read-only demo admin mode.
  - Demo account is admin so the UI is visible.
  - Read-only admin GET endpoints stay available.
  - Admin mutations remain blocked by demo mode.
- [x] Reindex demo data during startup so search and text/Markdown/CSV preview metadata are ready immediately.
- [ ] Add demo reset implementation.
  - Immutable seed volume or object store.
  - [x] Reset-on-container-start with `tmpfs` data/appdata.
  - Periodic container/database reset.
  - Reset button/job for maintainers.
  - Post-reset health verification.
- [x] Add demo-specific config profile.
  - Disable or sandbox risky admin features.
  - Short sessions.
  - Strict upload limits.
  - [x] Clear in-app banner that data is public and reset regularly.

## Suggested Work Order

1. Fix WebDAV confinement and auth hardening.
2. Add webhook egress validation.
3. Fix the file-edit helper guard.
4. Add missing security regression tests.
5. Decide and document the feature support matrix.
6. Bring mobile admin/file parity up to the chosen matrix.
7. Prepare Android/iOS production signing and store metadata.
8. Add open source project files and dependency/security automation.
9. Build the secure auto-resetting demo instance.
10. Run the full release gate: automated tests, manual test plan, security audit, mobile real-device tests, and Docker deployment smoke test.
