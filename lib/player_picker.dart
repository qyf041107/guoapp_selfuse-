import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'lan_models.dart';
import 'local_store.dart';
import 'models.dart';
import 'player_route.dart';
import 'player_screen.dart';

/// 所有 VOD 播放跳转点统一走这里。
///
/// 应用只保留一套播放器（同步自 guoappxxx 的实现），不再提供
/// guo / xigua 两套播放器的切换，因此这里恒构建 [PlayerScreen]。
Route<T> buildPlayerRoute<T>(
  BuildContext context, {
  required DramaDetail detail,
  required int initialIndex,
  required AppRepository repository,
  required LocalStore store,
  double initialPosition = 0,
  bool localOnly = false,
  bool allowOnlineFallback = true,
  String? mediaId,
  LanIncomingPlayback? handoff,
  String searchKeyword = '',
}) {
  return playerRoute(
        PlayerScreen(
          detail: detail,
          initialIndex: initialIndex,
          initialPosition: initialPosition,
          localOnly: localOnly,
          allowOnlineFallback: allowOnlineFallback,
          mediaId: mediaId,
          handoff: handoff,
          repository: repository,
          store: store,
        ),
      )
      as Route<T>;
}

Future<T?> pushPlayer<T>(
  BuildContext context, {
  required DramaDetail detail,
  required int initialIndex,
  required AppRepository repository,
  required LocalStore store,
  double initialPosition = 0,
  bool localOnly = false,
  bool allowOnlineFallback = true,
  String? mediaId,
  LanIncomingPlayback? handoff,
  String searchKeyword = '',
}) => Navigator.of(context).push<T>(
  buildPlayerRoute<T>(
    context,
    detail: detail,
    initialIndex: initialIndex,
    initialPosition: initialPosition,
    localOnly: localOnly,
    allowOnlineFallback: allowOnlineFallback,
    mediaId: mediaId,
    handoff: handoff,
    repository: repository,
    store: store,
    searchKeyword: searchKeyword,
  ),
);
