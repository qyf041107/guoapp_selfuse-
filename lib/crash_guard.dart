import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';

import 'diary_service.dart';

/// 全局未捕获异常兜底。
///
/// 桌面/电视端没有系统崩溃收集器，任何 zone 外的 Dart 异常、渲染期抛错
/// 默认都会让进程直接退出（表现为「闪退」）。这里接管 [FlutterError.onError]
/// 与 [PlatformDispatcher.instance.onError]：先把异常与堆栈追加写盘到崩溃
/// 日志，同时记入 [DiaryService]，再交回框架默认处理 / 抑制终止行为，使
/// 「闪退」可被事后定位（崩溃日志带 ISO 时间戳，能对齐「启动到闪退」的耗时
/// 与所处阶段）。
///
/// 局限：原生层（Go FFI c-shared 库）段错误 Dart 无法捕获，本兜底只能记录
/// 「闪退前最后一条异常」；原生崩溃需结合系统 WER / 崩溃转储定位。
class CrashGuard {
  CrashGuard._();
  static bool _installed = false;

  /// 安装全局兜底。须在 [WidgetsFlutterBinding.ensureInitialized] 之后调用。
  static Future<void> install() async {
    if (_installed) return;
    _installed = true;
    final previousOnError = FlutterError.onError;
    FlutterError.onError = (details) {
      unawaited(_report('ui', details.exception, details.stack));
      previousOnError?.call(details);
    };
    PlatformDispatcher.instance.onError = (error, stack) {
      unawaited(_report('async', error, stack));
      // 标记为已处理，避免框架因未捕获异常终止进程。
      return true;
    };
  }

  static Future<void> _report(
    String kind,
    Object error,
    StackTrace? stack,
  ) async {
    final entry =
        '[${DateTime.now().toIso8601String()}] [CrashGuard/$kind] $error\n${stack ?? StackTrace.current}\n';
    DiaryService.add('[CrashGuard/$kind] $error');
    try {
      final directory = await getApplicationSupportDirectory();
      final file = File(
        '${directory.path}${Platform.pathSeparator}crash.log',
      );
      await file.writeAsString(
        entry,
        mode: FileMode.append,
        flush: true,
      );
    } catch (error) {
      debugPrint('[CrashGuard] 写崩溃日志失败：$error');
    }
  }
}
