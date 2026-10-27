import 'package:flutter/material.dart';

import 'package:duanju_app/app_layout.dart';
import 'package:duanju_app/models.dart';

class WatchDayStats {
  const WatchDayStats({this.seconds = 0, this.episodes = 0});

  final double seconds;
  final int episodes;

  WatchDayStats add(double seconds, int episodes) => WatchDayStats(
    seconds: this.seconds + seconds,
    episodes: this.episodes + episodes,
  );

  List<dynamic> toJson() => [seconds, episodes];

  factory WatchDayStats.fromJson(dynamic row) => row is List && row.isNotEmpty
      ? WatchDayStats(
          seconds: (row[0] as num?)?.toDouble() ?? 0,
          episodes: (row[1] as num?)?.toInt() ?? 0,
        )
      : const WatchDayStats();
}

class WatchStats {
  const WatchStats({
    this.days = const {},
    this.sources = const {},
    this.totalSeconds = 0,
    this.totalEpisodes = 0,
  });

  final Map<String, WatchDayStats> days;
  final Map<String, double> sources;
  final double totalSeconds;
  final int totalEpisodes;

  static const maxDays = 400;

  static String _dayKey(DateTime date) =>
      '${date.year.toString().padLeft(4, '0')}-'
      '${date.month.toString().padLeft(2, '0')}-'
      '${date.day.toString().padLeft(2, '0')}';

  static const _maxDelta = 20.0;

  WatchStats addSample({
    required String source,
    required double seconds,
    required int episodes,
    DateTime? now,
  }) {
    if (!seconds.isFinite || seconds < 0 || episodes < 0) {
      throw const FormatException('观看统计无效');
    }
    if (seconds == 0 && episodes == 0) return this;
    final day = _dayKey(now ?? DateTime.now());
    final updatedDays = Map.of(days);
    updatedDays[day] = (updatedDays[day] ?? const WatchDayStats()).add(
      seconds,
      episodes,
    );
    while (updatedDays.length > maxDays) {
      updatedDays.remove(
        updatedDays.keys.reduce((a, b) => a.compareTo(b) < 0 ? a : b),
      );
    }
    final updatedSources = Map.of(sources);
    updatedSources[source] = (updatedSources[source] ?? 0) + seconds;
    return WatchStats(
      days: updatedDays,
      sources: updatedSources,
      totalSeconds: totalSeconds + seconds,
      totalEpisodes: totalEpisodes + episodes,
    );
  }

  WatchStats record({
    required WatchEntry previous,
    required WatchEntry current,
    DateTime? now,
  }) {
    if (current.drama.id != previous.drama.id) return this;
    final at = now ?? DateTime.now();
    final day = _dayKey(at);
    var seconds = 0.0;
    var episodes = 0;
    if (previous.episode == current.episode) {
      final delta = current.position - previous.position;
      if (delta > 0 && delta <= _maxDelta) seconds = delta;
    }
    if (current.finished && !previous.finished) episodes = 1;
    if (seconds <= 0 && episodes == 0) return this;
    final days = Map.of(this.days);
    final dayStats = (days[day] ?? const WatchDayStats()).add(
      seconds,
      episodes,
    );
    days[day] = dayStats;
    while (days.length > maxDays) {
      days.remove(days.keys.reduce((a, b) => a.compareTo(b) < 0 ? a : b));
    }
    final sources = Map.of(this.sources);
    final source = current.drama.source;
    sources[source] = (sources[source] ?? 0) + seconds;
    return WatchStats(
      days: days,
      sources: sources,
      totalSeconds: totalSeconds + seconds,
      totalEpisodes: totalEpisodes + episodes,
    );
  }

  double secondsBetween(DateTime from, DateTime to) {
    var total = 0.0;
    for (
      var date = from;
      !date.isAfter(to);
      date = date.add(const Duration(days: 1))
    ) {
      total += days[_dayKey(date)]?.seconds ?? 0;
    }
    return total;
  }

  double get todaySeconds => days[_dayKey(DateTime.now())]?.seconds ?? 0;

  double get weekSeconds => secondsBetween(
    DateTime.now().subtract(const Duration(days: 6)),
    DateTime.now(),
  );

