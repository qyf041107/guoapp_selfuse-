import 'dart:convert';

import 'live_models.dart';

enum LiveFormat { m3u, text, json }

class LiveParseResult {
  const LiveParseResult({
    required this.groups,
    this.epg = '',
    this.format = LiveFormat.text,
  });

  final List<LiveGroup> groups;
  final String epg;
  final LiveFormat format;

  int get channelCount =>
      groups.fold(0, (total, group) => total + group.channels.length);
  bool get isEmpty => channelCount == 0;
}

const _metaPrefixes = [
  '更新时间',
  '更新日期',
  'update time',
  'update date',
  'last update',
];

const liveUnsortedGroup = '其他';

class _ChannelBuilder {
  _ChannelBuilder(this.name, this.group);
  final String name;
  final String group;
  final List<String> urls = [];
  Map<String, String> headers = {};
  String logo = '';
  String number = '';
  String tvgId = '';
}

class _GroupBuilder {
  _GroupBuilder(this.name);
  final String name;
  final Map<String, _ChannelBuilder> channels = {};

  _ChannelBuilder channel(String name) =>
      channels.putIfAbsent(name, () => _ChannelBuilder(name, this.name));

  bool get isEmpty => channels.isEmpty;
}

class LiveParser {
  LiveParser._();

  static final _directive = RegExp(
    r'^(ua|parse|click|header|format|origin|referer|forceKey|#EXTHTTP:|#EXTVLCOPT:|#KODIPROP:)',
  );
  static final _genre = RegExp(r'#genre#');
  static final _m3uHeader = RegExp(r'#EXTM3U', caseSensitive: false);

  static LiveParseResult parse(
    String text, {
    required String source,
    String defaultGroup = liveUnsortedGroup,
  }) {
    if (text.trim().isEmpty) {
      return const LiveParseResult(groups: []);
    }
    final normalized = text.replaceAll('\r\n', '\n').replaceAll('\r', '\n');
    if (normalized.trimLeft().startsWith('[') ||
        normalized.trimLeft().startsWith('{')) {
      final json = _parseJson(normalized, source);
      if (json != null && !json.isEmpty) return json;
    }
    if (!_genre.hasMatch(normalized) && _m3uHeader.hasMatch(normalized)) {
      return _parseM3u(normalized, source, defaultGroup);
    }
    return _parseText(normalized, source, defaultGroup);
  }

  static bool _isMetaChannel(String name) {
    final text = name.trim().toLowerCase();
    if (text.isEmpty) return false;
    return _metaPrefixes.any(text.startsWith);
  }

  static String? _attribute(String line, String key) =>
      RegExp('$key="(.*?)"').firstMatch(line)?.group(1)?.trim();

  static (String, String?) _splitFirst(String value, String separator) {
    final at = value.indexOf(separator);
    if (at < 0) return (value, null);
    return (value.substring(0, at), value.substring(at + separator.length));
  }

  static String _afterComma(String line) {
    final at = line.lastIndexOf(',');
    if (at < 0) return '';
    return line.substring(at + 1).trim();
  }

  static LiveParseResult _parseText(
    String text,
    String source,
    String defaultGroup,
  ) {
    final groups = <_GroupBuilder>[];
    final setters = _Setters();
    for (final raw in text.split('\n')) {
      if (raw.isEmpty) continue;
      if (_directive.hasMatch(raw)) {
        setters.read(raw);
        continue;
      }
      final parts = _splitFirst(raw, ',');
      if (_genre.hasMatch(raw)) {
        setters.clear();
        final name = parts.$1.trim();
        groups.add(
          _GroupBuilder(
            name.isEmpty || name == '#genre#' ? defaultGroup : name,
          ),
        );
        continue;
      }
      final tail = parts.$2;
      if (tail == null) continue;
      final name = parts.$1.trim();
      if (name.isEmpty || _isMetaChannel(name)) continue;
      for (final entry in tail.split('#')) {
        final pair = _splitFirst(entry, '|');
        final url = pair.$1.trim();
        if (!isPlayableLiveUrl(url)) continue;
        if (pair.$2 != null && pair.$2!.trim().isNotEmpty) {
          setters.readHeader(pair.$2!);
        }
        final group = groups.isEmpty
            ? (groups..add(_GroupBuilder(defaultGroup))).last
            : groups.last;
        final channel = group.channel(name);
        channel.urls.add(url);
        channel.headers = setters.headers();
      }
    }
    return LiveParseResult(groups: _finalize(groups, source));
  }

