---
title: goDrive — Self-Hosted File Manager for Families
description: Fast, private, filesystem-native file management. No cloud lock-in.
hide:
  - navigation
  - toc
---

<style>
.hero {
  text-align: center;
  padding: 4rem 1rem 3rem;
}
.hero h1 {
  font-size: 3rem;
  font-weight: 800;
  margin-bottom: 1rem;
  background: linear-gradient(135deg, #5c6bc0 0%, #7e57c2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}
.hero p {
  font-size: 1.25rem;
  max-width: 600px;
  margin: 0 auto 2rem;
  opacity: 0.85;
}
.hero-buttons {
  display: flex;
  gap: 1rem;
  justify-content: center;
  flex-wrap: wrap;
  margin-bottom: 3rem;
}
.btn-primary {
  background: #5c6bc0;
  color: white !important;
  padding: 0.75rem 2rem;
  border-radius: 0.5rem;
  font-weight: 600;
  text-decoration: none !important;
  transition: background 0.2s;
}
.btn-primary:hover { background: #3f51b5; }
.btn-secondary {
  background: transparent;
  color: inherit !important;
  padding: 0.75rem 2rem;
  border-radius: 0.5rem;
  font-weight: 600;
  text-decoration: none !important;
  border: 2px solid currentColor;
  opacity: 0.75;
  transition: opacity 0.2s;
}
.btn-secondary:hover { opacity: 1; }
.features-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 1.5rem;
  max-width: 1100px;
  margin: 0 auto 4rem;
  padding: 0 1rem;
}
.feature-card {
  border: 1px solid var(--md-default-fg-color--lightest);
  border-radius: 0.75rem;
  padding: 1.5rem;
  transition: border-color 0.2s, transform 0.2s;
}
.feature-card:hover {
  border-color: #5c6bc0;
  transform: translateY(-2px);
}
.feature-icon { font-size: 2rem; margin-bottom: 0.75rem; }
.feature-card h3 { margin: 0 0 0.5rem; font-size: 1.1rem; }
.feature-card p { margin: 0; opacity: 0.75; font-size: 0.95rem; }
.section-title {
  text-align: center;
  font-size: 2rem;
  font-weight: 700;
  margin-bottom: 0.5rem;
}
.section-sub {
  text-align: center;
  opacity: 0.7;
  margin-bottom: 2.5rem;
}
.quick-start {
  max-width: 720px;
  margin: 0 auto 4rem;
  padding: 0 1rem;
}
.badge-row {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
  flex-wrap: wrap;
  margin-bottom: 2rem;
}
</style>

<div class="hero">
  <h1>goDrive</h1>
  <p>Self-hosted file manager for families. Your files stay on your hardware, in normal folders, forever accessible without the app.</p>
  <div class="hero-buttons">
    <a href="getting-started/quick-start/" class="btn-primary">Get Started</a>
    <a href="https://github.com/MrCodeEU/goDrive" class="btn-secondary">View on GitHub</a>
  </div>
  <div class="badge-row">
    <img src="https://img.shields.io/github/license/MrCodeEU/goDrive?style=flat-square" alt="License">
    <img src="https://img.shields.io/github/go-mod/go-version/MrCodeEU/goDrive?style=flat-square" alt="Go version">
    <img src="https://img.shields.io/badge/platforms-web%20%7C%20android%20%7C%20iOS-blue?style=flat-square" alt="Platforms">
    <img src="https://img.shields.io/badge/storage-filesystem%20native-green?style=flat-square" alt="Storage">
  </div>
</div>

<p class="section-title">Everything you need, nothing you don't</p>
<p class="section-sub">Built for a small family setup. No cloud accounts, no subscriptions, no lock-in.</p>

<div class="features-grid">
  <div class="feature-card">
    <div class="feature-icon">📁</div>
    <h3>Filesystem Native</h3>
    <p>Files stay as normal files on disk. No content-addressed storage, no opaque UUIDs. Access them via SMB, rsync, or shell anytime.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">⬆️</div>
    <h3>Resumable Uploads</h3>
    <p>TUS protocol for all uploads. Drop a 20 GB video, lose wifi halfway — it resumes exactly where it left off.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🖼️</div>
    <h3>Rich Previews</h3>
    <p>Thumbnails and previews for images, RAW photos, video poster frames, PDFs, and Office documents. Inode-stable across renames.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">📱</div>
    <h3>Mobile Apps</h3>
    <p>Flutter apps for Android and iOS. Browse, view, and upload from your phone. Background uploads with foreground-service support.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🔍</div>
    <h3>Fast Search</h3>
    <p>Indexed filename and path search. Finds files instantly across hundreds of thousands of entries.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🔒</div>
    <h3>Security First</h3>
    <p>Argon2id passwords, CSRF protection, login rate limiting, path confinement. Designed for private home deployment.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🔔</div>
    <h3>Webhooks</h3>
    <p>HMAC-signed event delivery for upload, move, delete, and restore. Wire up automations without polling.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🐳</div>
    <h3>Docker Ready</h3>
    <p>Multi-stage, multi-arch builds for amd64 and arm64. Compose templates for Unraid and standard NAS setups.</p>
  </div>
  <div class="feature-card">
    <div class="feature-icon">🔄</div>
    <h3>External Sync</h3>
    <p>fsnotify watcher plus periodic reconciliation scanner picks up files added by SMB, rsync, or shell without manual reindex.</p>
  </div>
</div>

<p class="section-title">Up in minutes</p>
<p class="section-sub">One config file. One container. Done.</p>

<div class="quick-start">

```bash
# Copy and edit config
cp .env.example .env

# Set at minimum:
# GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me
# GODRIVE_DATA_ROOT=/path/to/your/files
# GODRIVE_ADDR=0.0.0.0:8121

# Start with Docker
docker compose -f deploy/docker-compose.yml up -d
```

[:material-rocket-launch: Full Quick Start Guide](getting-started/quick-start.md){ .btn-primary }

</div>
