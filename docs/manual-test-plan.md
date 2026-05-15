# Manual Test Plan

Use this plan after automated tests pass and before a real deployment. Test with disposable data first.

## Setup

Create a fresh test deployment:

```sh
export GODRIVE_DATA_ROOT=/tmp/godrive-manual-data
export GODRIVE_APPDATA_DIR=/tmp/godrive-manual-appdata
export GODRIVE_ADDR=127.0.0.1:8121
export GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me
go run ./cmd/godrive
```

Start the web UI:

```sh
make web-dev
```

For Android emulator testing, use `http://10.0.2.2:8121`.

## Test Data

Prepare:

- Small text file.
- Large text file over 512 KiB.
- JPEG with EXIF orientation.
- PNG.
- Short MP4.
- PDF.
- Markdown file containing links, headings, and raw HTML/script text.
- Nested folders with 1000+ generated files.
- Duplicate filenames to test conflict suffixing.

## Web Functional Tests

Authentication:

- Login as admin.
- Logout and confirm protected pages return to login.
- Create a non-admin user.
- Login as non-admin and verify admin modal/actions are unavailable.

File browsing:

- Open root folder.
- Create folder.
- Confirm created folders and completed uploads appear without refreshing the browser.
- Navigate using breadcrumbs.
- Refresh browser and confirm path is preserved.
- Switch list/grid/panel/search views.
- Open a folder with 1000+ entries and use load-more pagination.

File operations:

- Upload single file.
- Upload multiple files.
- Rename a file.
- Move a file between folders.
- Bulk move multiple files.
- Delete a file to trash.
- Restore from trash.
- Permanently delete from trash.
- Download a single file.
- Bulk download multiple files.
- Confirm conflict suffix behavior when a target name already exists.

Previews:

- Open image preview.
- Use next/previous image navigation.
- Zoom, reset zoom, drag pan, and toggle original/preview.
- Open video preview.
- Open PDF preview.
- Open text preview.
- Open Markdown preview and confirm raw script text is not executed.
- Clear preview cache from admin and confirm previews regenerate.

Search:

- Search by filename fragment.
- Search by folder/path fragment.
- Search for a literal `%` or `_` in a filename.
- Open a folder result.
- Open a file result and confirm its parent folder is selected.

Admin:

- View stats.
- Start full reindex.
- Confirm the progress bar disappears once the completed status is shown.
- Confirm Warm previews/Full reindex buttons are disabled while a job is running.
- Start a long job, click Cancel job, and confirm the job ends as canceled.
- Start preview warmup after the previous job has finished or been canceled.
- Create a user.
- Disable a user and confirm login fails.
- Reset a user's password.
- Change a user's home root and confirm watcher roots reload.

Webhooks:

- Create webhook subscription.
- Test webhook delivery.
- Upload a file and verify `upload.complete`.
- Move/delete/restore a file and verify corresponding events.
- Verify HMAC signature in a test receiver.
- Delete the webhook and confirm it no longer receives events.

## Mobile Tests

Login:

- Login with Android emulator.
- Login with a physical Android device if available.
- Confirm token persists after app restart.
- Logout and confirm local state clears.

Browsing:

- Navigate folders.
- Switch list/grid.
- Search indexed files.
- Open image viewer.
- Open video.
- Open text/Markdown.
- Open PDF/external viewer.

Foreground uploads:

- Pick multiple files.
- Upload while screen is awake.
- Interrupt network and retry.
- Restart app and confirm queued metadata behavior is clear.
- Confirm completed uploads appear in web UI.

Android background uploads:

- Start background upload from queued item.
- Grant notification permission when prompted.
- Confirm foreground-service notification appears.
- Background the app during upload.
- Reopen upload sheet and confirm progress refreshes.
- Confirm completion updates queue state and file appears on server.
- Deny notification permission and confirm the app reports that background upload cannot start.

iOS:

- Run foreground upload flow.
- Start background upload from a queued item.
- Background the app during upload and lock the device.
- Reopen the app and confirm upload sheet progress/completion refreshes.
- Start a background upload, swipe the app away after the upload task has started, wait for completion, reopen the app, and confirm the persisted queue shows either `Done` with the final path or a retryable failure.
- Start a background upload, reboot the device before completion, reopen the app, and confirm stale native task state is reconciled into a retryable queue item rather than staying stuck on `Background`.
- Interrupt network during a background upload, restore it, and verify retry/error behavior is understandable.
- Force-quit the app during a background upload and document the observed iOS behavior.

## Operational Tests

CLI:

```sh
godrive verify
godrive status
godrive reindex
godrive reindex --user admin
godrive reindex --user admin --path /Photos
godrive preview-warmup
godrive uploads cleanup --ttl 1h
godrive preview-cache clear
godrive admin create --username test --password change-me --root /tmp/godrive-manual-data/test
godrive admin reset-password --username test --password changed
```

Watcher and reconciliation:

- Add a file directly with shell under a watched user root.
- With the web UI open on the parent folder, confirm the file appears without refreshing the browser.
- Confirm it appears in search without manual reindex.
- Rename/delete a file directly with shell.
- Confirm the open web UI and index update.
- Simulate a watcher error if practical, or manually run reindex and confirm watcher health in admin stats.

Recovery:

- Stop server.
- Delete preview cache.
- Start server and confirm previews regenerate.
- Stop server.
- Delete incomplete `.part` files in upload dir.
- Start server and confirm upload cleanup/status remains healthy.
- Restore a copied SQLite/appdata backup into a fresh temp deployment and confirm login/files/trash metadata.

## Performance Smoke Tests

- Run the 400k index benchmark and record timings.
- Browse a 1000+ file folder in web UI.
- Search for a rare filename.
- Search for a broad folder fragment.
- Upload a large file over local network and confirm memory/CPU remain acceptable.
- Run preview warmup on a folder with mixed image/video/PDF files.

## Pass Criteria

- No data loss in file operation scenarios.
- No cross-user access.
- No unhandled server panic.
- Uploads can resume or fail clearly.
- Watcher/reconciliation eventually repairs external changes.
- Preview generation cannot hang the server.
- Admin can recover common operational issues from the CLI.
