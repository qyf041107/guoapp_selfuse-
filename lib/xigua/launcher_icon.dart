import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'package:duanju_app/app_build.dart';

/// 全站源版可随完整模式切换桌面入口；默认版保持原有短剧视界入口。
class LauncherIcon {
  static const _channel = MethodChannel('duanju/device');

  static bool get supported =>
      allSourcesEnabled && !kIsWeb && Platform.isAndroid;

  static Future<void> apply(bool full) async {
    if (!supported) return;
    try {
      await _channel.invokeMethod<bool>('setLauncherEdition', {'full': full});
    } catch (_) {
      // 切换失败时保留当前入口：桌面图标属于系统状态，不影响应用内功能。
    }
  }
}
