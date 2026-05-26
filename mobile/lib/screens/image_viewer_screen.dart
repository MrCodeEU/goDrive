import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:photo_view/photo_view.dart';
import 'package:photo_view/photo_view_gallery.dart';
import '../api/client.dart';
import '../api/models.dart';

class ImageViewerScreen extends StatefulWidget {
  final List<FileEntry> entries;
  final int initialIndex;
  final ApiClient client;

  const ImageViewerScreen({
    super.key,
    required this.entries,
    required this.initialIndex,
    required this.client,
  });

  @override
  State<ImageViewerScreen> createState() => _ImageViewerScreenState();
}

class _ImageViewerScreenState extends State<ImageViewerScreen> {
  late int _current;
  late PageController _ctrl;
  bool _showHud = true;
  bool _original = false;
  bool _showInfo = false;

  @override
  void initState() {
    super.initState();
    _current = widget.initialIndex;
    _ctrl = PageController(initialPage: _current);
  }

  FileEntry get _entry => widget.entries[_current];

  String _imageUrl(FileEntry entry) => _original
      ? widget.client.rawFileUrl(entry.path)
      : widget.client.thumbnailUrl(entry.path, 2048);

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }

  String _formatDate(DateTime dt) =>
      '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')}  '
      '${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      extendBodyBehindAppBar: true,
      appBar: _showHud
          ? AppBar(
              backgroundColor: Colors.black54,
              foregroundColor: Colors.white,
              title: Text(
                '${_entry.name}  ${_current + 1}/${widget.entries.length}',
                style: const TextStyle(fontSize: 14),
              ),
              actions: [
                IconButton(
                  icon: Icon(_showInfo ? Icons.info : Icons.info_outline,
                      color: Colors.white),
                  tooltip: 'File info',
                  onPressed: () => setState(() => _showInfo = !_showInfo),
                ),
                TextButton(
                  style: TextButton.styleFrom(foregroundColor: Colors.white),
                  onPressed: () => setState(() {
                    _original = !_original;
                    _showInfo = false;
                  }),
                  child: Text(_original ? 'Preview' : 'Original'),
                ),
              ],
            )
          : null,
      body: Stack(
        children: [
          GestureDetector(
            onTap: () => setState(() {
              _showHud = !_showHud;
              if (!_showHud) _showInfo = false;
            }),
            child: PhotoViewGallery.builder(
              pageController: _ctrl,
              itemCount: widget.entries.length,
              onPageChanged: (i) => setState(() {
                _current = i;
                _showInfo = false;
              }),
              builder: (context, index) {
                final entry = widget.entries[index];
                return PhotoViewGalleryPageOptions(
                  imageProvider: CachedNetworkImageProvider(
                    _imageUrl(entry),
                    headers: widget.client.authHeader,
                  ),
                  minScale: PhotoViewComputedScale.contained,
                  maxScale: PhotoViewComputedScale.covered * 4,
                  heroAttributes: PhotoViewHeroAttributes(tag: entry.path),
                );
              },
              loadingBuilder: (_, __) =>
                  const Center(child: CircularProgressIndicator()),
              backgroundDecoration: const BoxDecoration(color: Colors.black),
            ),
          ),
          // Metadata overlay
          if (_showInfo && _showHud)
            Positioned(
              left: 0,
              right: 0,
              bottom: 0,
              child: Container(
                padding: EdgeInsets.fromLTRB(
                    16, 16, 16, MediaQuery.of(context).padding.bottom + 16),
                decoration: const BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.bottomCenter,
                    end: Alignment.topCenter,
                    colors: [Color(0xDD000000), Colors.transparent],
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(_entry.name,
                        style: const TextStyle(
                            color: Colors.white,
                            fontWeight: FontWeight.w600,
                            fontSize: 15)),
                    const SizedBox(height: 6),
                    _infoRow(Icons.storage_outlined, _formatBytes(_entry.size)),
                    _infoRow(Icons.calendar_today_outlined,
                        _formatDate(_entry.modifiedAt.toLocal())),
                    if (_entry.mimeType != null)
                      _infoRow(Icons.code_outlined, _entry.mimeType!),
                    _infoRow(
                        Icons.folder_outlined,
                        _entry.path.contains('/')
                            ? _entry.path
                                .substring(0, _entry.path.lastIndexOf('/'))
                            : '/'),
                  ],
                ),
              ),
            ),
        ],
      ),
      bottomNavigationBar: _showHud && widget.entries.length > 1
          ? Container(
              color: Colors.black54,
              child: SafeArea(
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                  children: [
                    IconButton(
                      icon:
                          const Icon(Icons.arrow_back_ios, color: Colors.white),
                      onPressed: _current > 0
                          ? () => _ctrl.previousPage(
                              duration: const Duration(milliseconds: 200),
                              curve: Curves.easeInOut)
                          : null,
                    ),
                    Text('${_current + 1} / ${widget.entries.length}',
                        style: const TextStyle(color: Colors.white)),
                    IconButton(
                      icon: const Icon(Icons.arrow_forward_ios,
                          color: Colors.white),
                      onPressed: _current < widget.entries.length - 1
                          ? () => _ctrl.nextPage(
                              duration: const Duration(milliseconds: 200),
                              curve: Curves.easeInOut)
                          : null,
                    ),
                  ],
                ),
              ),
            )
          : null,
    );
  }

  Widget _infoRow(IconData icon, String text) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 2),
        child: Row(
          children: [
            Icon(icon, size: 14, color: Colors.white54),
            const SizedBox(width: 6),
            Expanded(
                child: Text(text,
                    style: const TextStyle(color: Colors.white70, fontSize: 13),
                    overflow: TextOverflow.ellipsis)),
          ],
        ),
      );

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }
}
