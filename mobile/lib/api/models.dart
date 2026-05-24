class ListPage {
  final String path;
  final List<FileEntry> entries;
  final int total;
  final int offset;
  final int limit;
  final bool hasMore;
  final String? nextCursor;

  const ListPage({
    required this.path,
    required this.entries,
    required this.total,
    required this.offset,
    required this.limit,
    required this.hasMore,
    this.nextCursor,
  });

  factory ListPage.fromJson(Map<String, dynamic> j) => ListPage(
        path: j['path'] as String,
        entries: (j['entries'] as List<dynamic>)
            .map((e) => FileEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
        total: j['total'] as int? ?? 0,
        offset: j['offset'] as int? ?? 0,
        limit: j['limit'] as int? ?? 200,
        hasMore: j['has_more'] as bool? ?? false,
        nextCursor: j['next_cursor'] as String?,
      );
}

class User {
  final int id;
  final String username;
  final bool isAdmin;
  final bool disabled;
  final String homeRoot;

  const User({
    required this.id,
    required this.username,
    required this.isAdmin,
    required this.disabled,
    required this.homeRoot,
  });

  factory User.fromJson(Map<String, dynamic> j) => User(
        id: j['id'] as int,
        username: j['username'] as String,
        isAdmin: j['is_admin'] as bool,
        disabled: j['disabled'] as bool,
        homeRoot: j['home_root'] as String,
      );
}

class FileEntry {
  final String name;
  final String path;
  final String type;
  final int size;
  final DateTime modifiedAt;
  final String? mimeType;
  final String? previewKind;

  bool get isDir => type == 'dir';

  const FileEntry({
    required this.name,
    required this.path,
    required this.type,
    required this.size,
    required this.modifiedAt,
    this.mimeType,
    this.previewKind,
  });

  factory FileEntry.fromJson(Map<String, dynamic> j) => FileEntry(
        name: j['name'] as String,
        path: j['path'] as String,
        type: j['type'] as String,
        size: j['size'] as int,
        modifiedAt: DateTime.parse(j['modified_at'] as String),
        mimeType: j['mime_type'] as String?,
        previewKind: j['preview_kind'] as String?,
      );
}

class TrashItem {
  final String id;
  final int userId;
  final String originalPath;
  final String originalName;
  final bool isDir;
  final int size;
  final DateTime deletedAt;

  const TrashItem({
    required this.id,
    required this.userId,
    required this.originalPath,
    required this.originalName,
    required this.isDir,
    required this.size,
    required this.deletedAt,
  });

  factory TrashItem.fromJson(Map<String, dynamic> j) => TrashItem(
        id: j['id'] as String,
        userId: j['user_id'] as int,
        originalPath: j['original_path'] as String,
        originalName: j['original_name'] as String,
        isDir: j['is_dir'] as bool,
        size: j['size'] as int,
        deletedAt: DateTime.parse(j['deleted_at'] as String),
      );
}

class TextPreview {
  final String path;
  final String name;
  final String content;
  final bool truncated;
  final int size;
  final int maxBytes;
  final String mimeType;
  final DateTime modifiedAt;

  const TextPreview({
    required this.path,
    required this.name,
    required this.content,
    required this.truncated,
    required this.size,
    required this.maxBytes,
    required this.mimeType,
    required this.modifiedAt,
  });

  factory TextPreview.fromJson(Map<String, dynamic> j) => TextPreview(
        path: j['path'] as String,
        name: j['name'] as String,
        content: j['content'] as String,
        truncated: j['truncated'] as bool,
        size: j['size'] as int,
        maxBytes: j['max_bytes'] as int,
        mimeType: j['mime_type'] as String,
        modifiedAt: DateTime.parse(j['modified_at'] as String),
      );
}

class ExifData {
  final Map<String, dynamic> fields;
  final double? gpsLat;
  final double? gpsLon;
  final bool hasGps;

  const ExifData({
    required this.fields,
    this.gpsLat,
    this.gpsLon,
    required this.hasGps,
  });

  factory ExifData.fromJson(Map<String, dynamic> j) => ExifData(
        fields: (j['fields'] as Map<String, dynamic>?) ?? const {},
        gpsLat: (j['gps_lat'] as num?)?.toDouble(),
        gpsLon: (j['gps_lon'] as num?)?.toDouble(),
        hasGps: j['has_gps'] as bool? ?? false,
      );
}

class AdminJob {
  final String id;
  final String type;
  final String status;
  final int done;
  final int total;
  final bool totalKnown;
  final int failed;
  final int deleted;
  final String? user;
  final String? scope;
  final bool cancelable;
  final String message;
  final DateTime startedAt;
  final DateTime? finishedAt;

  const AdminJob({
    required this.id,
    required this.type,
    required this.status,
    required this.done,
    required this.total,
    required this.totalKnown,
    required this.failed,
    required this.deleted,
    this.user,
    this.scope,
    required this.cancelable,
    required this.message,
    required this.startedAt,
    this.finishedAt,
  });

  factory AdminJob.fromJson(Map<String, dynamic> j) => AdminJob(
        id: j['id'] as String,
        type: j['type'] as String,
        status: j['status'] as String,
        done: j['done'] as int? ?? 0,
        total: j['total'] as int? ?? 0,
        totalKnown: j['total_known'] as bool? ?? false,
        failed: j['failed'] as int? ?? 0,
        deleted: j['deleted'] as int? ?? 0,
        user: j['user'] as String?,
        scope: j['scope'] as String?,
        cancelable: j['cancelable'] as bool? ?? false,
        message: j['message'] as String? ?? '',
        startedAt: DateTime.parse(j['started_at'] as String),
        finishedAt: j['finished_at'] != null
            ? DateTime.parse(j['finished_at'] as String)
            : null,
      );
}

class PreviewToolStatus {
  final String name;
  final String purpose;
  final bool available;
  final String? path;
  final String? error;

  const PreviewToolStatus({
    required this.name,
    required this.purpose,
    required this.available,
    this.path,
    this.error,
  });

  factory PreviewToolStatus.fromJson(Map<String, dynamic> j) =>
      PreviewToolStatus(
        name: j['name'] as String,
        purpose: j['purpose'] as String,
        available: j['available'] as bool? ?? false,
        path: j['path'] as String?,
        error: j['error'] as String?,
      );
}

class APIKey {
  final String id;
  final int userId;
  final String username;
  final String name;
  final DateTime createdAt;
  final DateTime? lastUsedAt;
  final DateTime? revokedAt;

  const APIKey({
    required this.id,
    required this.userId,
    required this.username,
    required this.name,
    required this.createdAt,
    this.lastUsedAt,
    this.revokedAt,
  });

  bool get revoked => revokedAt != null;

  factory APIKey.fromJson(Map<String, dynamic> j) => APIKey(
        id: j['id'] as String,
        userId: j['user_id'] as int,
        username: j['username'] as String? ?? '',
        name: j['name'] as String,
        createdAt: DateTime.parse(j['created_at'] as String),
        lastUsedAt: j['last_used_at'] != null
            ? DateTime.parse(j['last_used_at'] as String)
            : null,
        revokedAt: j['revoked_at'] != null
            ? DateTime.parse(j['revoked_at'] as String)
            : null,
      );
}

class Webhook {
  final String id;
  final String url;
  final List<String> events;
  final String description;
  final DateTime createdAt;
  final DateTime updatedAt;

  const Webhook({
    required this.id,
    required this.url,
    required this.events,
    required this.description,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Webhook.fromJson(Map<String, dynamic> j) => Webhook(
        id: j['id'] as String,
        url: j['url'] as String,
        events: ((j['events'] as List<dynamic>?) ?? const [])
            .map((e) => e as String)
            .toList(),
        description: j['description'] as String? ?? '',
        createdAt: DateTime.parse(j['created_at'] as String),
        updatedAt: DateTime.parse(j['updated_at'] as String),
      );
}

class AdminStats {
  final int indexedFiles;
  final int indexedDirs;
  final int indexedBytes;
  final int totalUsers;
  final int disabledUsers;
  final int trashItems;
  final int trashBytes;
  final int cacheFiles;
  final int cacheBytes;
  final int previewWorkers;
  final List<int> previewSizes;
  final List<PreviewToolStatus> previewTools;
  final bool watcherEnabled;
  final int watcherRoots;
  final bool reconcileEnabled;
  final String reconcileInterval;
  final AdminJob? currentJob;

  const AdminStats({
    required this.indexedFiles,
    required this.indexedDirs,
    required this.indexedBytes,
    required this.totalUsers,
    required this.disabledUsers,
    required this.trashItems,
    required this.trashBytes,
    required this.cacheFiles,
    required this.cacheBytes,
    required this.previewWorkers,
    required this.previewSizes,
    required this.previewTools,
    required this.watcherEnabled,
    required this.watcherRoots,
    required this.reconcileEnabled,
    required this.reconcileInterval,
    this.currentJob,
  });

  factory AdminStats.fromJson(Map<String, dynamic> j) {
    final index = j['index'] as Map<String, dynamic>? ?? {};
    final users = j['users'] as Map<String, dynamic>? ?? {};
    final trash = j['trash'] as Map<String, dynamic>? ?? {};
    final cache = j['preview_cache'] as Map<String, dynamic>? ?? {};
    final preview = j['preview'] as Map<String, dynamic>? ?? {};
    final watcher = j['watcher'] as Map<String, dynamic>? ?? {};
    final recon = j['reconciliation'] as Map<String, dynamic>? ?? {};
    return AdminStats(
      indexedFiles: index['indexed_files'] as int? ?? 0,
      indexedDirs: index['indexed_directories'] as int? ?? 0,
      indexedBytes: index['indexed_bytes'] as int? ?? 0,
      totalUsers: users['total'] as int? ?? 0,
      disabledUsers: users['disabled'] as int? ?? 0,
      trashItems: trash['items'] as int? ?? 0,
      trashBytes: trash['bytes'] as int? ?? 0,
      cacheFiles: cache['files'] as int? ?? 0,
      cacheBytes: cache['bytes'] as int? ?? 0,
      previewWorkers: (preview['workers'] as int?) ?? 0,
      previewSizes: ((preview['sizes'] as List<dynamic>?) ?? const [])
          .map((e) => e as int)
          .toList(),
      previewTools: ((preview['tools'] as List<dynamic>?) ?? const [])
          .map((e) => PreviewToolStatus.fromJson(e as Map<String, dynamic>))
          .toList(),
      watcherEnabled: watcher['enabled'] as bool? ?? false,
      watcherRoots: watcher['roots'] as int? ?? 0,
      reconcileEnabled: recon['enabled'] as bool? ?? false,
      reconcileInterval: recon['interval'] as String? ?? '',
      currentJob: j['current_job'] != null
          ? AdminJob.fromJson(j['current_job'] as Map<String, dynamic>)
          : null,
    );
  }
}
