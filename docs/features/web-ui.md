# Web UI

The web frontend is built with Svelte and SVAR components.

## File Browser

- List and grid views
- Drag-and-drop upload
- Breadcrumb navigation
- Bulk select, move, delete, and ZIP download
- Filename and path search (indexed)
- Responsive layout for phone browsers

## Upload Queue

- TUS resumable uploads
- Per-file progress indicator
- Upload state persists across page reloads
- Resumes automatically after connection drops

## Preview

- Image lightbox with zoom, pan, prev/next, original/preview toggle
- In-browser video player
- Native PDF viewer
- Text and Markdown preview with syntax highlighting
- RAW photo, Office document, and 3D model cached previews
- EXIF and GPS metadata display

## Trash

- Delete moves files to trash — no permanent loss on first delete
- Browse, restore, or permanently delete trash items
- Thumbnails shown for trashed files

## Admin

- Dashboard with storage stats and active job state
- User management (create, edit, delete users)
- API key management
- Webhook subscription management
- Reindex and preview warmup jobs with progress and cancellation
- Preview cache clear
