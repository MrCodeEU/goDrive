// Canonical upload queue persistence schema shared with Android and iOS native services.
// Any change here must be mirrored in BackgroundUploadService.kt and AppDelegate.swift.

const String queuePrefKey = 'godrive_upload_queue';
const int queueSchemaVersion = 1;

// Outer envelope
const String qfEnvVersion = 'version';
const String qfEnvItems = 'items';

// Per-item fields
const String qfSchemaVersion = 'schema_version';
const String qfId = 'id';
const String qfFilePath = 'file_path';
const String qfName = 'name';
const String qfFileSize = 'file_size';
const String qfTargetPath = 'target_path';
const String qfStatus = 'status';
const String qfProgress = 'progress';
const String qfFinalPath = 'final_path';
const String qfTusUrl = 'tus_url';
const String qfError = 'error';
