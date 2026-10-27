import 'dart:async';
import 'dart:ui';

import 'package:flutter/material.dart';

/// 顶部悬浮玻璃气泡通知：浮在应用最上层，约 3 秒后自动滑出消失，
/// 点击气泡可提前关闭；新通知到来时立即替换上一条。
class AppNotice {
  AppNotice._();

  static OverlayEntry? _entry;

  static void show(BuildContext context, String message) =>
      _present(context, message: message);

  static void showAction(
    BuildContext context,
    String message, {
    required String actionLabel,
    required VoidCallback onAction,
  }) => _present(
    context,
    message: message,
    actionLabel: actionLabel,
    onAction: onAction,
  );

  static void _present(
    BuildContext context, {
    required String message,
    String? actionLabel,
    VoidCallback? onAction,
  }) {
    _remove();
    final overlay = Overlay.maybeOf(context, rootOverlay: true);
    if (overlay == null) return;
    late final OverlayEntry entry;
    entry = OverlayEntry(
      builder: (_) => _NoticeView(
        entry: entry,
        message: message,
        actionLabel: actionLabel,
        onAction: onAction,
      ),
    );
    _entry = entry;
    overlay.insert(entry);
  }

  static void _remove() {
    final entry = _entry;
    _entry = null;
    if (entry != null && entry.mounted) entry.remove();
  }
}

class _NoticeView extends StatefulWidget {
  const _NoticeView({
    required this.entry,
    required this.message,
    this.actionLabel,
    this.onAction,
  });

  final OverlayEntry entry;
  final String message;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  State<_NoticeView> createState() => _NoticeViewState();
}

class _NoticeViewState extends State<_NoticeView>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final CurvedAnimation _curved;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 260),
      reverseDuration: const Duration(milliseconds: 200),
    )..forward();
    _curved = CurvedAnimation(
      parent: _controller,
      curve: Curves.easeOutCubic,
      reverseCurve: Curves.easeInCubic,
    );
    _timer = Timer(const Duration(milliseconds: 3000), _close);
  }

  @override
  void dispose() {
    _timer?.cancel();
    _curved.dispose();
    _controller.dispose();
    super.dispose();
  }

  void _close() {
    _timer?.cancel();
    _timer = null;
    if (_controller.status == AnimationStatus.reverse) return;
    _controller.reverse().whenComplete(() {
      final entry = widget.entry;
      if (entry.mounted) entry.remove();
    });
  }

  void _runAction() {
    final action = widget.onAction;
    _close();
    action?.call();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final dark = theme.brightness == Brightness.dark;
    final width = MediaQuery.sizeOf(context).width;
    final maxWidth = width - 32;
    return Positioned(
      top: 0,
      left: 0,
      right: 0,
      child: SafeArea(
        minimum: const EdgeInsets.only(top: 10),
        child: SlideTransition(
          position: Tween<Offset>(
            begin: const Offset(0, -1.2),
            end: Offset.zero,
          ).animate(_curved),
          child: FadeTransition(
            opacity: _curved,
            child: Center(
              child: ConstrainedBox(
                constraints: BoxConstraints(
                  maxWidth: maxWidth > 480 ? 480 : maxWidth,
                ),
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(24),
                    boxShadow: [
                      BoxShadow(
                        color: Colors.black.withValues(alpha: .2),
                        blurRadius: 18,
                        offset: const Offset(0, 6),
                      ),
                    ],
                  ),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(24),
                    child: BackdropFilter(
                      filter: ImageFilter.blur(sigmaX: 22, sigmaY: 22),
                      child: Material(
                        color: dark
                            ? Colors.black.withValues(alpha: .78)
                            : Colors.white.withValues(alpha: .88),
                        borderRadius: BorderRadius.circular(24),
                        child: GestureDetector(
                          behavior: HitTestBehavior.opaque,
                          onTap: _close,
                          child: Semantics(
                            container: true,
                            liveRegion: true,
                            button: true,
                            label: widget.message,
                            child: Padding(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 18,
                                vertical: 12,
                              ),
                              child: Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  Flexible(
                                    child: Text(
                                      widget.message,
                                      maxLines: 3,
                                      overflow: TextOverflow.ellipsis,
                                      style: TextStyle(
                                        fontSize: 14,
                                        height: 1.35,
                                        fontWeight: FontWeight.w500,
                                        color: theme.colorScheme.onSurface,
                                      ),
                                    ),
                                  ),
                                  if (widget.actionLabel != null &&
                                      widget.onAction != null) ...[
                                    const SizedBox(width: 12),
                                    GestureDetector(
                                      behavior: HitTestBehavior.opaque,
                                      onTap: _runAction,
                                      child: Padding(
                                        padding: const EdgeInsets.symmetric(
                                          horizontal: 4,
                                          vertical: 2,
                                        ),
                                        child: Text(
                                          widget.actionLabel!,
                                          style: TextStyle(
                                            fontSize: 14,
                                            height: 1.35,
                                            fontWeight: FontWeight.w700,
                                            color: theme.colorScheme.primary,
                                          ),
                                        ),
                                      ),
                                    ),
                                  ],
                                ],
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