  static LiveParseResult _parseM3u(
    String text,
    String source,
    String defaultGroup,
  ) {
    final groups = <_GroupBuilder>[];
    final setters = _Setters();
    var epg = '';
    _GroupBuilder? group;
    _ChannelBuilder? pending;

    for (final raw in text.split('\n')) {
      if (raw.isEmpty) continue;
      if (_directive.hasMatch(raw)) {
        setters.read(raw);
        continue;
      }
      if (raw.startsWith('#EXTM3U')) {
        epg =
            _attribute(raw, 'url-tvg') ??
            _attribute(raw, 'tvg-url') ??
            _attribute(raw, 'x-tvg-url') ??
            '';
        continue;
      }
      if (raw.startsWith('#EXTINF:')) {
        final name = _afterComma(raw);
        if (name.isEmpty || _isMetaChannel(name)) {
          pending = null;
          continue;
        }
        final title = _attribute(raw, 'group-title');
        final target = title == null || title.isEmpty ? defaultGroup : title;
        group = groups.where((entry) => entry.name == target).firstOrNull;
        if (group == null) {
          group = _GroupBuilder(target);
          groups.add(group);
        }
        pending = group.channel(name);
        pending.logo = _attribute(raw, 'tvg-logo') ?? pending.logo;
        pending.tvgId = _attribute(raw, 'tvg-id') ?? pending.tvgId;
        pending.number = _attribute(raw, 'tvg-chno') ?? pending.number;
        final userAgent = _attribute(raw, 'http-user-agent');
        if (userAgent != null && userAgent.isNotEmpty) {
          setters.readHeader('User-Agent=$userAgent');
        }
        continue;
      }
      if (pending == null || raw.startsWith('#')) continue;
      final pair = _splitFirst(raw, '|');
      final url = pair.$1.trim();
      if (!isPlayableLiveUrl(url)) continue;
      if (pair.$2 != null && pair.$2!.trim().isNotEmpty) {
        setters.readHeader(pair.$2!);
      }
      pending.urls.add(url);
      pending.headers = setters.headers();
    }
    return LiveParseResult(
      groups: _finalize(groups, source),
      epg: epg,
      format: LiveFormat.m3u,
    );
  }

  static LiveParseResult? _parseJson(String text, String source) {
    final dynamic decoded;
    try {
      decoded = jsonDecode(text);
    } catch (_) {
      return null;
    }
    final rows = <dynamic>[];
    if (decoded is List) {
      rows.addAll(decoded);
    } else if (decoded is Map) {
      final nested = decoded['groups'] ?? decoded['list'] ?? decoded['data'];
      if (nested is List) {
        rows.addAll(nested);
      } else {
        rows.add(decoded);
      }
    }
    final groups = <_GroupBuilder>[];
    for (final row in rows) {
      if (row is! Map) continue;
      final raw =
          row['channel'] ?? row['channels'] ?? row['list'] ?? row['items'];
      if (raw is! List) continue;
      if (groups.any(
        (entry) => entry.name == _pick(row, const ['group', 'name']),
      )) {
        continue;
      }
      final group = _GroupBuilder(_pick(row, const ['group', 'name', 'title']));
      for (final child in raw) {
        if (child is! Map) continue;
        final name = _pick(child, const ['name', 'tvg-name', 'title']);
        if (name.isEmpty || _isMetaChannel(name)) continue;
        final urls = <String>[];
        final value = child['urls'] ?? child['url'];
        if (value is String) {
          urls.addAll(value.split('#'));
        } else if (value is List) {
          urls.addAll(value.whereType<String>());
        }
        final playable = urls
            .map((entry) => entry.trim())
            .where(isPlayableLiveUrl)
            .toList();
        if (playable.isEmpty) continue;
        final channel = group.channel(name);
        final known = channel.urls.length;
        for (final url in playable) {
          if (!channel.urls.contains(url)) channel.urls.add(url);
        }
        if (channel.urls.length == known) continue;
        final headers = child['header'] ?? child['headers'];
        if (headers is Map) {
          channel.headers = {
            for (final entry in headers.entries)
              entry.key.toString(): entry.value.toString(),
          };
        } else if (headers is String) {
          channel.headers = parseLiveHeaders(headers);
        }
        channel.logo = _pick(child, const ['logo', 'tvg-logo']);
        channel.tvgId = _pick(child, const ['tvg-id', 'tvgId']);
        channel.number = _pick(child, const ['number', 'tvg-chno']);
      }
      if (!group.isEmpty) groups.add(group);
    }
    if (groups.isEmpty) return null;
    return LiveParseResult(
      groups: _finalize(groups, source),
      format: LiveFormat.json,
    );
  }

