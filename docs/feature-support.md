# Feature Support Matrix

This matrix tracks what goDrive intentionally supports across user-facing surfaces. `N/A` means the feature does not fit that surface, not that the implementation is incomplete.

Legend:

- `Yes`: supported directly.
- `Partial`: supported with known limits.
- `External`: handed to the operating system or another app.
- `N/A`: intentionally not applicable for that surface.
- `Planned`: desired, not implemented yet.

## Platforms

| Feature | Web | Android | iOS | WebDAV | CLI |
|---|---:|---:|---:|---:|---:|
| Browse files and folders | Yes | Yes | Yes | Yes | Partial |
| Create folders | Yes | Yes | Yes | Yes | N/A |
| Rename/move files | Yes | Yes | Yes | Yes | N/A |
| Delete to trash | Yes | Yes | Yes | Yes | N/A |
| Restore/delete trash items | Yes | Yes | Yes | N/A | N/A |
| Trash thumbnails | Yes | Yes | Yes | N/A | N/A |
| Upload files | Yes | Yes | Yes | Yes | N/A |
| Resumable uploads | Yes | Yes | Yes | Client-dependent | N/A |
| Background/share-sheet uploads | N/A | Partial | Partial | N/A | N/A |
| Bulk delete/move | Yes | Yes | Yes | Client-dependent | N/A |
| Bulk ZIP download | Yes | External | External | Client-dependent | N/A |
| Filename/path search | Yes | Yes | Yes | N/A | N/A |
| Indexed content search | Yes | Yes | Yes | N/A | N/A |
| Live folder refresh | Yes | Yes | Yes | Client-dependent | N/A |
| Image preview | Yes | Yes | Yes | Client-dependent | N/A |
| Nextcloud-compatible preview API | Planned | Planned | Planned | Planned | N/A |
| RAW/photo cached preview | Yes | Yes | Yes | Client-dependent | N/A |
| Video preview | Yes | Yes | Yes | Client-dependent | N/A |
| PDF preview | Yes | External | External | Client-dependent | N/A |
| Office preview | Yes | External | External | Client-dependent | N/A |
| Text/Markdown preview | Yes | Yes | Yes | Client-dependent | N/A |
| Text/Markdown editing | Yes | Yes | Yes | Client-dependent | N/A |
| 3D preview | Yes | External | External | Client-dependent | N/A |
| EXIF/GPS display | Yes | Yes | Yes | N/A | N/A |
| API key authentication | Yes | Yes | Yes | Yes | Yes |
| Cookie/session authentication | Yes | N/A | N/A | Partial | N/A |
| Basic authentication | N/A | N/A | N/A | Yes | N/A |
| Responsive phone web UI | Planned | N/A | N/A | N/A | N/A |

## Admin And Operations

| Feature | Web | Android | iOS | WebDAV | CLI |
|---|---:|---:|---:|---:|---:|
| Admin dashboard/stats | Yes | Yes | Yes | N/A | Yes |
| User management | Yes | Yes | Yes | N/A | Partial |
| API key management | Yes | Yes | Yes | N/A | N/A |
| Webhook management | Yes | Yes | Yes | N/A | N/A |
| Reindex all files | Yes | Yes | Yes | N/A | Yes |
| Scoped reindex by user/path | Yes | Yes | Yes | N/A | Yes |
| Preview warmup | Yes | Yes | Yes | N/A | Yes |
| Admin job cancellation | Yes | Yes | Yes | N/A | N/A |
| Preview cache clear | Yes | Yes | Yes | N/A | Yes |
| Upload cleanup | Automatic | Automatic | Automatic | N/A | Yes |
| Database/index verification | N/A | N/A | N/A | N/A | Yes |

## Intentional Differences

Generic WebDAV is a file-access compatibility surface for Finder, iOS Files, rclone, and similar clients. It intentionally does not expose goDrive search, admin operations, trash management UI, previews, EXIF panels, webhook management, or API key management. Those features belong to the web/mobile apps or CLI.

The web UI is currently a desktop-first surface. A responsive phone layout is release-blocking for the public web app, but the native Android and iOS apps remain the primary phone experience until that work is complete.

Nextcloud-compatible previews are possible as a future compatibility layer, but they should be separate from generic `/dav` behavior. A future implementation would likely expose Nextcloud-style capability and preview endpoints, such as OCS capability discovery and `/index.php/core/preview`, while keeping `/dav` focused on file operations.

Mobile uses the platform for some file-opening workflows. PDF, Office, 3D, and selected-file downloads are opened externally rather than fully rendered or packaged inside the app. This is intentional until native viewers or in-app ZIP handling provide a better mobile experience than the operating system.

The CLI is an operator and maintenance surface, not a daily file-browsing client. It focuses on status, verification, indexing, preview cache, upload cleanup, and admin recovery operations.

## Known Remaining Parity Work

- Harden Android and iOS background uploads on physical devices before store release.
- Redesign the web shell, toolbars, file lists, dialogs, and preview viewer for phone and tablet breakpoints.
- Decide whether mobile should eventually add in-app PDF/Office/3D viewers.
- Decide whether mobile should eventually save multi-file selections as a local ZIP instead of opening selected downloads externally.
- Add a Nextcloud-compatible preview/capabilities layer if compatibility with clients that expect Nextcloud preview APIs becomes a project goal.
- Improve indexing integration for WebDAV writes if WebDAV remains a first-class write path; today watcher/reconciliation can cover changes with some delay.
