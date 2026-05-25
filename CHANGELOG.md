# Changelog

All notable user-facing changes to goDrive should be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and releases use semantic version tags such as `v0.1.0`.

## [Unreleased]

### Added

- MIT license, contribution guide, issue templates, pull request template, and code of conduct.
- Public architecture overview.
- OpenAPI contract and generated web API schema types.
- Hardened public demo mode with seeded demo data, read-only demo admin surfaces, and deployment dispatch support.
- Mobile feature parity improvements for admin, file metadata, live refresh, and documented platform support.
- Android release build workflow with signed/dry-run AAB and APK artifacts.
- Local-first CI workflow guidance using `act`.
- Web favicon.
- Playwright end-to-end browser test suite for the web app.

### Changed

- Code-split heavy web editor and 3D viewer bundles so the initial web app load is smaller.
- README now documents pre-release status, deployment expectations, reverse proxy/TLS guidance, and release readiness links.
- Demo image uses the full production preview pipeline.

### Security

- Added WebDAV confinement hardening and WebDAV/auth rate limiting.
- Added webhook egress validation to reduce SSRF risk.
- Added HSTS/cookie policy hardening and expanded security checks.
- Added demo-mode restrictions for public deployments.
