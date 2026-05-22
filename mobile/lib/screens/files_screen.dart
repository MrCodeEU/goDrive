import 'dart:async';
import 'dart:io';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_staggered_grid_view/flutter_staggered_grid_view.dart';
import 'package:image_picker/image_picker.dart';
import 'package:provider/provider.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:url_launcher/url_launcher.dart';
import '../api/client.dart';
import '../api/events.dart';
import '../api/models.dart';
import '../api/tus.dart';
import '../state/auth_state.dart';
import '../state/upload_queue.dart';
import '../widgets/breadcrumb_bar.dart';
import '../widgets/file_tile.dart';
import '../widgets/upload_queue_sheet.dart';
import 'package:receive_sharing_intent/receive_sharing_intent.dart';
import 'admin_screen.dart';
import 'image_viewer_screen.dart';
import 'video_player_screen.dart';

bool _supportsThumbnail(FileEntry entry) {
  return switch (entry.previewKind) {
    'image' || 'raw' || 'video' || 'pdf' || 'office' => true,
    _ => false,
  };
}

enum _ViewMode { list, grid, masonry }

class FilesScreen extends StatefulWidget {
  final String path;
  const FilesScreen({super.key, required this.path});

  @override
  State<FilesScreen> createState() => _FilesScreenState();
}

class _FilesScreenState extends State<FilesScreen> with WidgetsBindingObserver {
  List<FileEntry> _entries = [];
  bool _loading = true;
  bool _hasMore = false;
  int _offset = 0;
  int _total = 0;
  String? _cursor;
  bool _loadingMore = false;
  String? _error;
  String _currentPath = '/';
  final List<String> _pathStack = [];
  final _searchCtrl = TextEditingController();
  bool _searching = false;
  _ViewMode _viewMode = _ViewMode.list;
  StreamSubscription? _sharingSubscription;
  final Set<String> _selectedPaths = {};
  final _scrollCtrl = ScrollController();
  FileEventService? _events;
  StreamSubscription<FileEvent>? _eventsSub;
  Timer? _liveRefreshTimer;
  bool _fabVisible = true;
  // Sort & filter
  String _sortBy = 'name'; // 'name', 'size', 'modified', 'type'
  bool _sortAsc = true;
  String _filterType =
      'all'; // 'all', 'folders', 'images', 'videos', 'documents', 'text', '3d', 'other'
  // Recently visited
  List<String> _recentPaths = [];
  // Search filter
  String _searchFilter = 'all';

  @override
  void initState() {
    super.initState();
    _currentPath = widget.path;
    _load(_currentPath);
    _initSharing();
    _scrollCtrl.addListener(_onScroll);
    _loadRecentPaths();
    WidgetsBinding.instance.addObserver(this);
    WidgetsBinding.instance.addPostFrameCallback((_) => _startFileEvents());
  }

