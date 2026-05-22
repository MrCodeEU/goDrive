import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../api/client.dart';
import '../api/models.dart';

class AdminScreen extends StatefulWidget {
  final ApiClient client;
  const AdminScreen({super.key, required this.client});

  @override
  State<AdminScreen> createState() => _AdminScreenState();
}

class _AdminScreenState extends State<AdminScreen>
    with SingleTickerProviderStateMixin {
  late TabController _tabs;
  AdminStats? _stats;
  AdminJob? _job;
  List<User> _users = [];
  List<APIKey> _apiKeys = [];
  List<Webhook> _webhooks = [];
  bool _loading = true;
  String? _error;
  Timer? _pollTimer;
  String? _newAPIKeyToken;
  String? _newWebhookSecret;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 4, vsync: this);
    _refresh();
  }

  @override
  void dispose() {
    _tabs.dispose();
    _pollTimer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        widget.client.adminStats(),
        widget.client.listAdminUsers(),
        widget.client.listAPIKeys(),
        widget.client.listWebhooks(),
      ]);
      if (mounted) {
        setState(() {
          _stats = results[0] as AdminStats;
          _users = results[1] as List<User>;
          _apiKeys = results[2] as List<APIKey>;
          _webhooks = results[3] as List<Webhook>;
          _job = (_stats as AdminStats).currentJob;
          _loading = false;
        });
        if (_job != null && _job!.status == 'running') _startPolling();
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

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (_) async {
      try {
        final job = await widget.client.currentAdminJob();
        if (mounted) {
          setState(() => _job = job);
          if (job == null || job.status != 'running') {
            _pollTimer?.cancel();
            _refresh();
          }
        }
      } catch (_) {}
    });
  }

  Future<void> _runJob(Future<AdminJob> Function() action) async {
    try {
      final job = await action();
      setState(() => _job = job);
      _startPolling();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _showScopedReindex() async {
    final result = await showDialog<_ReindexFormResult>(
      context: context,
      builder: (_) => _ReindexFormDialog(users: _users),
    );
    if (result == null || !mounted) return;
    await _runJob(
      () => widget.client.startReindex(
        username: result.username,
        path: result.path,
      ),
    );
  }

  Future<void> _cancelJob() async {
    try {
      final job = await widget.client.cancelAdminJob();
      setState(() => _job = job);
      _startPolling();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _clearCache() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Clear preview cache'),
        content: const Text(
            'All cached thumbnails will be deleted and regenerated on demand.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Clear')),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await widget.client.clearPreviewCache();
      await _refresh();
      if (mounted) _showSnack('Preview cache cleared');
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  void _showSnack(String msg) =>
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Admin'),
        actions: [
          IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)
        ],
        bottom: TabBar(
          controller: _tabs,
          tabs: const [
            Tab(text: 'System'),
            Tab(text: 'Users'),
            Tab(text: 'Keys'),
            Tab(text: 'Hooks'),
          ],
        ),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? Center(
                  child: Column(mainAxisSize: MainAxisSize.min, children: [
                  Text(_error!,
                      style: TextStyle(
                          color: Theme.of(context).colorScheme.error)),
                  const SizedBox(height: 12),
                  FilledButton.tonal(
                      onPressed: _refresh, child: const Text('Retry')),
                ]))
              : TabBarView(
                  controller: _tabs,
                  children: [
                    _systemTab(),
                    _usersTab(),
                    _apiKeysTab(),
                    _webhooksTab(),
                  ],
                ),
    );
  }

  Widget _systemTab() {
    final stats = _stats!;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Current job
        if (_job != null) ...[
          _section('Active Job'),
          _JobCard(job: _job!, onCancel: _job!.cancelable ? _cancelJob : null),
          const SizedBox(height: 16),
        ],
        // Jobs
        _section('Jobs'),
        Row(children: [
          Expanded(
              child: FilledButton.tonal(
            onPressed: _job?.status == 'running'
                ? null
                : () => _runJob(widget.client.startReindex),
            child: const Text('Full reindex'),
          )),
          const SizedBox(width: 8),
          Expanded(
              child: FilledButton.tonal(
            onPressed: _job?.status == 'running'
                ? null
                : () => _runJob(widget.client.startPreviewWarmup),
            child: const Text('Warmup previews'),
          )),
        ]),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: _job?.status == 'running' ? null : _showScopedReindex,
          icon: const Icon(Icons.manage_search_outlined),
          label: const Text('Scoped reindex'),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: _clearCache,
          icon: const Icon(Icons.delete_outline),
          label: const Text('Clear preview cache'),
        ),
        const SizedBox(height: 20),
        // Index stats
        _section('File Index'),
        _statRow('Files', _fmt(stats.indexedFiles)),
        _statRow('Directories', _fmt(stats.indexedDirs)),
        _statRow('Total size', _formatBytes(stats.indexedBytes)),
        const SizedBox(height: 16),
        // Cache
        _section('Preview Cache'),
        _statRow('Cached files', _fmt(stats.cacheFiles)),
        _statRow('Cache size', _formatBytes(stats.cacheBytes)),
        _statRow('Workers', '${stats.previewWorkers}'),
        if (stats.previewSizes.isNotEmpty)
          _statRow('Sizes', stats.previewSizes.join(', ')),
        if (stats.previewTools.isNotEmpty) ...[
          const SizedBox(height: 8),
          ...stats.previewTools.map((tool) => _PreviewToolRow(tool: tool)),
        ],
        const SizedBox(height: 16),
        // Users / Trash
        _section('Users & Trash'),
        _statRow(
            'Users', '${stats.totalUsers} (${stats.disabledUsers} disabled)'),
        _statRow('Trash items', _fmt(stats.trashItems)),
        _statRow('Trash size', _formatBytes(stats.trashBytes)),
        const SizedBox(height: 16),
        // Watcher
        _section('Watcher & Reconciliation'),
        _statRow(
            'Watcher',
            stats.watcherEnabled
                ? 'Enabled (${stats.watcherRoots} roots)'
                : 'Disabled'),
        _statRow('Reconciliation',
            stats.reconcileEnabled ? stats.reconcileInterval : 'Disabled'),
      ],
    );
  }

  Widget _apiKeysTab() {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _section('Create API Key'),
        FilledButton.icon(
          onPressed: _users.isEmpty ? null : _showCreateAPIKey,
          icon: const Icon(Icons.add_link_outlined),
          label: const Text('New API key'),
        ),
        if (_newAPIKeyToken != null) ...[
          const SizedBox(height: 12),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Copy this token now. It will not be shown again.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 8),
                  SelectableText(
                    _newAPIKeyToken!,
                    style: const TextStyle(fontFamily: 'monospace'),
                  ),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    children: [
                      OutlinedButton.icon(
                        onPressed: () async {
                          await Clipboard.setData(
                            ClipboardData(text: _newAPIKeyToken!),
                          );
                          if (mounted) _showSnack('Token copied');
                        },
                        icon: const Icon(Icons.copy_outlined),
                        label: const Text('Copy'),
                      ),
                      TextButton(
                        onPressed: () => setState(() => _newAPIKeyToken = null),
                        child: const Text('Dismiss'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ],
        const SizedBox(height: 20),
        _section('Existing Keys'),
        if (_apiKeys.isEmpty)
          Text(
            'No API keys yet.',
            style: Theme.of(context).textTheme.bodyMedium,
          )
        else
          ..._apiKeys.map((key) => _APIKeyTile(
                apiKey: key,
                onRevoke: key.revoked ? null : () => _revokeAPIKey(key),
              )),
      ],
    );
  }

  Widget _webhooksTab() {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _section('Create Webhook'),
        FilledButton.icon(
          onPressed: _showCreateWebhook,
          icon: const Icon(Icons.add_link_outlined),
          label: const Text('New webhook'),
        ),
        if (_newWebhookSecret != null) ...[
          const SizedBox(height: 12),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Copy this signing secret now. It will not be shown again.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 8),
                  SelectableText(
                    _newWebhookSecret!,
                    style: const TextStyle(fontFamily: 'monospace'),
                  ),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    children: [
                      OutlinedButton.icon(
                        onPressed: () async {
                          await Clipboard.setData(
                            ClipboardData(text: _newWebhookSecret!),
                          );
                          if (mounted) _showSnack('Secret copied');
                        },
                        icon: const Icon(Icons.copy_outlined),
                        label: const Text('Copy'),
                      ),
                      TextButton(
                        onPressed: () =>
                            setState(() => _newWebhookSecret = null),
                        child: const Text('Dismiss'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ],
        const SizedBox(height: 20),
        _section('Subscriptions'),
        if (_webhooks.isEmpty)
          Text(
            'No webhooks yet.',
            style: Theme.of(context).textTheme.bodyMedium,
          )
        else
          ..._webhooks.map((hook) => _WebhookTile(
                webhook: hook,
                onTest: () => _testWebhook(hook),
                onDelete: () => _deleteWebhook(hook),
              )),
      ],
    );
  }

  Widget _usersTab() {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: FilledButton.icon(
            onPressed: _showCreateUser,
            icon: const Icon(Icons.person_add_outlined),
            label: const Text('New user'),
          ),
        ),
        Expanded(
          child: ListView.separated(
            itemCount: _users.length,
            separatorBuilder: (_, __) => const Divider(height: 1),
            itemBuilder: (_, i) => _UserTile(
              user: _users[i],
              onEdit: () => _showEditUser(_users[i]),
            ),
          ),
        ),
      ],
    );
  }

  Future<void> _showCreateUser() async {
    final result = await showDialog<_UserFormResult>(
      context: context,
      builder: (_) => const _UserFormDialog(title: 'New User'),
    );
    if (result == null || !mounted) return;
    try {
      await widget.client.createAdminUser(
        username: result.username,
        password: result.password!,
        homeRoot: result.homeRoot,
        isAdmin: result.isAdmin,
      );
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _showEditUser(User user) async {
    final result = await showDialog<_UserFormResult>(
      context: context,
      builder: (_) =>
          _UserFormDialog(title: 'Edit ${user.username}', user: user),
    );
    if (result == null || !mounted) return;
    try {
      await widget.client.updateAdminUser(
        user.id,
        username: result.username != user.username ? result.username : null,
        homeRoot: result.homeRoot != user.homeRoot ? result.homeRoot : null,
        isAdmin: result.isAdmin,
        disabled: result.disabled,
      );
      if (result.password != null && result.password!.isNotEmpty) {
        await widget.client.setAdminUserPassword(user.id, result.password!);
      }
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _showCreateAPIKey() async {
    final result = await showDialog<_APIKeyFormResult>(
      context: context,
      builder: (_) => _APIKeyFormDialog(users: _users),
    );
    if (result == null || !mounted) return;
    try {
      final created = await widget.client.createAPIKey(
        userId: result.userId,
        name: result.name,
      );
      setState(() => _newAPIKeyToken = created.$2);
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _revokeAPIKey(APIKey key) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Revoke API key'),
        content: Text('Revoke "${key.name}" for ${key.username}?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Revoke'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await widget.client.revokeAPIKey(key.id);
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _showCreateWebhook() async {
    final result = await showDialog<_WebhookFormResult>(
      context: context,
      builder: (_) => const _WebhookFormDialog(),
    );
    if (result == null || !mounted) return;
    try {
      final created = await widget.client.createWebhook(
        url: result.url,
        events: result.events,
        description: result.description,
      );
      setState(() => _newWebhookSecret = created.$2);
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _testWebhook(Webhook webhook) async {
    try {
      await widget.client.testWebhook(webhook.id);
      if (mounted) _showSnack('Webhook delivered');
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Future<void> _deleteWebhook(Webhook webhook) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Delete webhook'),
        content: Text('Delete webhook for ${webhook.url}?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    try {
      await widget.client.deleteWebhook(webhook.id);
      await _refresh();
    } catch (e) {
      if (mounted) _showSnack('$e');
    }
  }

  Widget _section(String title) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text(title,
            style: Theme.of(context)
                .textTheme
                .titleSmall
                ?.copyWith(color: Theme.of(context).colorScheme.primary)),
      );

  Widget _statRow(String label, String value) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 3),
        child: Row(children: [
          Expanded(
              child:
                  Text(label, style: Theme.of(context).textTheme.bodyMedium)),
          Text(value,
              style: Theme.of(context)
                  .textTheme
                  .bodyMedium
                  ?.copyWith(fontWeight: FontWeight.w600)),
        ]),
      );

  String _fmt(int n) => n >= 1000000
      ? '${(n / 1000000).toStringAsFixed(1)}M'
      : n >= 1000
          ? '${(n / 1000).toStringAsFixed(0)}k'
          : '$n';

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }
}

class _JobCard extends StatelessWidget {
  final AdminJob job;
  final VoidCallback? onCancel;
  const _JobCard({required this.job, this.onCancel});

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final progress =
        job.totalKnown && job.total > 0 ? job.done / job.total : null;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Text(job.type.replaceAll('_', ' ').toUpperCase(),
                style: TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 12,
                    color: cs.primary)),
            const Spacer(),
            _StatusChip(status: job.status),
          ]),
          const SizedBox(height: 6),
          if (job.status == 'running') ...[
            LinearProgressIndicator(value: progress),
            const SizedBox(height: 4),
            Text(
              job.totalKnown
                  ? '${job.done} / ${job.total}'
                  : '${job.done} indexed',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          if (job.message.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(job.message,
                style: Theme.of(context)
                    .textTheme
                    .bodySmall
                    ?.copyWith(color: cs.onSurfaceVariant),
                maxLines: 2,
                overflow: TextOverflow.ellipsis),
          ],
          if (onCancel != null && job.status == 'running') ...[
            const SizedBox(height: 8),
            Align(
              alignment: Alignment.centerRight,
              child: OutlinedButton.icon(
                onPressed: onCancel,
                icon: const Icon(Icons.stop_circle_outlined),
                label: const Text('Cancel job'),
              ),
            ),
          ],
        ]),
      ),
    );
  }
}

