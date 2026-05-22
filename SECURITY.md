# Security Policy

goDrive is a self-hosted file manager intended for small private deployments, such as a household or trusted team. It is not designed or supported as a public multi-tenant SaaS platform.

## Supported Versions

Until the project starts publishing stable releases, security fixes are made on the `main` branch only.

After the first stable release, supported versions will be listed here. Older development snapshots, forks, and locally modified builds are not covered unless the issue also reproduces on supported upstream code.

## Reporting a Vulnerability

Please do not report suspected vulnerabilities in public issues, discussions, pull requests, or chat logs.

Use GitHub's private vulnerability reporting for this repository if it is available. If private reporting is not available yet, contact the maintainer privately using the contact method listed on the repository owner's GitHub profile.

Include as much of the following as possible:

- Affected commit, version, or container image tag.
- Deployment mode, including whether goDrive is behind a reverse proxy.
- Clear reproduction steps or a minimal proof of concept.
- Impact, such as filesystem escape, authentication bypass, privilege escalation, cross-user data exposure, webhook SSRF, upload abuse, or stored/client-side code execution.
- Whether the issue requires an authenticated user, admin user, local network access, filesystem access, or a specific server configuration.

You should receive an initial response within 7 days when the project is actively maintained. The target is to confirm valid high-impact issues within 14 days and publish a fix or mitigation as soon as practical.

## Scope

In scope:

- Authentication, session, CSRF, API key, and WebDAV authorization flaws.
- Access to files outside a user's configured home root.
- Cross-user data exposure.
- Unsafe upload, trash, preview, thumbnail, or text-edit behavior.
- Webhook egress issues, including SSRF or unsafe redirects.
- Stored XSS or other browser-executed content in the web UI.
- Vulnerable dependencies that are reachable in goDrive's supported use cases.
- Container image vulnerabilities that materially affect goDrive deployments.

Out of scope:

- Issues that require full administrator access to the goDrive host.
- Issues caused by intentionally unsafe deployment choices, such as exposing a demo instance with default credentials.
- Attacks against reverse proxies, NAS devices, operating systems, or storage backends unless goDrive's documented configuration directly causes the issue.
- Denial-of-service reports based only on high request volume without a goDrive-specific amplification or bypass.
- Public multi-tenant isolation concerns that do not apply to the supported private self-hosted model.

## Safe Testing

Only test against systems you own or have explicit permission to assess. Do not access, modify, delete, or exfiltrate other people's files. Keep proof-of-concept payloads minimal and avoid persistence, destructive actions, or broad scanning.

## Deployment Guidance

Production instances should normally run behind HTTPS, set `GODRIVE_COOKIE_SECURE=true`, keep webhook private-network egress disabled unless explicitly needed, and use strong admin credentials. Public demo instances should use isolated disposable data, non-admin demo accounts where possible, automatic reset, and restrictive webhook settings.
