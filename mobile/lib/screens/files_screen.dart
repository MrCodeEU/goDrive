import 'dart:async';
import 'dart:io';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:provider/provider.dart';
import 'package:url_launcher/url_launcher.dart';
import '../api/client.dart';
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

class FilesScreen extends StatefulWidget {
  final String path;
  const FilesScreen({super.key, required this.path});

  @override
  State<FilesScreen> createState() => _FilesScreenState();
}

class _FilesScreenState extends State<FilesScreen> {
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
  bool _gridView = false;
  StreamSubscription? _sharingSubscription;

  @override
  void initState() {
    super.initState();
    _currentPath = widget.path;
    _load(_currentPath);
    _initSharing();
  }

  ApiClient get _client => context.read<AuthState>().client!;
  TusClient get _tus => TusClient(_client);

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
          builder: (_, scroll) => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.all(16),
                child: Text(entry.name,
                    style: Theme.of(context).textTheme.titleMedium),
              ),
              if (preview.truncated)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Text(
                      'Showing first ${preview.maxBytes} bytes of ${preview.size}',
                      style: Theme.of(context).textTheme.bodySmall),
                ),
              Expanded(
                child: SingleChildScrollView(
                  controller: scroll,
                  padding: const EdgeInsets.all(16),
                  child: Text(preview.content,
                      style: const TextStyle(
                          fontFamily: 'monospace', fontSize: 13)),
                ),
              ),
            ],
          ),
        ),
      );
    } catch (e) {
      if (mounted) _showSnack('Failed to load preview: $e');
    }
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
    try {
      await _client.deleteFile(entry.path);
      await _load(_currentPath);
    } catch (e) {
      if (mounted) _showSnack('Delete failed: $e');
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
    _sharingSubscription = ReceiveSharingIntent.instance.getMediaStream().listen(
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
    final files = shared
        .map((f) => File(f.path))
        .where((f) => f.existsSync())
        .toList();
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

    return showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setState) {
            if (loading) {
              // Load folders async, then rebuild
              _client.listFileTree().then((resp) {
                if (ctx.mounted) {
                  setState(() {
                    folders = resp
                        .where((e) => e.type == 'dir')
                        .toList()
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
                  // Handle
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
                              // Root entry
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
        appBar: AppBar(
          title: _searching
              ? TextField(
                  controller: _searchCtrl,
                  autofocus: true,
                  decoration: const InputDecoration(
                      hintText: 'Search files…', border: InputBorder.none),
                  onSubmitted: _search,
                )
              : const Text('goDrive'),
          actions: [
            if (!_searching) ...[
              IconButton(
                icon: Icon(_gridView ? Icons.view_list : Icons.grid_view),
                tooltip: _gridView ? 'List view' : 'Grid view',
                onPressed: () => setState(() => _gridView = !_gridView),
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
              },
              itemBuilder: (_) => [
                const PopupMenuItem(value: 'trash', child: Text('Trash')),
                if (user?.isAdmin == true)
                  const PopupMenuItem(value: 'admin', child: Text('Admin')),
                PopupMenuItem(
                    value: 'logout',
                    child: Text('Sign out (${user?.username ?? ''})')),
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
        floatingActionButton: Column(
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
    if (_entries.isEmpty) return const Center(child: Text('Empty folder'));
    if (_gridView) return _gridBody();
    return RefreshIndicator(
      onRefresh: () => _load(_currentPath),
      child: ListView.separated(
        itemCount: _entries.length,
        separatorBuilder: (_, __) => const Divider(height: 1, indent: 68),
        itemBuilder: (context, i) {
          final entry = _entries[i];
          return FileTile(
            entry: entry,
            thumbnailUrl: _supportsThumbnail(entry)
                ? _client.thumbnailUrl(entry.path, 96)
                : '',
            authHeaders: _client.authHeader,
            onTap: () => entry.isDir ? _navigate(entry.path) : _openFile(entry),
            onLongPress: () => _showFileActions(entry),
          );
        },
      ),
    );
  }

  Widget _gridBody() {
    return RefreshIndicator(
      onRefresh: () => _load(_currentPath),
      child: GridView.builder(
        padding: const EdgeInsets.all(2),
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 3,
          mainAxisSpacing: 2,
          crossAxisSpacing: 2,
        ),
        itemCount: _entries.length,
        itemBuilder: (context, i) {
          final entry = _entries[i];
          return _GridCell(
            entry: entry,
            client: _client,
            onTap: () => entry.isDir ? _navigate(entry.path) : _openFile(entry),
            onLongPress: () => _showFileActions(entry),
          );
        },
      ),
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
    _sharingSubscription?.cancel();
    _searchCtrl.dispose();
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
                      leading: Icon(item.isDir
                          ? Icons.folder_outlined
                          : Icons.insert_drive_file_outlined),
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

  const _GridCell({
    required this.entry,
    required this.client,
    required this.onTap,
    required this.onLongPress,
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
          if (entry.previewKind == 'video')
            const Center(
              child: Icon(Icons.play_circle_outline,
                  color: Colors.white70, size: 32),
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
    final depth = path == '/'
        ? 0
        : path.split('/').where((s) => s.isNotEmpty).length - 1;
    final name = path == '/'
        ? 'My files'
        : path.split('/').where((s) => s.isNotEmpty).last;
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