  static String _pick(Map row, List<String> keys) {
    for (final key in keys) {
      final value = row[key];
      if (value is String && value.trim().isNotEmpty) return value.trim();
      if (value is num) return '$value';
    }
    return '';
  }

  static List<LiveGroup> _finalize(List<_GroupBuilder> groups, String source) {
    var number = 0;
    final result = <LiveGroup>[];
    for (final group in groups) {
      final channels = <LiveChannel>[];
      for (final channel in group.channels.values) {
        if (channel.urls.isEmpty) continue;
        number++;
        channels.add(
          LiveChannel(
            key: LiveChannel.makeKey(source, group.name, channel.name),
            name: channel.name,
            urls: channel.urls,
            number: channel.number.isEmpty
                ? number.toString().padLeft(3, '0')
                : channel.number.padLeft(3, '0'),
            headers: channel.headers,
            logo: channel.logo,
            group: group.name,
            source: source,
            tvgId: channel.tvgId,
          ),
        );
      }
      if (channels.isNotEmpty) result.add(LiveGroup(group.name, channels));
    }
    return result;
  }
}

class _Setters {
  final Map<String, String> _headers = {};
  String _userAgent = '';
  String _referer = '';
  String _origin = '';

  static String _value(String line) {
    final at = line.indexOf('=');
    if (at < 0) return '';
    return line.substring(at + 1).trim();
  }

  void read(String line) {
    if (line.startsWith('#EXTHTTP:')) {
      _headers.addAll(parseLiveHeaders(line.substring(9)));
      return;
    }
    if (line.startsWith('#KODIPROP:inputstream.adaptive.stream_headers') ||
        line.startsWith('#KODIPROP:inputstream.adaptive.common_headers')) {
      readHeader(_value(line));
      return;
    }
    if (line.startsWith('#EXTVLCOPT:http-cookie')) {
      _headers['Cookie'] = _value(line);
      return;
    }
    if (line.startsWith('#EXTVLCOPT:http-origin')) {
      _origin = _value(line);
      return;
    }
    if (line.startsWith('#EXTVLCOPT:http-user-agent')) {
      _userAgent = _value(line);
      return;
    }
    if (line.startsWith('#EXTVLCOPT:http-referrer')) {
      _referer = _value(line);
      return;
    }
    if (line.startsWith('ua')) {
      _userAgent = _value(line);
      return;
    }
    if (line.startsWith('referer')) {
      _referer = _value(line);
      return;
    }
    if (line.startsWith('origin')) {
      _origin = _value(line);
      return;
    }
    if (line.startsWith('header')) readHeader(_value(line));
  }

  void readHeader(String raw) {
    if (raw.trim().isEmpty) return;
    _headers.addAll(parseLiveHeaders(raw));
  }

  Map<String, String> headers() {
    final result = <String, String>{};
    if (_userAgent.isNotEmpty) result['User-Agent'] = _userAgent;
    if (_referer.isNotEmpty) result['Referer'] = _referer;
    if (_origin.isNotEmpty) result['Origin'] = _origin;
    result.addAll(_headers);
    return result;
  }

  void clear() {
    _headers.clear();
    _userAgent = '';
    _referer = '';
    _origin = '';
  }
}