  Map<String, dynamic> toJson() => {
    'v': 1,
    'days': {for (final entry in days.entries) entry.key: entry.value.toJson()},
    'sources': sources,
    'total': [totalSeconds, totalEpisodes],
  };

  static void validateJson(dynamic value) {
    if (value == null || value is Map && value.isEmpty) return;
    bool validSeconds(dynamic seconds) =>
        seconds is num && seconds.isFinite && seconds >= 0;
    bool validRow(dynamic row) =>
        row is List &&
        row.length == 2 &&
        validSeconds(row[0]) &&
        row[1] is int &&
        row[1] >= 0;
    if (value is! Map ||
        value['v'] != 1 ||
        value['days'] is! Map ||
        value['sources'] is! Map ||
        !validRow(value['total'])) {
      throw const FormatException('观看统计无效');
    }
    final days = value['days'] as Map;
    final sources = value['sources'] as Map;
    if (days.length > maxDays || sources.length > SourceSite.allValues.length) {
      throw const FormatException('观看统计过多');
    }
    for (final entry in days.entries) {
      final date = entry.key is String
          ? DateTime.tryParse(entry.key as String)
          : null;
      if (date == null ||
          _dayKey(date) != entry.key ||
          !validRow(entry.value)) {
        throw const FormatException('观看日期统计无效');
      }
    }
    for (final entry in sources.entries) {
      if (entry.key is! String ||
          !SourceSite.isKnown(entry.key as String) ||
          !validSeconds(entry.value)) {
        throw const FormatException('观看站源统计无效');
      }
    }
  }

  static WatchStats fromJson(dynamic decoded) {
    if (decoded is! Map) return const WatchStats();
    try {
      validateJson(decoded);
      final days = <String, WatchDayStats>{};
      final rawDays = Map<String, dynamic>.from(decoded['days'] as Map? ?? {});
      for (final entry in rawDays.entries) {
        days[entry.key] = WatchDayStats.fromJson(entry.value);
      }
      final sources = <String, double>{};
      final rawSources = Map<String, dynamic>.from(
        decoded['sources'] as Map? ?? {},
      );
      for (final entry in rawSources.entries) {
        final value = (entry.value as num?)?.toDouble();
        if (value != null) {
          sources[entry.key] = value;
        }
      }
      final total = decoded['total'];
      return WatchStats(
        days: days,
        sources: sources,
        totalSeconds: total is List && total.isNotEmpty
            ? (total[0] as num?)?.toDouble() ?? 0
            : 0,
        totalEpisodes: total is List && total.length > 1
            ? (total[1] as num?)?.toInt() ?? 0
            : 0,
      );
    } catch (_) {
      return const WatchStats();
    }
  }
}

String formatWatchDuration(double seconds) {
  final total = (seconds / 60).round();
  if (total <= 0) return '0 分钟';
  if (total < 60) return '$total 分钟';
  final hours = total ~/ 60;
  final minutes = total % 60;
  return minutes == 0 ? '$hours 小时' : '$hours 小时 $minutes 分钟';
}

class WatchStatsPanel extends StatelessWidget {
  const WatchStatsPanel({
    super.key,
    required this.stats,
    required this.watchingCount,
  });

  final WatchStats stats;
  final int watchingCount;

