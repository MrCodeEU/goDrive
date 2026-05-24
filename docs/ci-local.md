# Local CI and GitHub Actions

This project keeps GitHub Actions for release orchestration, GitHub Releases, package publishing, and iOS/macOS builds. Routine Linux checks should be run locally first to conserve hosted Actions minutes.

## Local First

Run the direct project checks before using GitHub runners:

```sh
make test
make web-check
make web-test
make web-build
```

For mobile changes:

```sh
make mobile-test
flutter analyze
```

Use `act` to validate Linux GitHub Actions jobs locally:

```sh
act -l
act -W .github/workflows/ci.yml -j backend --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/ci.yml -j frontend --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/security.yml -j web-audit --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
```

Docker workflow checks can be run locally too, but they are heavier:

```sh
act -W .github/workflows/ci.yml -j docker --input docker_build=true --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/security.yml -j docker-image --input docker_image_scan=true --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
```

Use `-n` for a dry-run when validating workflow shape without executing the commands. In this environment, the explicit `127.0.0.1` cache/artifact bind flags avoid `act` trying to listen on its default `<nil>` address.

`act` cannot realistically validate iOS/macOS jobs on Linux. Use GitHub Actions for iOS only when a real iOS build or packaging check is needed.

## Hosted Actions Policy

- `CI` runs backend and frontend checks on code changes to `main` and PRs. It ignores documentation-only changes.
- The CI Docker build is manual only through `workflow_dispatch` with `docker_build=true`.
- `Security` runs weekly and manually. Docker image scanning is weekly or manual with `docker_image_scan=true`.
- `Mobile` runs only for `mobile/**` changes. Pull requests run analyze/tests; debug APK artifacts are produced on `main` or manual dispatch.
- `Docker Publish` publishes the demo image from relevant `main` changes. Main-branch demo publishes build only `linux/arm64` for the homelab target. Version tags and manual dispatch can build multi-arch images.
- Production Docker image publishing is tag/manual only.
- `Release` builds Android on tags/manual dispatch. iOS dry-run builds are manual only.

## Self-Hosted Runner Notes

A self-hosted Linux GitHub runner is the preferred next step if hosted minutes become a recurring bottleneck. It can run backend, frontend, Android, Docker build, and Docker scan jobs without duplicating pipeline definitions in Jenkins or GitLab.

Do not run untrusted fork PRs on a persistent self-hosted runner with secrets. Use self-hosted runners only for trusted branches/events unless the runner is ephemeral and isolated.
