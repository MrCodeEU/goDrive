import 'package:flutter/material.dart';

class BreadcrumbBar extends StatelessWidget {
  final String path;
  final void Function(String path) onNavigate;

  const BreadcrumbBar(
      {super.key, required this.path, required this.onNavigate});

  List<(String label, String path)> get _segments {
    final parts = path.split('/').where((p) => p.isNotEmpty).toList();
    final result = <(String, String)>[('Home', '/')];
    for (var i = 0; i < parts.length; i++) {
      final segPath = '/${parts.sublist(0, i + 1).join('/')}';
      result.add((parts[i], segPath));
    }
    return result;
  }

  @override
  Widget build(BuildContext context) {
    final segs = _segments;
    return SizedBox(
      height: 40,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        itemCount: segs.length,
        separatorBuilder: (_, __) => const Padding(
          padding: EdgeInsets.symmetric(horizontal: 2),
          child: Icon(Icons.chevron_right, size: 18),
        ),
        itemBuilder: (context, i) {
          final (label, segPath) = segs[i];
          final isLast = i == segs.length - 1;
          return Center(
            child: TextButton(
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                foregroundColor: isLast
                    ? Theme.of(context).colorScheme.onSurface
                    : Theme.of(context).colorScheme.primary,
              ),
              onPressed: isLast ? null : () => onNavigate(segPath),
              child: Text(
                label,
                style: TextStyle(
                    fontWeight: isLast ? FontWeight.w600 : FontWeight.normal),
              ),
            ),
          );
        },
      ),
    );
  }
}