  void _onScroll() {
    final visible =
        _scrollCtrl.position.userScrollDirection == ScrollDirection.forward ||
            _scrollCtrl.offset < 10;
    if (visible != _fabVisible) setState(() => _fabVisible = visible);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      _startFileEvents();
      _load(_currentPath);
      return;
    }
    if (state == AppLifecycleState.inactive ||
        state == AppLifecycleState.paused ||
        state == AppLifecycleState.detached) {
      unawaited(_stopFileEvents());
    }
  }

  ApiClient get _client => context.read<AuthState>().client!;
  TusClient get _tus => TusClient(_client);

  void _startFileEvents() {
    if (!mounted || _events != null) return;
    final service = FileEventService(client: _client);
    _events = service;
    _eventsSub = service.events.listen(_handleFileEvent);
    service.start();
  }

  Future<void> _stopFileEvents() async {
    _liveRefreshTimer?.cancel();
    _liveRefreshTimer = null;
    await _eventsSub?.cancel();
    _eventsSub = null;
    await _events?.dispose();
    _events = null;
  }

  void _handleFileEvent(FileEvent event) {
    if (!_eventAffectsCurrentFolder(event)) return;
    _liveRefreshTimer?.cancel();
    _liveRefreshTimer = Timer(const Duration(milliseconds: 250), () {
      _liveRefreshTimer = null;
      if (mounted) {
        _load(_currentPath);
      }
    });
  }

  bool _eventAffectsCurrentFolder(FileEvent event) {
    final paths = [
      event.data['path'],
      event.data['old_path'],
    ].whereType<String>();
    for (final raw in paths) {
      final path = _normalizePath(raw);
      if (_parentPath(path) == _currentPath ||
          path == _currentPath ||
          _currentPath.startsWith('$path/')) {
        return true;
      }
    }
    return false;
  }

  String _normalizePath(String path) {
    if (path.isEmpty) return '/';
    final normalized = path.startsWith('/') ? path : '/$path';
    return normalized.length > 1 && normalized.endsWith('/')
        ? normalized.substring(0, normalized.length - 1)
        : normalized;
  }

  String _parentPath(String path) {
    final normalized = _normalizePath(path);
    if (normalized == '/') return '/';
    final parts =
        normalized.split('/').where((part) => part.isNotEmpty).toList();
    parts.removeLast();
    return parts.isEmpty ? '/' : '/${parts.join('/')}';
  }

  Future<void> _load(String path) async {
    setState(() {
      _loading = true;
      _error = null;
      _offset = 0;
      _cursor = null;
      _hasMore = false;
    });
    try {
      final page = await _client.listFiles(path);
      if (mounted) {
        setState(() {
          _currentPath = page.path;
          _entries = page.entries;
          _hasMore = page.hasMore;
          _offset = page.entries.length;
          _cursor = page.nextCursor;
          _total = page.total;
          _loading = false;
        });
        if (path != '/') _saveRecentPath(path);
      }
    } on ApiException catch (e) {
      if (mounted) {
        setState(() {
          _error = e.message;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _loading = false;
        });
      }
    }
  }

  Future<void> _loadMore() async {
    if (!_hasMore || _loadingMore) return;
    setState(() => _loadingMore = true);
    try {
      final page = await _client.listFiles(_currentPath,
          offset: _offset, cursor: _cursor);
      if (mounted) {
        setState(() {
          _entries = [..._entries, ...page.entries];
          _hasMore = page.hasMore;
          _offset += page.entries.length;
          _cursor = page.nextCursor;
          _total = page.total;
          _loadingMore = false;
        });
      }
    } catch (_) {
      if (mounted) setState(() => _loadingMore = false);
    }
  }

  void _navigate(String path) {
    _pathStack.add(_currentPath);
    _load(path);
  }

  bool _pop() {
    if (_pathStack.isEmpty) return false;
    _load(_pathStack.removeLast());
    return true;
  }

  Future<void> _openFile(FileEntry entry) async {
    switch (entry.previewKind) {
      case 'image':
        final images = _entries.where((e) => e.previewKind == 'image').toList();
        final idx = images.indexOf(entry);
        if (!mounted) return;
        Navigator.of(context).push(MaterialPageRoute(
          builder: (_) => ImageViewerScreen(
            entries: images,
            initialIndex: idx < 0 ? 0 : idx,
            client: _client,
          ),
        ));
      case 'video':
        if (!mounted) return;
        Navigator.of(context).push(MaterialPageRoute(
          builder: (_) => VideoPlayerScreen(
            url: _client.rawFileUrl(entry.path),
            title: entry.name,
            headers: _client.authHeader,
          ),
        ));
      case 'pdf':
        await launchUrl(Uri.parse(_client.rawFileUrl(entry.path)),
            mode: LaunchMode.externalApplication);
      case 'text' || 'markdown':
        if (!mounted) return;
        _showTextPreview(entry);
      case 'raw' || 'office' || '3d':
        await launchUrl(Uri.parse(_client.rawFileUrl(entry.path)),
            mode: LaunchMode.externalApplication);
      default:
        await launchUrl(Uri.parse(_client.downloadUrl(entry.path)),
            mode: LaunchMode.externalApplication);
    }
  }

  Future<void> _showTextPreview(FileEntry entry) async {
    try {
      final preview = await _client.textPreview(entry.path);
      if (!mounted) return;
      showModalBottomSheet(
        context: context,
        isScrollControlled: true,
        builder: (_) => DraggableScrollableSheet(
          expand: false,
          initialChildSize: 0.8,
          builder: (_, scroll) => _TextPreviewSheet(
            entry: entry,
            preview: preview,
            client: _client,
            scrollController: scroll,
            onSaved: () => _load(_currentPath),
          ),
        ),
      );
    } catch (e) {
      if (mounted) _showSnack('Failed to load preview: $e');
    }
  }

  Future<void> _showExif(FileEntry entry) async {
    try {
      final exif = await _client.fileExif(entry.path);
      if (!mounted) return;
      showModalBottomSheet(
        context: context,
        isScrollControlled: true,
        builder: (_) => DraggableScrollableSheet(
          expand: false,
          initialChildSize: 0.75,
          builder: (_, scroll) => ListView(
            controller: scroll,
            padding: const EdgeInsets.all(16),
            children: [
              Text(entry.name, style: Theme.of(context).textTheme.titleMedium),
              if (exif.hasGps &&
                  exif.gpsLat != null &&
                  exif.gpsLon != null) ...[
                const SizedBox(height: 12),
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: const Icon(Icons.location_on_outlined),
                  title: Text(
                    '${exif.gpsLat!.toStringAsFixed(6)}, ${exif.gpsLon!.toStringAsFixed(6)}',
                  ),
                  trailing: const Icon(Icons.open_in_new),
                  onTap: () => _openMap(exif.gpsLat!, exif.gpsLon!),
                ),
              ],
              const SizedBox(height: 12),
              if (exif.fields.isEmpty)
                Text('No EXIF metadata found.',
                    style: Theme.of(context).textTheme.bodyMedium)
              else
                ...exif.fields.entries
                    .where((field) =>
                        field.key != 'GPSLatitude' &&
                        field.key != 'GPSLongitude' &&
                        field.key != 'GPSPosition')
                    .map((field) => _ExifRow(
                          label: _humanizeExifKey(field.key),
                          value: '${field.value}',
                        )),
            ],
          ),
        ),
      );
    } catch (e) {
      if (mounted) _showSnack('Failed to load EXIF: $e');
    }
  }

  Future<void> _openMap(double lat, double lon) async {
    final url = Uri.parse(
      'https://www.openstreetmap.org/?mlat=$lat&mlon=$lon#map=16/$lat/$lon',
    );
    await launchUrl(url, mode: LaunchMode.externalApplication);
  }

  String _humanizeExifKey(String key) {
    return key.replaceAllMapped(
      RegExp(r'([a-z0-9])([A-Z])'),
      (match) => '${match.group(1)} ${match.group(2)}',
    );
  }

  Future<void> _showFileActions(FileEntry entry) async {
    final action = await showModalBottomSheet<String>(
      context: context,
      builder: (_) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
                leading: const Icon(Icons.download_outlined),
                title: const Text('Download'),
                onTap: () => Navigator.pop(context, 'download')),
            if (!entry.isDir)
              ListTile(
                  leading: const Icon(Icons.open_in_new),
                  title: const Text('Open externally'),
                  onTap: () => Navigator.pop(context, 'open')),
            if (entry.previewKind == 'image' || entry.previewKind == 'raw')
              ListTile(
                  leading: const Icon(Icons.info_outline),
                  title: const Text('EXIF / GPS'),
                  onTap: () => Navigator.pop(context, 'exif')),
            ListTile(
              leading: const Icon(Icons.content_copy_outlined),
              title: const Text('Copy path'),
              onTap: () {
                Navigator.pop(context);
                Clipboard.setData(ClipboardData(text: entry.path));
                _showSnack('Path copied to clipboard');
              },
            ),
            ListTile(
                leading: const Icon(Icons.drive_file_rename_outline),
                title: const Text('Rename'),
                onTap: () => Navigator.pop(context, 'rename')),
            ListTile(
                leading: const Icon(Icons.drive_file_move_outline),
                title: const Text('Move to…'),
                onTap: () => Navigator.pop(context, 'move')),
            ListTile(
                leading: const Icon(Icons.delete_outline),
                title: const Text('Delete'),
                onTap: () => Navigator.pop(context, 'delete')),
          ],
        ),
      ),
    );
    if (action == null || !mounted) return;
    switch (action) {
      case 'download':
        await launchUrl(Uri.parse(_client.downloadUrl(entry.path)),
            mode: LaunchMode.externalApplication);
      case 'open':
        await launchUrl(Uri.parse(_client.rawFileUrl(entry.path)),
            mode: LaunchMode.externalApplication);
      case 'rename':
        await _rename(entry);
      case 'move':
        await _move(entry);
      case 'delete':
        await _delete(entry);
      case 'exif':
        await _showExif(entry);
    }
  }

  Future<void> _rename(FileEntry entry) async {
    final ctrl = TextEditingController(text: entry.name);
    final newName = await showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Rename'),
        content: TextField(
            controller: ctrl,
            decoration: const InputDecoration(border: OutlineInputBorder()),
            autofocus: true),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, ctrl.text.trim()),
              child: const Text('Rename')),
        ],
      ),
    );
    if (newName == null ||
        newName.isEmpty ||
        newName == entry.name ||
        !mounted) {
      return;
    }
    try {
      final dir = entry.path.contains('/')
          ? entry.path.substring(0, entry.path.lastIndexOf('/'))
          : '/';
      final newPath = dir == '/' ? '/$newName' : '$dir/$newName';
      await _client.move(entry.path, newPath);
      await _load(_currentPath);
    } catch (e) {
      if (mounted) _showSnack('Rename failed: $e');
    }
  }

  Future<void> _move(FileEntry entry) async {
    final targetPath =
        await Navigator.of(context).push<String>(MaterialPageRoute(
      builder: (_) =>
          _FolderPickerScreen(client: _client, initialPath: _currentPath),
    ));
    if (targetPath == null || !mounted) return;
    try {
      await _client.move(entry.path, '$targetPath/${entry.name}');
      await _load(_currentPath);
    } catch (e) {
      if (mounted) _showSnack('Move failed: $e');
    }
  }

  Future<void> _deleteEntry(FileEntry entry) async {
    try {
      await _client.deleteFile(entry.path);
      if (mounted) {
        setState(() => _selectedPaths.remove(entry.path));
        await _load(_currentPath);
      }
    } catch (e) {
      if (mounted) _showSnack('Delete failed: $e');
    }
  }

  Future<void> _delete(FileEntry entry) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Move to Trash'),
        content: Text('Move "${entry.name}" to trash?'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Delete')),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    await _deleteEntry(entry);
  }

  Future<void> _deleteSelected() async {
    final paths = List<String>.from(_selectedPaths);
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Move to Trash'),
        content: Text('Move ${paths.length} item(s) to trash?'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Delete')),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    for (final path in paths) {
      try {
        await _client.deleteFile(path);
      } catch (_) {}
    }
    if (mounted) {
      setState(() => _selectedPaths.clear());
      await _load(_currentPath);
    }
  }

  Future<void> _downloadSelected() async {
    for (final path in _selectedPaths) {
      final entry = _entries.firstWhere((e) => e.path == path,
          orElse: () => _entries.first);
      await launchUrl(Uri.parse(_client.downloadUrl(entry.path)),
          mode: LaunchMode.externalApplication);
    }
  }

  Future<void> _moveSelected() async {
    final targetPath =
        await Navigator.of(context).push<String>(MaterialPageRoute(
      builder: (_) =>
          _FolderPickerScreen(client: _client, initialPath: _currentPath),
    ));
    if (targetPath == null || !mounted) return;
    final paths = List<String>.from(_selectedPaths);
    for (final path in paths) {
      try {
        final name = path.split('/').last;
        await _client.move(path, '$targetPath/$name');
      } catch (_) {}
    }
    if (mounted) {
      setState(() => _selectedPaths.clear());
      await _load(_currentPath);
    }
  }

  Future<void> _createFolder() async {
    final ctrl = TextEditingController();
    final name = await showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Folder'),
        content: TextField(
            controller: ctrl,
            decoration: const InputDecoration(
                labelText: 'Folder name', border: OutlineInputBorder()),
            autofocus: true),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, ctrl.text.trim()),
              child: const Text('Create')),
        ],
      ),
    );
    if (name == null || name.isEmpty || !mounted) return;
    try {
      final newPath = _currentPath == '/' ? '/$name' : '$_currentPath/$name';
      await _client.mkdir(newPath);
      await _load(_currentPath);
    } catch (e) {
      if (mounted) _showSnack('Create folder failed: $e');
    }
  }

  void _initSharing() {
    // Files shared while app is open
    _sharingSubscription =
        ReceiveSharingIntent.instance.getMediaStream().listen(
              (files) => _handleSharedFiles(files),
            );
    // Files shared to launch the app (cold start)
    ReceiveSharingIntent.instance.getInitialMedia().then((files) {
      if (files.isNotEmpty) _handleSharedFiles(files);
      ReceiveSharingIntent.instance.reset();
    });
  }

  Future<void> _handleSharedFiles(List<SharedMediaFile> shared) async {
    if (shared.isEmpty || !mounted) return;
    final files =
        shared.map((f) => File(f.path)).where((f) => f.existsSync()).toList();
    if (files.isEmpty) return;

    // Let user pick target folder, defaulting to current path
    final targetPath = await _pickUploadFolder();
    if (targetPath == null || !mounted) return;

    final queue = context.read<UploadQueue>();
    queue.enqueue(files, targetPath);
    final tus = TusClient(_client);
    await queue.processQueue(tus, onComplete: (_) {
      if (mounted) _load(_currentPath);
    });
  }

  Future<String?> _pickUploadFolder() async {
    if (!mounted) return null;

    List<FileEntry> folders = [];
    String selected = _currentPath;
    bool loading = true;
    bool started = false;

    return showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setState) {
            if (!started) {
              started = true;
              _client.listFileTree().then((resp) {
                if (ctx.mounted) {
                  setState(() {
                    folders = resp.toList()
                      ..sort((a, b) => a.path.compareTo(b.path));
                    loading = false;
                  });
                }
              }).catchError((_) {
                if (ctx.mounted) setState(() => loading = false);
              });
            }

            return DraggableScrollableSheet(
              expand: false,
              initialChildSize: 0.6,
              maxChildSize: 0.9,
              builder: (_, scroll) => Column(
                children: [
                  Container(
                    margin: const EdgeInsets.symmetric(vertical: 8),
                    width: 36,
                    height: 4,
                    decoration: BoxDecoration(
                      color: Colors.grey.shade400,
                      borderRadius: BorderRadius.circular(2),
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                    child: Row(
                      children: [
                        const Icon(Icons.folder_outlined, size: 20),
                        const SizedBox(width: 8),
                        Text('Upload to folder',
                            style: Theme.of(ctx).textTheme.titleMedium),
                      ],
                    ),
                  ),
                  const Divider(height: 1),
                  Expanded(
                    child: loading
                        ? const Center(child: CircularProgressIndicator())
                        : ListView(
                            controller: scroll,
                            children: [
                              _FolderPickerTile(
                                path: '/',
                                isSelected: selected == '/',
                                onTap: () => setState(() => selected = '/'),
                              ),
                              ...folders
                                  .where((e) => e.path != '/')
                                  .map((e) => _FolderPickerTile(
                                        path: e.path,
                                        isSelected: selected == e.path,
                                        onTap: () =>
                                            setState(() => selected = e.path),
                                      )),
                            ],
                          ),
                  ),
                  const Divider(height: 1),
                  Padding(
                    padding: EdgeInsets.fromLTRB(
                        16, 8, 16, 8 + MediaQuery.of(ctx).viewInsets.bottom),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            selected,
                            style: Theme.of(ctx).textTheme.bodySmall,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        const SizedBox(width: 8),
                        TextButton(
                          onPressed: () => Navigator.pop(ctx),
                          child: const Text('Cancel'),
                        ),
                        const SizedBox(width: 8),
                        FilledButton(
                          onPressed: () => Navigator.pop(ctx, selected),
                          child: const Text('Upload here'),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            );
          },
        );
      },
    );
  }

  Future<void> _pickAndUpload() async {
    final source = await showModalBottomSheet<String>(
      context: context,
      builder: (_) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
                leading: const Icon(Icons.photo_library_outlined),
                title: const Text('Photo library'),
                onTap: () => Navigator.pop(context, 'photos')),
            ListTile(
                leading: const Icon(Icons.camera_alt_outlined),
                title: const Text('Camera'),
                onTap: () => Navigator.pop(context, 'camera')),
            ListTile(
                leading: const Icon(Icons.folder_open_outlined),
                title: const Text('Files'),
                onTap: () => Navigator.pop(context, 'files')),
          ],
        ),
      ),
    );
    if (source == null || !mounted) return;

    List<File> files = [];

    if (source == 'files') {
      final result = await FilePicker.platform.pickFiles(allowMultiple: true);
      if (result != null) {
        files = result.files
            .where((f) => f.path != null)
            .map((f) => File(f.path!))
            .toList();
      }
    } else {
      final picker = ImagePicker();
      if (source == 'camera') {
        final photo = await picker.pickImage(source: ImageSource.camera);
        if (photo != null) files = [File(photo.path)];
      } else {
        final media = await picker.pickMultipleMedia();
        files = media.map((x) => File(x.path)).toList();
      }
    }

    if (files.isEmpty || !mounted) return;

    final queue = context.read<UploadQueue>();
    queue.enqueue(files, _currentPath);
    queue.processQueue(_tus, onComplete: (path) {
      if (mounted && path == _currentPath) _load(_currentPath);
    });
    UploadQueueSheet.show(context);
  }

  Future<void> _search(String query) async {
    if (query.trim().isEmpty) {
      setState(() {
        _searching = false;
      });
      await _load(_currentPath);
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
      _searching = true;
      _searchFilter = 'all';
    });
    try {
      final results = await _client.searchFiles(query);
      if (mounted) {
        setState(() {
          _entries = results;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _loading = false;
        });
      }
    }
  }

  void _showSnack(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  // ── Sort / filter ──────────────────────────────────────────────────────────

  List<FileEntry> _sortedFilteredEntries() {
    var list = _entries.where((e) {
      if (_filterType == 'all') return true;
      if (_filterType == 'folders') return e.isDir;
      if (e.isDir) return false;
      final k = e.previewKind ?? '';
      return switch (_filterType) {
        'images' => k == 'image' || k == 'raw',
        'videos' => k == 'video',
        'documents' => k == 'pdf' || k == 'office',
        'text' => k == 'text' || k == 'markdown',
        '3d' => k == '3d',
        _ => ![
            'image',
            'raw',
            'video',
            'pdf',
            'office',
            'text',
            'markdown',
            '3d'
          ].contains(k),
      };
    }).toList();

    list.sort((a, b) {
      if (a.isDir != b.isDir) return a.isDir ? -1 : 1;
      int cmp = switch (_sortBy) {
        'size' => a.size.compareTo(b.size),
        'modified' => a.modifiedAt.compareTo(b.modifiedAt),
        'type' => (a.previewKind ?? '').compareTo(b.previewKind ?? ''),
        _ => a.name.toLowerCase().compareTo(b.name.toLowerCase()),
      };
      return _sortAsc ? cmp : -cmp;
    });
    return list;
  }

  void _showSortFilterSheet() {
    showModalBottomSheet(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setModalState) => SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Sort by', style: Theme.of(ctx).textTheme.titleSmall),
                const SizedBox(height: 8),
                Wrap(spacing: 8, children: [
                  for (final opt in [
                    ('name', 'Name'),
                    ('size', 'Size'),
                    ('modified', 'Modified'),
                    ('type', 'Type'),
                  ])
                    ChoiceChip(
                      label: Text(opt.$2),
                      selected: _sortBy == opt.$1,
                      onSelected: (_) => setState(() {
                        if (_sortBy == opt.$1) {
                          _sortAsc = !_sortAsc;
                        } else {
                          _sortBy = opt.$1;
                          _sortAsc = true;
                        }
                        setModalState(() {});
                      }),
                      avatar: _sortBy == opt.$1
                          ? Icon(
                              _sortAsc
                                  ? Icons.arrow_upward
                                  : Icons.arrow_downward,
                              size: 14,
                            )
                          : null,
                    ),
                ]),
                const SizedBox(height: 16),
                Text('Filter', style: Theme.of(ctx).textTheme.titleSmall),
                const SizedBox(height: 8),
                Wrap(spacing: 8, children: [
                  for (final opt in [
                    ('all', 'All'),
                    ('folders', 'Folders'),
                    ('images', 'Images'),
                    ('videos', 'Videos'),
                    ('documents', 'Docs'),
                    ('text', 'Text'),
                    ('3d', '3D'),
                    ('other', 'Other'),
                  ])
                    ChoiceChip(
                      label: Text(opt.$2),
                      selected: _filterType == opt.$1,
                      onSelected: (_) => setState(() {
                        _filterType = opt.$1;
                        setModalState(() {});
                      }),
                    ),
                ]),
                const SizedBox(height: 8),
              ],
            ),
          ),
        ),
      ),
    );
  }

  // ── Recent paths ───────────────────────────────────────────────────────────

  Future<void> _loadRecentPaths() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      if (mounted) {
        setState(() {
          _recentPaths = prefs.getStringList('godrive_recent_paths') ?? [];
        });
      }
    } catch (_) {}
  }

  Future<void> _saveRecentPath(String path) async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final updated =
          [path, ..._recentPaths.where((p) => p != path)].take(5).toList();
      await prefs.setStringList('godrive_recent_paths', updated);
      if (mounted) setState(() => _recentPaths = updated);
    } catch (_) {}
  }

  // ── Search filter ──────────────────────────────────────────────────────────

  List<FileEntry> get _filteredSearchResults {
    if (_searchFilter == 'all') return _entries;
    return _entries.where((e) {
      if (_searchFilter == 'folders') return e.isDir;
      if (e.isDir) return false;
      final k = e.previewKind ?? '';
      return switch (_searchFilter) {
        'images' => k == 'image' || k == 'raw',
        'videos' => k == 'video',
        'documents' => k == 'pdf' || k == 'office',
        'text' => k == 'text' || k == 'markdown',
        _ => false,
      };
    }).toList();
  }

  @override
  Widget build(BuildContext context) {
    final queue = context.watch<UploadQueue>();
    final user = context.watch<AuthState>().user;

    return PopScope(
      canPop: _pathStack.isEmpty,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _pop();
      },
      child: Scaffold(
        appBar: _selectedPaths.isNotEmpty
            ? AppBar(
                leading: IconButton(
                  icon: const Icon(Icons.close),
                  onPressed: () => setState(() => _selectedPaths.clear()),
                ),
                title: Text('${_selectedPaths.length} selected'),
                actions: [
                  IconButton(
                      icon: const Icon(Icons.download_outlined),
                      onPressed: _downloadSelected,
                      tooltip: 'Download'),
                  IconButton(
                      icon: const Icon(Icons.drive_file_move_outline),
                      onPressed: _moveSelected,
                      tooltip: 'Move'),
                  IconButton(
                      icon: const Icon(Icons.delete_outline),
                      onPressed: _deleteSelected,
                      tooltip: 'Move to trash'),
                ],
              )
            : AppBar(
                title: _searching
                    ? TextField(
                        controller: _searchCtrl,
                        autofocus: true,
                        decoration: const InputDecoration(
                            hintText: 'Search files…',
                            border: InputBorder.none),
                        onSubmitted: _search,
                      )
                    : const Text('goDrive'),
                actions: [
                  if (!_searching) ...[
                    IconButton(
                      icon: const Icon(Icons.tune_outlined),
                      tooltip: 'Sort & filter',
                      onPressed: _showSortFilterSheet,
                    ),
                    IconButton(
                      icon: Icon(_viewModeIcon(_viewMode)),
                      tooltip: _viewModeLabel(_viewMode),
                      onPressed: () => setState(() {
                        _viewMode = _ViewMode.values[
                            (_viewMode.index + 1) % _ViewMode.values.length];
                      }),
                    ),
                    IconButton(
                        icon: const Icon(Icons.search),
                        onPressed: () => setState(() => _searching = true)),
                  ],
                  if (_searching)
                    IconButton(
                        icon: const Icon(Icons.close),
                        onPressed: () {
                          _searchCtrl.clear();
                          setState(() => _searching = false);
                          _load(_currentPath);
                        }),
                  if (queue.hasActive)
                    Stack(
                      alignment: Alignment.center,
                      children: [
                        IconButton(
                            icon: const Icon(Icons.upload_outlined),
                            onPressed: () => UploadQueueSheet.show(context)),
                        Positioned(
                            top: 8,
                            right: 8,
                            child: Container(
                              width: 8,
                              height: 8,
                              decoration: BoxDecoration(
                                  color: Theme.of(context).colorScheme.primary,
                                  shape: BoxShape.circle),
                            )),
                      ],
                    )
                  else
                    IconButton(
                        icon: const Icon(Icons.upload_outlined),
                        onPressed: () => UploadQueueSheet.show(context)),
                  PopupMenuButton<String>(
                    onSelected: (v) async {
                      if (v == 'trash') _showTrash();
                      if (v == 'admin') _showAdmin();
                      if (v == 'logout') context.read<AuthState>().logout();
                      if (v.startsWith('recent:')) {
                        _navigate(v.substring(7));
                        return;
                      }
                    },
                    itemBuilder: (_) => [
                      const PopupMenuItem(value: 'trash', child: Text('Trash')),
                      if (user?.isAdmin == true)
                        const PopupMenuItem(
                            value: 'admin', child: Text('Admin')),
                      PopupMenuItem(
                          value: 'logout',
                          child: Text('Sign out (${user?.username ?? ''})')),
                      if (_recentPaths.isNotEmpty) ...[
                        const PopupMenuDivider(),
                        const PopupMenuItem(
                          value: '__recent_header',
                          enabled: false,
                          child: Text('Recent', style: TextStyle(fontSize: 11)),
                        ),
                        for (final p in _recentPaths)
                          PopupMenuItem(
                            value: 'recent:$p',
                            child: Text(
                              p,
                              style: const TextStyle(fontSize: 13),
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                      ],
                    ],
                  ),
                ],
              ),
        body: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (!_searching)
              BreadcrumbBar(path: _currentPath, onNavigate: _navigate),
            if (_hasMore)
              MaterialBanner(
                content: Text('Showing $_offset of $_total items.'),
                actions: [
                  TextButton(
                      onPressed: _loadingMore ? null : _loadMore,
                      child: _loadingMore
                          ? const SizedBox(
                              width: 16,
                              height: 16,
                              child: CircularProgressIndicator(strokeWidth: 2))
                          : const Text('Load more')),
                ],
              ),
            Expanded(child: _body()),
          ],
        ),
        floatingActionButton: AnimatedScale(
          scale: _fabVisible ? 1.0 : 0.0,
          duration: const Duration(milliseconds: 200),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              FloatingActionButton.small(
                heroTag: 'folder',
                onPressed: _createFolder,
                tooltip: 'New folder',
                child: const Icon(Icons.create_new_folder_outlined),
              ),
              const SizedBox(height: 8),
              FloatingActionButton(
                heroTag: 'upload',
                onPressed: _pickAndUpload,
                tooltip: 'Upload files',
                child: const Icon(Icons.upload_file),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _body() {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
            const SizedBox(height: 12),
            FilledButton.tonal(
              onPressed: () => _load(_currentPath),
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (_entries.isEmpty && !_loading) {
      return RefreshIndicator(
        onRefresh: () => _load(_currentPath),
        child: SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          child: SizedBox(
            height: MediaQuery.of(context).size.height * 0.6,
            child: const _EmptyFolderState(),
          ),
        ),
      );
    }
    if (_viewMode == _ViewMode.grid && !_searching) return _gridBody();
    if (_viewMode == _ViewMode.masonry && !_searching) return _masonryBody();

    // Search results with filter chips
    if (_searching) {
      final results = _filteredSearchResults;
      return Column(
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
            child: Row(children: [
              for (final opt in [
                ('all', 'All'),
                ('folders', 'Folders'),
                ('images', 'Images'),
                ('videos', 'Videos'),
                ('documents', 'Docs'),
                ('text', 'Text'),
              ])
                Padding(
                  padding: const EdgeInsets.only(right: 6),
                  child: FilterChip(
                    label: Text(opt.$2),
                    selected: _searchFilter == opt.$1,
                    onSelected: (_) => setState(() => _searchFilter = opt.$1),
                  ),
                ),
            ]),
          ),
          Expanded(
            child: results.isEmpty
                ? const Center(child: Text('No results'))
                : ListView.separated(
                    controller: _scrollCtrl,
                    itemCount: results.length,
                    separatorBuilder: (_, __) =>
                        const Divider(height: 1, indent: 68),
                    itemBuilder: (context, i) {
                      final entry = results[i];
                      return FileTile(
                        entry: entry,
                        thumbnailUrl: _supportsThumbnail(entry)
                            ? _client.thumbnailUrl(entry.path, 96)
                            : '',
                        authHeaders: _client.authHeader,
                        onTap: () => entry.isDir
                            ? _navigate(entry.path)
                            : _openFile(entry),
                        onLongPress: () => _showFileActions(entry),
                        isSelected: false,
                        inSelectionMode: false,
                      );
                    },
                  ),
          ),
        ],
      );
    }

    final displayEntries = _sortedFilteredEntries();
    return RefreshIndicator(
      onRefresh: () async {
        await _load(_currentPath);
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Folder refreshed'),
              duration: Duration(seconds: 1),
              behavior: SnackBarBehavior.floating,
            ),
          );
        }
      },
      child: ListView.separated(
        controller: _scrollCtrl,
        itemCount: displayEntries.length,
        separatorBuilder: (_, __) => const Divider(height: 1, indent: 68),
        itemBuilder: (context, i) {
          final entry = displayEntries[i];
          final isSelected = _selectedPaths.contains(entry.path);
          final inSelectionMode = _selectedPaths.isNotEmpty;
          final tile = FileTile(
            entry: entry,
            thumbnailUrl: _supportsThumbnail(entry)
                ? _client.thumbnailUrl(entry.path, 96)
                : '',
            authHeaders: _client.authHeader,
            onTap: () {
              if (inSelectionMode) {
                setState(() {
                  if (isSelected) {
                    _selectedPaths.remove(entry.path);
                  } else {
                    _selectedPaths.add(entry.path);
                  }
                });
              } else {
                entry.isDir ? _navigate(entry.path) : _openFile(entry);
              }
            },
            onLongPress: () {
              setState(() => _selectedPaths.add(entry.path));
            },
            isSelected: isSelected,
            inSelectionMode: inSelectionMode,
          );
          if (inSelectionMode) return tile;
          return Dismissible(
            key: ValueKey(entry.path),
            direction: DismissDirection.endToStart,
            background: Container(
              alignment: Alignment.centerRight,
              padding: const EdgeInsets.only(right: 20),
              color: Theme.of(context).colorScheme.errorContainer,
              child: Icon(Icons.delete_outline,
                  color: Theme.of(context).colorScheme.onErrorContainer),
            ),
            confirmDismiss: (_) async {
              return await showDialog<bool>(
                    context: context,
                    builder: (ctx) => AlertDialog(
                      title: const Text('Move to trash?'),
                      content: Text('Move "${entry.name}" to trash?'),
                      actions: [
                        TextButton(
                            onPressed: () => Navigator.pop(ctx, false),
                            child: const Text('Cancel')),
                        FilledButton(
                            onPressed: () => Navigator.pop(ctx, true),
                            child: const Text('Trash')),
                      ],
                    ),
                  ) ??
                  false;
            },
            onDismissed: (_) {
              _deleteEntry(entry);
            },
            child: tile,
          );
        },
      ),
    );
  }

  Widget _gridBody() {
    final displayEntries = _sortedFilteredEntries();
    return RefreshIndicator(
      onRefresh: () async {
        await _load(_currentPath);
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Folder refreshed'),
              duration: Duration(seconds: 1),
              behavior: SnackBarBehavior.floating,
            ),
          );
        }
      },
      child: GridView.builder(
        padding: const EdgeInsets.all(2),
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 3,
          mainAxisSpacing: 2,
          crossAxisSpacing: 2,
        ),
        itemCount: displayEntries.length,
        itemBuilder: (context, i) {
          final entry = displayEntries[i];
          final isSelected = _selectedPaths.contains(entry.path);
          final inSelectionMode = _selectedPaths.isNotEmpty;
          return _GridCell(
            entry: entry,
            client: _client,
            isSelected: isSelected,
            onTap: () {
              if (inSelectionMode) {
                setState(() {
                  if (isSelected) {
                    _selectedPaths.remove(entry.path);
                  } else {
                    _selectedPaths.add(entry.path);
                  }
                });
              } else {
                entry.isDir ? _navigate(entry.path) : _openFile(entry);
              }
            },
            onLongPress: () {
              if (inSelectionMode) {
                _showFileActions(entry);
              } else {
                setState(() => _selectedPaths.add(entry.path));
              }
            },
          );
        },
      ),
    );
  }

  IconData _viewModeIcon(_ViewMode mode) {
    return switch (mode) {
      _ViewMode.list => Icons.grid_view,
      _ViewMode.grid => Icons.view_module,
      _ViewMode.masonry => Icons.view_list,
    };
  }

  String _viewModeLabel(_ViewMode mode) {
    return switch (mode) {
      _ViewMode.list => 'Grid view',
      _ViewMode.grid => 'Masonry view',
      _ViewMode.masonry => 'List view',
    };
  }

  Widget _masonryBody() {
    final displayEntries = _sortedFilteredEntries();
    return RefreshIndicator(
      onRefresh: () async {
        await _load(_currentPath);
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Folder refreshed'),
              duration: Duration(seconds: 1),
              behavior: SnackBarBehavior.floating,
            ),
          );
        }
      },
      child: MasonryGridView.count(
        crossAxisCount: 2,
        mainAxisSpacing: 2,
        crossAxisSpacing: 2,
        padding: const EdgeInsets.all(2),
        itemCount: displayEntries.length,
        itemBuilder: (context, i) {
          final entry = displayEntries[i];
          final isSelected = _selectedPaths.contains(entry.path);
          final inSelectionMode = _selectedPaths.isNotEmpty;
          final hasThumbnail = _supportsThumbnail(entry);
          return GestureDetector(
            onTap: () {
              if (inSelectionMode) {
                setState(() {
                  if (isSelected) {
                    _selectedPaths.remove(entry.path);
                  } else {
                    _selectedPaths.add(entry.path);
                  }
                });
              } else {
                entry.isDir ? _navigate(entry.path) : _openFile(entry);
              }
            },
            onLongPress: () {
              if (inSelectionMode) {
                _showFileActions(entry);
              } else {
                setState(() => _selectedPaths.add(entry.path));
              }
            },
            child: Stack(
              children: [
                ClipRRect(
                  borderRadius: BorderRadius.circular(4),
                  child: entry.isDir
                      ? Container(
                          height: 80,
                          color: const Color(0xFFFFF8E1),
                          child: const Center(
                            child: Icon(Icons.folder_rounded,
                                color: Color(0xFFFFB300), size: 40),
                          ),
                        )
                      : hasThumbnail
                          ? CachedNetworkImage(
                              imageUrl: _client.thumbnailUrl(entry.path, 420),
                              httpHeaders: _client.authHeader,
                              fit: BoxFit.fitWidth,
                              width: double.infinity,
                              placeholder: (_, __) => Container(
                                height: 120,
                                color: const Color(0xFF1A2230),
                                child: const Center(
                                  child: SizedBox(
                                    width: 20,
                                    height: 20,
                                    child: CircularProgressIndicator(
                                        strokeWidth: 2, color: Colors.white24),
                                  ),
                                ),
                              ),
                              errorWidget: (_, __, ___) =>
                                  _masonryFallback(entry),
                            )
                          : _masonryFallback(entry),
                ),
                if (isSelected)
                  Positioned.fill(
                    child: ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: Container(
                          color: Theme.of(context)
                              .colorScheme
                              .primary
                              .withValues(alpha: 0.35)),
                    ),
                  ),
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: 0,
                  child: ClipRRect(
                    borderRadius:
                        const BorderRadius.vertical(bottom: Radius.circular(4)),
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 6, vertical: 4),
                      decoration: const BoxDecoration(
                        gradient: LinearGradient(
                          begin: Alignment.bottomCenter,
                          end: Alignment.topCenter,
                          colors: [Color(0xCC000000), Colors.transparent],
                        ),
                      ),
                      child: Text(
                        entry.name,
                        style: const TextStyle(
                            color: Colors.white, fontSize: 10, height: 1.2),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ),
                ),
                if (isSelected)
                  Positioned(
                    top: 6,
                    right: 6,
                    child: Container(
                      decoration: BoxDecoration(
                        color: Theme.of(context).colorScheme.primary,
                        shape: BoxShape.circle,
                      ),
                      padding: const EdgeInsets.all(2),
                      child: const Icon(Icons.check,
                          color: Colors.white, size: 16),
                    ),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _masonryFallback(FileEntry entry) {
    final (icon, color) = switch (entry.previewKind) {
      'image' || 'raw' => (Icons.image_outlined, const Color(0xFF0B6F68)),
      'video' => (Icons.videocam_outlined, const Color(0xFF6B4BD8)),
      'pdf' => (Icons.picture_as_pdf_outlined, const Color(0xFFB73232)),
      'office' => (Icons.description_outlined, const Color(0xFF2563EB)),
      '3d' => (Icons.view_in_ar_outlined, const Color(0xFF16845B)),
      'text' || 'markdown' => (
          Icons.text_snippet_outlined,
          const Color(0xFF50606B)
        ),
      _ => (Icons.insert_drive_file_outlined, const Color(0xFF50606B)),
    };
    return Container(
      height: 80,
      color: const Color(0xFFF0F4F5),
      child: Center(child: Icon(icon, size: 32, color: color)),
    );
  }

  void _showTrash() {
    Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => _TrashScreen(client: _client),
    ));
  }

  void _showAdmin() {
    Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => AdminScreen(client: _client),
    ));
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    unawaited(_stopFileEvents());
    _sharingSubscription?.cancel();
    _searchCtrl.dispose();
    _scrollCtrl.dispose();
    super.dispose();
  }
}