class _APIKeyTile extends StatelessWidget {
  final APIKey apiKey;
  final VoidCallback? onRevoke;
  const _APIKeyTile({required this.apiKey, this.onRevoke});

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Card(
      child: ListTile(
        leading: Icon(
          apiKey.revoked ? Icons.link_off_outlined : Icons.key_outlined,
          color: apiKey.revoked ? cs.outline : cs.primary,
        ),
        title: Row(children: [
          Flexible(
            child: Text(apiKey.name, overflow: TextOverflow.ellipsis),
          ),
          if (apiKey.revoked) ...[
            const SizedBox(width: 6),
            const _Badge('revoked', Colors.red),
          ],
        ]),
        subtitle: Text(
          '${apiKey.username} · created ${_shortDate(apiKey.createdAt)}'
          '${apiKey.lastUsedAt != null ? ' · used ${_shortDate(apiKey.lastUsedAt!)}' : ' · never used'}',
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: onRevoke == null
            ? null
            : IconButton(
                icon: const Icon(Icons.delete_outline),
                onPressed: onRevoke,
              ),
      ),
    );
  }

  static String _shortDate(DateTime value) {
    final local = value.toLocal();
    return '${local.year}-${local.month.toString().padLeft(2, '0')}-${local.day.toString().padLeft(2, '0')}';
  }
}

