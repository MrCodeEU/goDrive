import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import '../api/models.dart';

class FileTile extends StatelessWidget {
  final FileEntry entry;
  final String thumbnailUrl;
  final Map<String, String> authHeaders;
  final VoidCallback onTap;
  final VoidCallback onLongPress;
  final bool isSelected;
  final bool inSelectionMode;

  const FileTile({
    super.key,
    required this.entry,
    required this.thumbnailUrl,
    required this.authHeaders,
    required this.onTap,
    required this.onLongPress,
    this.isSelected = false,
    this.inSelectionMode = false,
  });

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: inSelectionMode
          ? Checkbox(
              value: isSelected,
              onChanged: (_) => onTap(),
            )
          : _leading(context),
      title: Text(entry.name, overflow: TextOverflow.ellipsis),
      subtitle: Text(
        entry.isDir
            ? 'Folder'
            : '${_formatBytes(entry.size)} · ${_formatDate(entry.modifiedAt)}',
        style: Theme.of(context).textTheme.bodySmall,
      ),
      trailing: inSelectionMode
          ? null
          : (entry.isDir ? const Icon(Icons.chevron_right) : null),
      selected: isSelected,
      onTap: onTap,
      onLongPress: onLongPress,
    );
  }

  Widget _leading(BuildContext context) {
    if (entry.isDir) {
      return const Icon(Icons.folder_rounded,
          size: 40, color: Color(0xFFFFB300));
    }
    if (thumbnailUrl.isNotEmpty) {
      return ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: CachedNetworkImage(
          imageUrl: thumbnailUrl,
          httpHeaders: authHeaders,
          width: 40,
          height: 40,
          fit: BoxFit.cover,
          placeholder: (_, __) => const SizedBox(
              width: 40,
              height: 40,
              child: Center(
                  child: SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2)))),
          errorWidget: (_, __, ___) => _kindIcon(context),
        ),
      );
    }
    return _kindIcon(context);
  }

  Widget _kindIcon(BuildContext context) {
    final (icon, color) = switch (entry.previewKind) {
      'image' => (Icons.image_outlined, const Color(0xFF0B6F68)),
      'raw' => (Icons.image_outlined, const Color(0xFF8A5A2B)),
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
    return Icon(icon, size: 36, color: color);
  }

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }

  String _formatDate(DateTime dt) {
    final now = DateTime.now();
    final diff = now.difference(dt);
    if (diff.inDays == 0) return 'Today';
    if (diff.inDays == 1) return 'Yesterday';
    if (diff.inDays < 30) return '${diff.inDays}d ago';
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')}';
  }
}