class _TrashScreen extends StatefulWidget {
  final ApiClient client;
  const _TrashScreen({required this.client});

  @override
  State<_TrashScreen> createState() => _TrashScreenState();
}

class _TrashScreenState extends State<_TrashScreen> {
  List<TrashItem> _items = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    _items = await widget.client.listTrash();
    if (mounted) setState(() => _loading = false);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Trash')),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _items.isEmpty
              ? const Center(child: Text('Trash is empty'))
              : ListView.builder(
                  itemCount: _items.length,
                  itemBuilder: (_, i) {
                    final item = _items[i];
                    return ListTile(
                      leading: _TrashThumb(item: item, client: widget.client),
                      title: Text(item.originalName),
                      subtitle: Text(item.originalPath),
                      trailing: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          IconButton(
                              icon: const Icon(Icons.restore_outlined),
                              onPressed: () async {
                                await widget.client.restoreTrash(item.id);
                                await _load();
                              }),
                          IconButton(
                              icon: const Icon(Icons.delete_forever_outlined),
                              onPressed: () async {
                                await widget.client.deleteTrash(item.id);
                                await _load();
                              }),
                        ],
                      ),
                    );
                  },
                ),
    );
  }
}

class _TrashThumb extends StatelessWidget {
  final TrashItem item;
  final ApiClient client;
  const _TrashThumb({required this.item, required this.client});

