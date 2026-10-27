import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'package:duanju_app/glass_panel.dart';

/// 查询是否有写入共享存储的能力；非 Android 恒为已授权。
Future<bool> hasAllFilesAccess() async {
  if (!Platform.isAndroid) return true;
  try {
    return await const MethodChannel(
          'duanju/device',
        ).invokeMethod<bool>('hasAllFilesAccess') ??
        true;
  } catch (_) {
    return true;
  }
}

/// 跳系统「所有文件访问」授权页（Android 11+）或弹运行时权限对话框。
Future<void> requestStorageAccess() async {
  if (!Platform.isAndroid) return;
  try {
    await const MethodChannel(
      'duanju/device',
    ).invokeMethod<void>('requestStorageAccess');
  } catch (_) {}
}

/// 未授权时弹引导对话框；用户点「去授权」后跳系统设置。
/// 返回引导结束时是否已授权（授权需用户在系统页操作，返回应用后需重新检测）。
Future<bool> ensureStorageAccess(BuildContext context) async {
  if (await hasAllFilesAccess()) return true;
  final open = await showGeneralDialog<bool>(
    context: context,
    barrierDismissible: true,
    barrierLabel: MaterialLocalizations.of(context).modalBarrierDismissLabel,
    barrierColor: Colors.black26,
    transitionBuilder: (context, animation, secondaryAnimation, child) {
      final curved = CurvedAnimation(
        parent: animation,
        curve: Curves.easeOutCubic,
        reverseCurve: Curves.easeInCubic,
      );
      return FadeTransition(
        opacity: curved,
        child: ScaleTransition(
          scale: Tween<double>(begin: .96, end: 1).animate(curved),
          child: child,
        ),
      );
    },
    pageBuilder: (dialogContext, _, _) => GlassDialog(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(18, 16, 18, 10),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.only(left: 4, bottom: 8),
              child: Text(
                '需要存储权限',
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
            const Padding(
              padding: EdgeInsets.only(left: 4, right: 4, bottom: 6),
              child: Text(
                'Android 限制应用只能写入自己的私有目录。要下载完整剧集并把视频导出、复制到所选文件夹，'
                '请在系统设置中授予「所有文件访问」权限；授权后返回应用即可继续。',
                style: TextStyle(fontSize: 14, height: 1.5),
              ),
            ),
            Align(
              alignment: Alignment.centerRight,
              child: Padding(
                padding: const EdgeInsets.only(right: 8),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    TextButton(
                      key: const ValueKey('storage-access-cancel'),
                      onPressed: () => Navigator.pop(dialogContext, false),
                      child: const Text('取消'),
                    ),
                    FilledButton(
                      key: const ValueKey('storage-access-open'),
                      onPressed: () => Navigator.pop(dialogContext, true),
                      child: const Text('去授权'),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    ),
  );
  if (open == true) await requestStorageAccess();
  return false;
}