  static Future<void> show(
    BuildContext context, {
    required WatchStats stats,
    required int watchingCount,
  }) async {
    final panel = WatchStatsPanel(stats: stats, watchingCount: watchingCount);
    if (AppLayout.isTelevision(context)) {
      await showDialog<void>(
        context: context,
        builder: (_) => Dialog(child: SizedBox(width: 720, child: panel)),
      );
    } else {
      await showModalBottomSheet<void>(
        context: context,
        showDragHandle: true,
        isScrollControlled: true,
        builder: (_) => panel,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final today = stats.todaySeconds;
    final week = stats.weekSeconds;
    final topSources = stats.sources.entries.toList()
      ..sort((a, b) => b.value.compareTo(a.value));
    final sourceMax = topSources.isEmpty ? 0.0 : topSources.first.value;
    final now = DateTime.now();
    final recent = List.generate(7, (index) {
      final date = now.subtract(Duration(days: 6 - index));
      final key =
          '${date.year.toString().padLeft(4, '0')}-'
          '${date.month.toString().padLeft(2, '0')}-'
          '${date.day.toString().padLeft(2, '0')}';
      return (date, stats.days[key]?.seconds ?? 0);
    });
    final recentMax = recent.fold<double>(
      0,
      (value, item) => value > item.$2 ? value : item.$2,
    );
    return SafeArea(
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(context).height * .78,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Row(
                children: [
                  const Expanded(
                    child: Text(
                      '观看统计',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: '关闭统计',
                    onPressed: () => Navigator.pop(context),
                    icon: const Icon(Icons.close_rounded),
                  ),
                  Text(
                    '仅本机记录',
                    style: TextStyle(
                      fontSize: 12,
                      color: scheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                children: [
                  const Text('从本次功能启用后记录点播的实际播放耗时；暂停、缓冲不计时，倍速按实际耗时计算。直播不计入。'),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      Expanded(
                        child: _MetricTile(
                          label: '累计观看',
                          value: formatWatchDuration(stats.totalSeconds),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: _MetricTile(
                          label: '看完集数',
                          value: '${stats.totalEpisodes} 集',
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: _MetricTile(
                          label: '在看剧目',
                          value: '$watchingCount 部',
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Row(
                    children: [
                      Expanded(
                        child: _MetricTile(
                          label: '最近 7 天',
                          value: formatWatchDuration(week),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: _MetricTile(
                          label: '今天',
                          value: formatWatchDuration(today),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  Text('最近 7 天', style: Theme.of(context).textTheme.titleSmall),
                  const SizedBox(height: 8),
                  SizedBox(
                    height: 84,
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.end,
                      children: [
                        for (final (index, item) in recent.indexed) ...[
                          Expanded(
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Expanded(
                                  child: Container(
                                    width: 12,
                                    alignment: Alignment.bottomCenter,
                                    child: Container(
                                      width: 12,
                                      height: recentMax <= 0
                                          ? 2
                                          : (item.$2 / recentMax * 56).clamp(
                                              2.0,
                                              56.0,
                                            ),
                                      decoration: BoxDecoration(
                                        color: item.$2 > 0
                                            ? scheme.primary
                                            : scheme.outlineVariant,
                                        borderRadius: BorderRadius.circular(4),
                                      ),
                                    ),
                                  ),
                                ),
                                const SizedBox(height: 4),
                                Text(
                                  item.$1.day == now.day
                                      ? '今天'
                                      : '周${'一二三四五六日'[item.$1.weekday - 1]}',
                                  style: TextStyle(
                                    fontSize: 11,
                                    color: scheme.onSurfaceVariant,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          if (index < recent.length - 1)
                            const SizedBox(width: 4),
                        ],
                      ],
                    ),
                  ),
                  if (topSources.isNotEmpty) ...[
                    const SizedBox(height: 16),
                    Text('站源占比', style: Theme.of(context).textTheme.titleSmall),
                    const SizedBox(height: 8),
                    for (final source in topSources.take(6))
                      Padding(
                        padding: const EdgeInsets.only(bottom: 6),
                        child: Row(
                          children: [
                            SizedBox(
                              width: 96,
                              child: Text(
                                SourceSite.byId(source.key).name,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(fontSize: 13),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: LinearProgressIndicator(
                                value: sourceMax <= 0
                                    ? 0
                                    : (source.value / sourceMax).clamp(0, 1),
                                minHeight: 6,
                                borderRadius: BorderRadius.circular(3),
                              ),
                            ),
                            const SizedBox(width: 8),
                            SizedBox(
                              width: 72,
                              child: Text(
                                formatWatchDuration(source.value),
                                textAlign: TextAlign.end,
                                style: TextStyle(
                                  fontSize: 12,
                                  color: scheme.onSurfaceVariant,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MetricTile extends StatelessWidget {
  const _MetricTile({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: scheme.surfaceContainerHighest.withValues(alpha: .5),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            label,
            style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant),
          ),
          const SizedBox(height: 2),
          Text(
            value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
          ),
        ],
      ),
    );
  }
}