class _PreviewToolRow extends StatelessWidget {
  final PreviewToolStatus tool;
  const _PreviewToolRow({required this.tool});

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            tool.available ? Icons.check_circle_outline : Icons.error_outline,
            size: 18,
            color: tool.available ? Colors.green : cs.error,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  tool.name,
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                ),
                Text(
                  tool.error?.isNotEmpty == true ? tool.error! : tool.purpose,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: cs.onSurfaceVariant,
                      ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _WebhookTile extends StatelessWidget {
  final Webhook webhook;
  final VoidCallback onTest;
  final VoidCallback onDelete;
  const _WebhookTile({
    required this.webhook,
    required this.onTest,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    final events =
        webhook.events.isEmpty ? 'all events' : webhook.events.join(', ');
    return Card(
      child: ListTile(
        leading: const Icon(Icons.webhook_outlined),
        title: Text(
          webhook.description.isNotEmpty ? webhook.description : webhook.url,
          overflow: TextOverflow.ellipsis,
        ),
        subtitle: Text(
          '${webhook.url}\n$events',
          maxLines: 3,
          overflow: TextOverflow.ellipsis,
        ),
        isThreeLine: true,
        trailing: PopupMenuButton<String>(
          onSelected: (value) {
            if (value == 'test') onTest();
            if (value == 'delete') onDelete();
          },
          itemBuilder: (_) => const [
            PopupMenuItem(value: 'test', child: Text('Send test')),
            PopupMenuItem(value: 'delete', child: Text('Delete')),
          ],
        ),
      ),
    );
  }
}

class _StatusChip extends StatelessWidget {
  final String status;
  const _StatusChip({required this.status});

  @override
  Widget build(BuildContext context) {
    final color = switch (status) {
      'running' => Colors.blue,
      'completed' => Colors.green,
      'failed' => Theme.of(context).colorScheme.error,
      _ => Colors.grey,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
          color: color.withAlpha(30), borderRadius: BorderRadius.circular(12)),
      child: Text(status,
          style: TextStyle(
              color: color, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }
}

class _UserTile extends StatelessWidget {
  final User user;
  final VoidCallback onEdit;
  const _UserTile({required this.user, required this.onEdit});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: CircleAvatar(child: Text(user.username[0].toUpperCase())),
      title: Row(children: [
        Text(user.username),
        if (user.isAdmin) ...[
          const SizedBox(width: 6),
          const _Badge('admin', Colors.blue)
        ],
        if (user.disabled) ...[
          const SizedBox(width: 6),
          const _Badge('disabled', Colors.red)
        ],
      ]),
      subtitle: Text(user.homeRoot, overflow: TextOverflow.ellipsis),
      trailing:
          IconButton(icon: const Icon(Icons.edit_outlined), onPressed: onEdit),
    );
  }
}

class _Badge extends StatelessWidget {
  final String label;
  final Color color;
  const _Badge(this.label, this.color);

  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
        decoration: BoxDecoration(
            color: color.withAlpha(25), borderRadius: BorderRadius.circular(4)),
        child: Text(label,
            style: TextStyle(
                color: color, fontSize: 10, fontWeight: FontWeight.w600)),
      );
}

class _UserFormResult {
  final String username;
  final String homeRoot;
  final bool isAdmin;
  final bool disabled;
  final String? password;
  const _UserFormResult(
      {required this.username,
      required this.homeRoot,
      required this.isAdmin,
      required this.disabled,
      this.password});
}

class _UserFormDialog extends StatefulWidget {
  final String title;
  final User? user;
  const _UserFormDialog({required this.title, this.user});

  @override
  State<_UserFormDialog> createState() => _UserFormDialogState();
}

class _UserFormDialogState extends State<_UserFormDialog> {
  late final TextEditingController _username;
  late final TextEditingController _homeRoot;
  late final TextEditingController _password;
  late bool _isAdmin;
  late bool _disabled;

  @override
  void initState() {
    super.initState();
    _username = TextEditingController(text: widget.user?.username ?? '');
    _homeRoot = TextEditingController(text: widget.user?.homeRoot ?? '/data/');
    _password = TextEditingController();
    _isAdmin = widget.user?.isAdmin ?? false;
    _disabled = widget.user?.disabled ?? false;
  }

  @override
  Widget build(BuildContext context) {
    final isNew = widget.user == null;
    return AlertDialog(
      title: Text(widget.title),
      content: SingleChildScrollView(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(
              controller: _username,
              decoration: const InputDecoration(
                  labelText: 'Username', border: OutlineInputBorder()),
              autofocus: isNew),
          const SizedBox(height: 12),
          TextField(
              controller: _homeRoot,
              decoration: const InputDecoration(
                  labelText: 'Home root path', border: OutlineInputBorder())),
          const SizedBox(height: 12),
          TextField(
              controller: _password,
              decoration: InputDecoration(
                  labelText:
                      isNew ? 'Password' : 'New password (leave empty to keep)',
                  border: const OutlineInputBorder()),
              obscureText: true),
          const SizedBox(height: 8),
          SwitchListTile(
              title: const Text('Admin'),
              value: _isAdmin,
              onChanged: (v) => setState(() => _isAdmin = v),
              contentPadding: EdgeInsets.zero),
          if (!isNew)
            SwitchListTile(
                title: const Text('Disabled'),
                value: _disabled,
                onChanged: (v) => setState(() => _disabled = v),
                contentPadding: EdgeInsets.zero),
        ]),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel')),
        FilledButton(
          onPressed: () {
            if (_username.text.trim().isEmpty) return;
            if (isNew && _password.text.isEmpty) return;
            Navigator.pop(
                context,
                _UserFormResult(
                  username: _username.text.trim(),
                  homeRoot: _homeRoot.text.trim(),
                  isAdmin: _isAdmin,
                  disabled: _disabled,
                  password: _password.text.isEmpty ? null : _password.text,
                ));
          },
          child: Text(isNew ? 'Create' : 'Save'),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _username.dispose();
    _homeRoot.dispose();
    _password.dispose();
    super.dispose();
  }
}

class _APIKeyFormResult {
  final int userId;
  final String name;
  const _APIKeyFormResult({required this.userId, required this.name});
}

class _APIKeyFormDialog extends StatefulWidget {
  final List<User> users;
  const _APIKeyFormDialog({required this.users});

  @override
  State<_APIKeyFormDialog> createState() => _APIKeyFormDialogState();
}

class _APIKeyFormDialogState extends State<_APIKeyFormDialog> {
  late int _userId;
  late final TextEditingController _name;

  @override
  void initState() {
    super.initState();
    _userId = widget.users.isNotEmpty ? widget.users.first.id : 0;
    _name = TextEditingController();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('New API key'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          DropdownButtonFormField<int>(
            initialValue: _userId == 0 ? null : _userId,
            decoration: const InputDecoration(
              labelText: 'User',
              border: OutlineInputBorder(),
            ),
            items: [
              for (final user in widget.users)
                DropdownMenuItem(value: user.id, child: Text(user.username)),
            ],
            onChanged: (value) => setState(() => _userId = value ?? 0),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _name,
            decoration: const InputDecoration(
              labelText: 'Key name',
              border: OutlineInputBorder(),
            ),
            autofocus: true,
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () {
            final name = _name.text.trim();
            if (_userId == 0 || name.isEmpty) return;
            Navigator.pop(
              context,
              _APIKeyFormResult(userId: _userId, name: name),
            );
          },
          child: const Text('Create'),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }
}

class _ReindexFormResult {
  final String username;
  final String path;
  const _ReindexFormResult({required this.username, required this.path});
}

class _ReindexFormDialog extends StatefulWidget {
  final List<User> users;
  const _ReindexFormDialog({required this.users});

  @override
  State<_ReindexFormDialog> createState() => _ReindexFormDialogState();
}

class _ReindexFormDialogState extends State<_ReindexFormDialog> {
  late String _username;
  late final TextEditingController _path;

  @override
  void initState() {
    super.initState();
    _username = widget.users.isNotEmpty ? widget.users.first.username : '';
    _path = TextEditingController(text: '/');
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Scoped reindex'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          DropdownButtonFormField<String>(
            initialValue: _username.isEmpty ? null : _username,
            decoration: const InputDecoration(
              labelText: 'User',
              border: OutlineInputBorder(),
            ),
            items: [
              for (final user in widget.users)
                DropdownMenuItem(
                  value: user.username,
                  child: Text(user.username),
                ),
            ],
            onChanged: (value) => setState(() => _username = value ?? ''),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _path,
            decoration: const InputDecoration(
              labelText: 'Path',
              hintText: '/Photos',
              border: OutlineInputBorder(),
            ),
            autofocus: true,
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () {
            final path = _path.text.trim();
            if (_username.isEmpty || path.isEmpty) return;
            Navigator.pop(
              context,
              _ReindexFormResult(username: _username, path: path),
            );
          },
          child: const Text('Start'),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _path.dispose();
    super.dispose();
  }
}

class _WebhookFormResult {
  final String url;
  final List<String> events;
  final String description;
  const _WebhookFormResult({
    required this.url,
    required this.events,
    required this.description,
  });
}

class _WebhookFormDialog extends StatefulWidget {
  const _WebhookFormDialog();

  @override
  State<_WebhookFormDialog> createState() => _WebhookFormDialogState();
}

class _WebhookFormDialogState extends State<_WebhookFormDialog> {
  late final TextEditingController _url;
  late final TextEditingController _description;
  late final TextEditingController _events;

  @override
  void initState() {
    super.initState();
    _url = TextEditingController();
    _description = TextEditingController();
    _events = TextEditingController();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('New webhook'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: _url,
              decoration: const InputDecoration(
                labelText: 'URL',
                hintText: 'https://example.com/godrive',
                border: OutlineInputBorder(),
              ),
              keyboardType: TextInputType.url,
              autofocus: true,
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _description,
              decoration: const InputDecoration(
                labelText: 'Description',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _events,
              decoration: const InputDecoration(
                labelText: 'Events',
                hintText: 'upload.complete, file.moved',
                helperText: 'Leave empty to receive all events.',
                border: OutlineInputBorder(),
              ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () {
            final url = _url.text.trim();
            if (url.isEmpty) return;
            final events = _events.text
                .split(',')
                .map((event) => event.trim())
                .where((event) => event.isNotEmpty)
                .toList();
            Navigator.pop(
              context,
              _WebhookFormResult(
                url: url,
                events: events,
                description: _description.text.trim(),
              ),
            );
          },
          child: const Text('Create'),
        ),
      ],
    );
  }

  @override
  void dispose() {
    _url.dispose();
    _description.dispose();
    _events.dispose();
    super.dispose();
  }
}
