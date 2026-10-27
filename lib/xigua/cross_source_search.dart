import 'package:flutter/material.dart';

import 'package:duanju_app/app_notice.dart';
import 'package:duanju_app/core_bridge.dart';
import 'package:duanju_app/detail_screen.dart';
import 'package:duanju_app/local_store.dart';
import 'package:duanju_app/models.dart';

String crossSourceQuery(String title) =>
    title.trim().replaceAll(RegExp(r'\s+'), '');

bool crossSourceTitleMatches(String left, String right) {
  final a = crossSourceQuery(left);
  final b = crossSourceQuery(right);
  if (a.isEmpty || b.isEmpty) return false;
  return a == b || a.contains(b) || b.contains(a);
}

class _SourceSearch {
  _SourceSearch(this.site);
  final SourceSite site;
  final results = <Drama>[];
  Object? error;
  bool running = false;
  bool started = false;
}

class CrossSourceSearchSheet extends StatefulWidget {
  const CrossSourceSearchSheet({
    super.key,
    required this.drama,
    required this.query,
    required this.sources,
    required this.repository,
    required this.store,
  });

  final Drama drama;
  final String query;
  final List<SourceSite> sources;
  final AppRepository repository;
  final LocalStore store;

  @override
  State<CrossSourceSearchSheet> createState() => _CrossSourceSearchSheetState();
}

class _CrossSourceSearchSheetState extends State<CrossSourceSearchSheet> {
  final _searches = <_SourceSearch>[];

  @override
  void initState() {
    super.initState();
    for (final site in widget.sources) {
      _searches.add(_SourceSearch(site));
    }
    for (var index = 0; index < 2; index++) {
      _pump();
    }
  }

  void _pump() {
    if (!mounted) return;
    final pending = _searches.indexWhere((search) => !search.started);
    if (pending < 0) return;
    final search = _searches[pending];
    search.started = true;
    search.running = true;
    _run(search).whenComplete(_pump);
  }

  Future<void> _run(_SourceSearch search) async {
    try {
      final page = await widget.repository.catalog(
        search.site.id,
        query: widget.query,
      );
      if (!mounted) return;
      search.results
        ..clear()
        ..addAll(
          page.items
              .where(
                (item) =>
                    item.id != widget.drama.id &&
                    crossSourceTitleMatches(item.title, widget.drama.title),
              )
              .take(2),
        );
      search.error = null;
    } catch (error) {
      if (!mounted) return;
      search.results.clear();
      search.error = error;
    } finally {
      if (mounted) {
        setState(() => search.running = false);
      }
    }
  }

  void _retry(_SourceSearch search) {
    if (search.running) return;
    setState(() {
      search.started = false;
      search.error = null;
    });
    _pump();
  }

  void _open(Drama drama) {
    Navigator.pop(context);
    Navigator.push(
      context,
      MaterialPageRoute<void>(
        builder: (_) => DetailScreen(
          drama: drama,
          repository: widget.repository,
          store: widget.store,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Dialog(
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxWidth: 560,
          maxHeight: MediaQuery.sizeOf(context).height * .8,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 8, 0),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      '其他站源 · ${widget.query}',
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                  ),
                  IconButton(
                    tooltip: '关闭',
                    onPressed: () => Navigator.pop(context),
                    icon: const Icon(Icons.close_rounded),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Text(
                '按剧名搜索其他站源；同名剧可能存在不同季或合集，请确认分集后再播放。',
                style: TextStyle(fontSize: 12, color: colors.onSurfaceVariant),
              ),
            ),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                padding: const EdgeInsets.fromLTRB(8, 0, 8, 12),
                children: [
                  for (final search in _searches) _sourceTile(search, colors),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _sourceTile(_SourceSearch search, ColorScheme colors) {
    if (search.running) {
      return ListTile(
        dense: true,
        leading: const SizedBox(
          width: 18,
          height: 18,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
        title: Text('正在搜索 ${search.site.name}'),
      );
    }
    if (search.error != null) {
      return ListTile(
        dense: true,
        leading: Icon(Icons.error_outline_rounded, color: colors.error),
        title: Text('${search.site.name} 搜索失败'),
        subtitle: const Text('点击重试', style: TextStyle(fontSize: 12)),
        onTap: () => _retry(search),
      );
    }
    if (search.results.isEmpty) {
      return ListTile(
        dense: true,
        enabled: false,
        leading: Icon(Icons.search_off_rounded, color: colors.outline),
        title: Text('${search.site.name} 未找到同名剧集'),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final drama in search.results)
          ListTile(
            dense: true,
            leading: const Icon(Icons.movie_outlined),
            title: Text(
              drama.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            subtitle: Text(
              '${search.site.name}'
              '${drama.episodes > 0 ? ' · ${drama.episodes} 集' : ''}'
              '${drama.category.isEmpty ? '' : ' · ${drama.category}'}',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            onTap: () => _open(drama),
          ),
      ],
    );
  }
}

Future<void> showCrossSourceSearch(
  BuildContext context, {
  required Drama drama,
  required AppRepository repository,
  required LocalStore store,
}) async {
  final query = crossSourceQuery(drama.title);
  if (query.isEmpty) return;
  final sources = store.sources
      .where((site) => site.id != drama.source && site.onlineSearch)
      .toList();
  if (sources.isEmpty) {
    AppNotice.show(context, '当前没有其他支持在线搜索的站源');
    return;
  }
  await showDialog<void>(
    context: context,
    builder: (context) => CrossSourceSearchSheet(
      drama: drama,
      query: query,
      sources: sources,
      repository: repository,
      store: store,
    ),
  );
}