  @override
  Widget build(BuildContext context) {
    if (item.isDir) {
      return const SizedBox(
        width: 44,
        height: 44,
        child: Icon(Icons.folder_outlined),
      );
    }
    return ClipRRect(
      borderRadius: BorderRadius.circular(4),
      child: SizedBox(
        width: 44,
        height: 44,
        child: CachedNetworkImage(
          imageUrl: client.trashThumbnailUrl(item.id, 96),
          httpHeaders: client.authHeader,
          fit: BoxFit.cover,
          placeholder: (_, __) => const Center(
            child: SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
          ),
          errorWidget: (_, __, ___) =>
              const Icon(Icons.insert_drive_file_outlined),
        ),
      ),
    );
  }
}

class _TextPreviewSheet extends StatefulWidget {
  final FileEntry entry;
  final TextPreview preview;
  final ApiClient client;
  final ScrollController scrollController;
  final Future<void> Function() onSaved;

  const _TextPreviewSheet({
    required this.entry,
    required this.preview,
    required this.client,
    required this.scrollController,
    required this.onSaved,
  });

  @override
  State<_TextPreviewSheet> createState() => _TextPreviewSheetState();
}

class _TextPreviewSheetState extends State<_TextPreviewSheet> {
  late final TextEditingController _controller;
  bool _editing = false;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(text: widget.preview.content);
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      await widget.client.saveFileContent(widget.entry.path, _controller.text);
      await widget.onSaved();
      if (mounted) {
        setState(() {
          _editing = false;
          _saving = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() => _saving = false);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Save failed: $e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final canEdit = !widget.preview.truncated && !_saving;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 8, 4),
          child: Row(
            children: [
              Expanded(
                child: Text(widget.entry.name,
                    style: Theme.of(context).textTheme.titleMedium),
              ),
              if (_editing) ...[
                TextButton(
                  onPressed: _saving
                      ? null
                      : () {
                          _controller.text = widget.preview.content;
                          setState(() => _editing = false);
                        },
                  child: const Text('Cancel'),
                ),
                FilledButton(
                  onPressed: _saving ? null : _save,
                  child: _saving
                      ? const SizedBox(
                          width: 16,
                          height: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Text('Save'),
                ),
              ] else
                TextButton.icon(
                  onPressed:
                      canEdit ? () => setState(() => _editing = true) : null,
                  icon: const Icon(Icons.edit_outlined),
                  label: const Text('Edit'),
                ),
            ],
          ),
        ),
        if (widget.preview.truncated)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Text(
              'Showing first ${widget.preview.maxBytes} bytes of ${widget.preview.size}. Editing is disabled for truncated previews.',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
        Expanded(
          child: _editing
              ? Padding(
                  padding: const EdgeInsets.all(16),
                  child: TextField(
                    controller: _controller,
                    expands: true,
                    maxLines: null,
                    minLines: null,
                    textAlignVertical: TextAlignVertical.top,
                    style:
                        const TextStyle(fontFamily: 'monospace', fontSize: 13),
                    decoration: const InputDecoration(
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                  ),
                )
              : SingleChildScrollView(
                  controller: widget.scrollController,
                  padding: const EdgeInsets.all(16),
                  child: Text(
                    widget.preview.content,
                    style:
                        const TextStyle(fontFamily: 'monospace', fontSize: 13),
                  ),
                ),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }
}

class _FolderPickerScreen extends StatefulWidget {
  final ApiClient client;
  final String initialPath;
  const _FolderPickerScreen({required this.client, required this.initialPath});

  @override
  State<_FolderPickerScreen> createState() => _FolderPickerScreenState();
}

class _FolderPickerScreenState extends State<_FolderPickerScreen> {
  String _path = '/';
  List<FileEntry> _dirs = [];
  bool _loading = true;
  final List<String> _stack = [];

  @override
  void initState() {
    super.initState();
    _path = widget.initialPath;
    _load(_path);
  }

  Future<void> _load(String path) async {
    setState(() => _loading = true);
    try {
      final page = await widget.client.listFiles(path, limit: 500);
      if (mounted) {
        setState(() {
          _path = page.path;
          _dirs = page.entries.where((e) => e.isDir).toList();
          _loading = false;
        });
      }
    } catch (_) {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text('Move to: $_path', style: const TextStyle(fontSize: 13)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, _path),
            child: const Text('Select here'),
          ),
        ],
      ),
      body: Column(
        children: [
          if (_stack.isNotEmpty)
            ListTile(
              leading: const Icon(Icons.arrow_upward),
              title: const Text('..'),
              onTap: () {
                final prev = _stack.removeLast();
                _load(prev);
              },
            ),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : _dirs.isEmpty
                    ? const Center(child: Text('No subfolders'))
                    : ListView.builder(
                        itemCount: _dirs.length,
                        itemBuilder: (_, i) => ListTile(
                          leading: const Icon(Icons.folder_outlined),
                          title: Text(_dirs[i].name),
                          trailing: const Icon(Icons.chevron_right),
                          onTap: () {
                            _stack.add(_path);
                            _load(_dirs[i].path);
                          },
                        ),
                      ),
          ),
        ],
      ),
    );
  }
}

