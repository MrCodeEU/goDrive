import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/tus.dart';
import '../state/auth_state.dart';
import '../state/upload_queue.dart';

class UploadQueueSheet extends StatefulWidget {
  const UploadQueueSheet({super.key});

  static void show(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => const UploadQueueSheet(),
    );
  }

  @override
  State<UploadQueueSheet> createState() => _UploadQueueSheetState();
}

class _UploadQueueSheetState extends State<UploadQueueSheet> {
  Timer? _refreshTimer;

  @override
  void initState() {
    super.initState();
    _refreshTimer = Timer.periodic(const Duration(seconds: 2), (_) {
      if (!mounted) return;
      unawaited(context.read<UploadQueue>().refreshPersisted());
    });
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final queue = context.watch<UploadQueue>();
    final client = context.read<AuthState>().client;
    final items = queue.items;

    return DraggableScrollableSheet(
      initialChildSize: 0.5,
      minChildSize: 0.25,
      maxChildSize: 0.9,
      expand: false,
      builder: (context, scroll) => Column(
        children: [
          _handle(),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 4, 8, 8),
            child: Row(
              children: [
                Text('Uploads', style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(width: 8),
                Text('${items.length}',
                    style: Theme.of(context).textTheme.bodySmall),
                const Spacer(),
                TextButton(
                  onPressed: queue.clearCompleted,
                  child: const Text('Clear done'),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          Expanded(
            child: items.isEmpty
                ? const Center(child: Text('No uploads'))
                : ListView.builder(
                    controller: scroll,
                    itemCount: items.length,
                    itemBuilder: (context, i) => _UploadItemTile(
                      item: items[i],
                      canAct: client != null,
                      onRetry: client == null
                          ? null
                          : () => queue.retry(items[i], TusClient(client)),
                      onBackground: client == null
                          ? null
                          : () => queue.startBackgroundUpload(items[i], client),
                      onRemove: () => queue.remove(items[i]),
                    ),
                  ),
          ),
        ],
      ),
    );
  }

  Widget _handle() => Center(
        child: Container(
          margin: const EdgeInsets.only(top: 8, bottom: 4),
          width: 36,
          height: 4,
          decoration: BoxDecoration(
            color: Colors.grey.shade400,
            borderRadius: BorderRadius.circular(2),
          ),
        ),
      );
}

class _UploadItemTile extends StatelessWidget {
  final UploadItem item;
  final bool canAct;
  final VoidCallback? onRetry;
  final VoidCallback? onBackground;
  final VoidCallback onRemove;

  const _UploadItemTile({
    required this.item,
    required this.canAct,
    required this.onRetry,
    required this.onBackground,
    required this.onRemove,
  });

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    Color statusColor = cs.onSurfaceVariant;
    String statusText = '';

    switch (item.status) {
      case UploadStatus.queued:
        statusText = 'Waiting';
      case UploadStatus.uploading:
        statusText = '${(item.progress * 100).round()}%';
        statusColor = cs.primary;
      case UploadStatus.background:
        final percent = (item.progress * 100).round();
        statusText = percent > 0 ? '$percent%' : 'Background';
        statusColor = cs.primary;
      case UploadStatus.done:
        statusText = 'Done';
        statusColor = Colors.green.shade700;
      case UploadStatus.error:
        statusText = 'Failed';
        statusColor = cs.error;
      case UploadStatus.interrupted:
        statusText = 'Interrupted';
        statusColor = Colors.orange.shade700;
    }

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                  child: Text(item.name,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontWeight: FontWeight.w500))),
              Text(statusText,
                  style: TextStyle(color: statusColor, fontSize: 12)),
              if (item.status == UploadStatus.error &&
                  item.file != null &&
                  canAct)
                IconButton(
                  visualDensity: VisualDensity.compact,
                  icon: const Icon(Icons.refresh),
                  tooltip: 'Retry',
                  onPressed: onRetry,
                ),
              if ((item.status == UploadStatus.queued ||
                      item.status == UploadStatus.error ||
                      item.status == UploadStatus.interrupted) &&
                  item.file != null &&
                  canAct &&
                  (Platform.isAndroid || Platform.isIOS))
                IconButton(
                  visualDensity: VisualDensity.compact,
                  icon: const Icon(Icons.cloud_upload_outlined),
                  tooltip: 'Continue in background',
                  onPressed: onBackground,
                ),
              if (item.status == UploadStatus.error ||
                  item.status == UploadStatus.interrupted ||
                  item.status == UploadStatus.background ||
                  item.status == UploadStatus.done)
                IconButton(
                  visualDensity: VisualDensity.compact,
                  icon: const Icon(Icons.close),
                  tooltip: 'Remove',
                  onPressed: onRemove,
                ),
            ],
          ),
          const SizedBox(height: 2),
          Text(item.targetPath,
              style: Theme.of(context).textTheme.bodySmall,
              overflow: TextOverflow.ellipsis),
          if (item.status == UploadStatus.uploading) ...[
            const SizedBox(height: 4),
            LinearProgressIndicator(value: item.progress),
          ] else if (item.status == UploadStatus.background) ...[
            const SizedBox(height: 4),
            LinearProgressIndicator(
              value:
                  item.progress > 0 && item.progress < 1 ? item.progress : null,
            ),
            const SizedBox(height: 4),
            Text(
              Platform.isIOS
                  ? 'iOS background URLSession'
                  : 'Android foreground service',
              style: const TextStyle(fontSize: 12),
            ),
          ] else if (item.status == UploadStatus.error &&
              item.error != null) ...[
            const SizedBox(height: 4),
            Text(item.error!, style: TextStyle(color: cs.error, fontSize: 12)),
          ] else if (item.status == UploadStatus.interrupted) ...[
            const SizedBox(height: 4),
            Text(
              item.file == null
                  ? 'File is no longer available on this device'
                  : 'Upload was interrupted',
              style: const TextStyle(fontSize: 12, color: Colors.orange),
            ),
          ],
        ],
      ),
    );
  }
}
