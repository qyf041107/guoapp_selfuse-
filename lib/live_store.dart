import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'live_models.dart';

class LiveStore extends ChangeNotifier {
  LiveStore(this.preferences, this.profileId);

  final SharedPreferences preferences;
  final String profileId;
  final List<LiveChannel> _favourites = [];
  final List<LiveChannel> _recent = [];
  bool _ready = false;

  bool get ready => _ready;
  List<LiveChannel> get favourites => List.unmodifiable(_favourites);
  List<LiveChannel> get recent => List.unmodifiable(_recent);
  bool isFavourite(String key) => _favourites.any((entry) => entry.key == key);

  String _scope(String key) =>
      profileId == 'default' ? key : 'profile.$profileId.$key';

  void load() {
    _favourites
      ..clear()
      ..addAll(_read(_scope('live.favourites')));
    _recent
      ..clear()
      ..addAll(_read(_scope('live.recent')));
    _ready = true;
    notifyListeners();
  }

  Future<void> toggleFavourite(LiveChannel channel) async {
    if (isFavourite(channel.key)) {
      _favourites.removeWhere((entry) => entry.key == channel.key);
    } else {
      _favourites.insert(0, channel);
    }
    notifyListeners();
    await _write(_scope('live.favourites'), _favourites);
  }

  Future<void> remember(LiveChannel channel) async {
    _recent.removeWhere((entry) => entry.key == channel.key);
    _recent.insert(0, channel);
    if (_recent.length > 60) _recent.removeRange(60, _recent.length);
    notifyListeners();
    await _write(_scope('live.recent'), _recent);
  }

  Future<void> clearRecent() async {
    _recent.clear();
    notifyListeners();
    await _write(_scope('live.recent'), _recent);
  }

  List<LiveChannel> _read(String key) {
    final raw = preferences.getString(key);
    if (raw == null || raw.isEmpty) return const [];
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! List) return const [];
      return decoded
          .whereType<Map>()
          .map(
            (row) => LiveChannel.fromFavourite(Map<String, dynamic>.from(row)),
          )
          .where((entry) => entry.key.isNotEmpty && entry.name.isNotEmpty)
          .toList();
    } catch (_) {
      return const [];
    }
  }

  Future<void> _write(String key, List<LiveChannel> values) async {
    try {
      await preferences.setString(
        key,
        jsonEncode(values.map((entry) => entry.toFavourite()).toList()),
      );
    } catch (_) {}
  }
}