class _GridCell extends StatelessWidget {
  final FileEntry entry;
  final ApiClient client;
  final VoidCallback onTap;
  final VoidCallback onLongPress;
  final bool isSelected;

  const _GridCell({
    required this.entry,
    required this.client,
    required this.onTap,
    required this.onLongPress,
    this.isSelected = false,
  });

  @override
  Widget build(BuildContext context) {
    final hasThumbnail = _supportsThumbnail(entry);
    return GestureDetector(
      onTap: onTap,
      onLongPress: onLongPress,
      child: Stack(
        fit: StackFit.expand,
        children: [
          if (entry.isDir)
            Container(
              color: const Color(0xFFFFF8E1),
              child: const Icon(Icons.folder_rounded,
                  color: Color(0xFFFFB300), size: 56),
            )
          else if (hasThumbnail)
            CachedNetworkImage(
              imageUrl: client.thumbnailUrl(entry.path, 240),
              httpHeaders: client.authHeader,
              fit: BoxFit.cover,
              placeholder: (_, __) => const ColoredBox(
                color: Color(0xFF1A2230),
                child: Center(
                    child: SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: Colors.white24))),
              ),
              errorWidget: (_, __, ___) => _fallbackIcon(),
            )
          else
            _fallbackIcon(),
          // Selection overlay
          if (isSelected)
            Container(
                color: Theme.of(context)
                    .colorScheme
                    .primary
                    .withValues(alpha: 0.35)),
          // Name overlay at bottom
          Positioned(
            left: 0,
            right: 0,
            bottom: 0,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 3),
              decoration: const BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.bottomCenter,
                  end: Alignment.topCenter,
                  colors: [Color(0xCC000000), Colors.transparent],
                ),
              ),
              child: Text(
                entry.name,
                style: const TextStyle(
                    color: Colors.white, fontSize: 10, height: 1.2),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ),
          // Video play icon overlay
          if (entry.previewKind == 'video' && !isSelected)
            const Center(
              child: Icon(Icons.play_circle_outline,
                  color: Colors.white70, size: 32),
            ),
          // Selection checkmark
          if (isSelected)
            Positioned(
              top: 6,
              right: 6,
              child: Container(
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.primary,
                  shape: BoxShape.circle,
                ),
                padding: const EdgeInsets.all(2),
                child: const Icon(Icons.check, color: Colors.white, size: 16),
              ),
            ),
        ],
      ),
    );
  }

  Widget _fallbackIcon() {
    final (icon, color) = switch (entry.previewKind) {
      'image' || 'raw' => (Icons.image_outlined, const Color(0xFF0B6F68)),
      'video' => (Icons.videocam_outlined, const Color(0xFF6B4BD8)),
      'pdf' => (Icons.picture_as_pdf_outlined, const Color(0xFFB73232)),
      'office' => (Icons.description_outlined, const Color(0xFF2563EB)),
      '3d' => (Icons.view_in_ar_outlined, const Color(0xFF16845B)),
      'text' || 'markdown' => (
          Icons.text_snippet_outlined,
          const Color(0xFF50606B)
        ),
      _ => (Icons.insert_drive_file_outlined, const Color(0xFF50606B)),
    };
    return ColoredBox(
      color: const Color(0xFFF0F4F5),
      child: Center(child: Icon(icon, size: 40, color: color)),
    );
  }
}

