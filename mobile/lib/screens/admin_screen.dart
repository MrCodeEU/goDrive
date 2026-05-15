import 'dart:async';
import 'package:flutter/material.dart';
import '../api/client.dart';
import '../api/models.dart';

class AdminScreen extends StatefulWidget {
  final ApiClient client;
  const AdminScreen({super.key, required this.client});

  @override
  State<AdminScreen> createState() => _AdminScreenState();
}

class _AdminScreenState extends State<AdminScreen> with SingleTickerProviderStateMixin {
  late TabController _tabs;
  AdminStats? _stats;
  AdminJob? _job;
  List<User> _users = [];
  bool _loading = true;
  String? _error;
  Timer? _pollTimer;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 2, vsync: this);
    _refresh();
  }

  @override
  void dispose() {
    _tabs.dispose();
    _pollTimer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    setState(() { _loading = true; _error = null; });
    try {
      final results = await Future.wait([
        widget.client.adminStats(),
        widget.client.listAdminUsers(),
      ]);
      if (mounted) {
        setState(() {
          _stats = results[0] as AdminStats;
          _users = results[1] as List<User>;
          _job = (_stats as AdminStats).currentJob;
          _loading = false;
        });
        if (_job != null && _job!.status == 'running') _startPolling();
      }
    } catch (e) {
      if (mounted) setState(() { _error = e.toString(); _loading = false; });
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

  Future<void> _clearCache() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Clear preview cache'),
        content: const Text('All cached thumbnails will be deleted and regenerated on demand.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Clear')),
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
        actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _refresh)],
        bottom: TabBar(controller: _tabs, tabs: const [Tab(text: 'System'), Tab(text: 'Users')]),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? Center(child: Column(mainAxisSize: MainAxisSize.min, children: [
                  Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                  const SizedBox(height: 12),
                  FilledButton.tonal(onPressed: _refresh, child: const Text('Retry')),
                ]))
              : TabBarView(controller: _tabs, children: [_systemTab(), _usersTab()]),
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
          _JobCard(job: _job!),
          const SizedBox(height: 16),
        ],
        // Jobs
        _section('Jobs'),
        Row(children: [
          Expanded(child: FilledButton.tonal(
            onPressed: _job?.status == 'running' ? null : () => _runJob(widget.client.startReindex),
            child: const Text('Reindex'),
          )),
          const SizedBox(width: 8),
          Expanded(child: FilledButton.tonal(
            onPressed: _job?.status == 'running' ? null : () => _runJob(widget.client.startPreviewWarmup),
            child: const Text('Warmup previews'),
          )),
        ]),
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
        const SizedBox(height: 16),
        // Users / Trash
        _section('Users & Trash'),
        _statRow('Users', '${stats.totalUsers} (${stats.disabledUsers} disabled)'),
        _statRow('Trash items', _fmt(stats.trashItems)),
        _statRow('Trash size', _formatBytes(stats.trashBytes)),
        const SizedBox(height: 16),
        // Watcher
        _section('Watcher & Reconciliation'),
        _statRow('Watcher', stats.watcherEnabled ? 'Enabled (${stats.watcherRoots} roots)' : 'Disabled'),
        _statRow('Reconciliation', stats.reconcileEnabled ? stats.reconcileInterval : 'Disabled'),
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
      builder: (_) => _UserFormDialog(title: 'Edit ${user.username}', user: user),
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

  Widget _section(String title) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text(title, style: Theme.of(context).textTheme.titleSmall?.copyWith(color: Theme.of(context).colorScheme.primary)),
      );

  Widget _statRow(String label, String value) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 3),
        child: Row(children: [
          Expanded(child: Text(label, style: Theme.of(context).textTheme.bodyMedium)),
          Text(value, style: Theme.of(context).textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w600)),
        ]),
      );

  String _fmt(int n) => n >= 1000000 ? '${(n / 1000000).toStringAsFixed(1)}M' : n >= 1000 ? '${(n / 1000).toStringAsFixed(0)}k' : '$n';

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }
}

class _JobCard extends StatelessWidget {
  final AdminJob job;
  const _JobCard({required this.job});

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final progress = job.totalKnown && job.total > 0 ? job.done / job.total : null;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Text(job.type.replaceAll('_', ' ').toUpperCase(), style: TextStyle(fontWeight: FontWeight.w600, fontSize: 12, color: cs.primary)),
            const Spacer(),
            _StatusChip(status: job.status),
          ]),
          const SizedBox(height: 6),
          if (job.status == 'running') ...[
            LinearProgressIndicator(value: progress),
            const SizedBox(height: 4),
            Text(
              job.totalKnown ? '${job.done} / ${job.total}' : '${job.done} indexed',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          if (job.message.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(job.message, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: cs.onSurfaceVariant), maxLines: 2, overflow: TextOverflow.ellipsis),
          ],
        ]),
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
      decoration: BoxDecoration(color: color.withAlpha(30), borderRadius: BorderRadius.circular(12)),
      child: Text(status, style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
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
        if (user.isAdmin) ...[const SizedBox(width: 6), const _Badge('admin', Colors.blue)],
        if (user.disabled) ...[const SizedBox(width: 6), const _Badge('disabled', Colors.red)],
      ]),
      subtitle: Text(user.homeRoot, overflow: TextOverflow.ellipsis),
      trailing: IconButton(icon: const Icon(Icons.edit_outlined), onPressed: onEdit),
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
        decoration: BoxDecoration(color: color.withAlpha(25), borderRadius: BorderRadius.circular(4)),
        child: Text(label, style: TextStyle(color: color, fontSize: 10, fontWeight: FontWeight.w600)),
      );
}

class _UserFormResult {
  final String username;
  final String homeRoot;
  final bool isAdmin;
  final bool disabled;
  final String? password;
  const _UserFormResult({required this.username, required this.homeRoot, required this.isAdmin, required this.disabled, this.password});
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
          TextField(controller: _username, decoration: const InputDecoration(labelText: 'Username', border: OutlineInputBorder()), autofocus: isNew),
          const SizedBox(height: 12),
          TextField(controller: _homeRoot, decoration: const InputDecoration(labelText: 'Home root path', border: OutlineInputBorder())),
          const SizedBox(height: 12),
          TextField(controller: _password, decoration: InputDecoration(labelText: isNew ? 'Password' : 'New password (leave empty to keep)', border: const OutlineInputBorder()), obscureText: true),
          const SizedBox(height: 8),
          SwitchListTile(title: const Text('Admin'), value: _isAdmin, onChanged: (v) => setState(() => _isAdmin = v), contentPadding: EdgeInsets.zero),
          if (!isNew) SwitchListTile(title: const Text('Disabled'), value: _disabled, onChanged: (v) => setState(() => _disabled = v), contentPadding: EdgeInsets.zero),
        ]),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(
          onPressed: () {
            if (_username.text.trim().isEmpty) return;
            if (isNew && _password.text.isEmpty) return;
            Navigator.pop(context, _UserFormResult(
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