class _ExifRow extends StatelessWidget {
  final String label;
  final String value;
  const _ExifRow({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
          ),
          const SizedBox(height: 2),
          SelectableText(value),
        ],
      ),
    );
  }
}

class _FolderPickerTile extends StatelessWidget {
  final String path;
  final bool isSelected;
  final VoidCallback onTap;

  const _FolderPickerTile({
    required this.path,
    required this.isSelected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final parts = path == '/'
        ? <String>[]
        : path.split('/').where((s) => s.isNotEmpty).toList();
    final depth = parts.isNotEmpty ? parts.length - 1 : 0;
    final name = parts.isEmpty ? 'My files' : parts.last;
    return ListTile(
      dense: true,
      contentPadding: EdgeInsets.only(left: 16.0 + depth * 12, right: 16),
      leading: Icon(
        isSelected ? Icons.folder : Icons.folder_outlined,
        color: isSelected ? Theme.of(context).colorScheme.primary : null,
        size: 20,
      ),
      title: Text(
        name,
        style: TextStyle(
          fontWeight: isSelected ? FontWeight.w600 : null,
          color: isSelected ? Theme.of(context).colorScheme.primary : null,
        ),
      ),
      subtitle:
          depth > 0 ? Text(path, style: const TextStyle(fontSize: 11)) : null,
      selected: isSelected,
      onTap: onTap,
    );
  }
}

class _EmptyFolderState extends StatelessWidget {
  const _EmptyFolderState();

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Icon(Icons.folder_open_outlined,
            size: 64, color: Theme.of(context).colorScheme.outline),
        const SizedBox(height: 16),
        Text('This folder is empty',
            style: Theme.of(context).textTheme.titleMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant)),
        const SizedBox(height: 8),
        Text('Upload files or create a folder to get started.',
            style: Theme.of(context)
                .textTheme
                .bodySmall
                ?.copyWith(color: Theme.of(context).colorScheme.outline),
            textAlign: TextAlign.center),
      ],
    );
  }
}
