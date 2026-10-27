import 'dart:async';

import 'package:flutter/material.dart';

import 'app_layout.dart';
import 'app_theme.dart';
import 'glass_panel.dart';
import 'remote_widgets.dart';

enum SleepTimerChoice {
  off,
  minutes15,
  minutes30,
  minutes60,
  minutes90,
  finishEpisode;

  String get label => switch (this) {
    SleepTimerChoice.off => '关闭睡眠定时',
    SleepTimerChoice.minutes15 => '15 分钟',
    SleepTimerChoice.minutes30 => '30 分钟',
    SleepTimerChoice.minutes60 => '60 分钟',
    SleepTimerChoice.minutes90 => '90 分钟',
    SleepTimerChoice.finishEpisode => '播完本集',
  };

  int? get minutes => switch (this) {
    SleepTimerChoice.minutes15 => 15,
    SleepTimerChoice.minutes30 => 30,
    SleepTimerChoice.minutes60 => 60,
    SleepTimerChoice.minutes90 => 90,
    _ => null,
  };
}

class SleepTimerController extends ChangeNotifier {
  SleepTimerController({this.onExpire});

  final VoidCallback? onExpire;

  DateTime? _deadline;
  bool _finishEpisode = false;
  Timer? _ticker;
  Timer? _expiry;

  bool get active => _deadline != null || _finishEpisode;
  bool get finishEpisode => _finishEpisode;

  Duration? get remaining {
    final deadline = _deadline;
    if (deadline == null) return null;
    final value = deadline.difference(DateTime.now());
    return value.isNegative ? Duration.zero : value;
  }

  String get label {
    if (_finishEpisode) return '播完本集';
    final value = remaining;
    if (value == null) return '';
    final minutes = (value.inSeconds / 60).ceil();
    if (minutes >= 60) {
      final hours = minutes ~/ 60;
      final rest = minutes % 60;
      return rest == 0 ? '$hours 时' : '$hours 时 $rest 分';
    }
    return '$minutes 分';
  }

  void select(SleepTimerChoice choice) {
    if (choice == SleepTimerChoice.off) {
      cancel();
    } else if (choice == SleepTimerChoice.finishEpisode) {
      startFinishEpisode();
    } else {
      startMinutes(choice.minutes!);
    }
  }

  void startMinutes(int minutes) {
    cancel();
    _deadline = DateTime.now().add(Duration(minutes: minutes));
    _expiry = Timer(Duration(minutes: minutes), () {
      cancel();
      onExpire?.call();
    });
    _startTicker();
    notifyListeners();
  }

  void startFinishEpisode() {
    cancel();
    _finishEpisode = true;
    notifyListeners();
  }

  void cancel() {
    _expiry?.cancel();
    _expiry = null;
    _ticker?.cancel();
    _ticker = null;
    final wasActive = _deadline != null || _finishEpisode;
    _deadline = null;
    _finishEpisode = false;
    if (wasActive) notifyListeners();
  }

  bool consumeFinishEpisode() {
    if (!_finishEpisode) return false;
    _finishEpisode = false;
    notifyListeners();
    return true;
  }

  void _startTicker() {
    _ticker?.cancel();
    _ticker = Timer.periodic(const Duration(seconds: 15), (_) {
      notifyListeners();
    });
  }

  @override
  void dispose() {
    _expiry?.cancel();
    _ticker?.cancel();
    _ticker = null;
    super.dispose();
  }
}

Future<void> showSleepTimerSheet(
  BuildContext context, {
  required SleepTimerController controller,
  required ValueChanged<SleepTimerChoice> onSelect,
  bool showFinishEpisode = true,
}) async {
  final options = [
    SleepTimerChoice.off,
    SleepTimerChoice.minutes15,
    SleepTimerChoice.minutes30,
    SleepTimerChoice.minutes60,
    SleepTimerChoice.minutes90,
    if (showFinishEpisode) SleepTimerChoice.finishEpisode,
  ];
  final choice = await showGeneralDialog<Object>(
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
    pageBuilder: (dialogContext, _, _) => Theme(
      data: AppTheme.dark,
      child: AnimatedBuilder(
        animation: controller,
        builder: (_, _) => AppLayout.isTelevision(context)
            ? TelevisionActionDialog(
                title: controller.active
                    ? '睡眠定时 · ${controller.label}'
                    : '睡眠定时',
                options: [
                  for (final option in options)
                    TelevisionAction(
                      value: option.name,
                      label: option.label,
                      icon: Icons.bedtime_outlined,
                    ),
                ],
              )
            : GlassDialog(
                maxWidth: 320,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(18, 16, 10, 8),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Padding(
                        padding: const EdgeInsets.only(left: 4, bottom: 4),
                        child: Text(
                          controller.active
                              ? '睡眠定时 · ${controller.label}'
                              : '睡眠定时',
                          style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                      for (final option in options)
                        InkWell(
                          key: ValueKey('sleep-timer-${option.name}'),
                          borderRadius: BorderRadius.circular(12),
                          onTap: () => Navigator.pop(dialogContext, option),
                          child: Padding(
                            padding: const EdgeInsets.symmetric(
                              vertical: 9,
                              horizontal: 10,
                            ),
                            child: Text(
                              option.label,
                              style: const TextStyle(fontSize: 14),
                            ),
                          ),
                        ),
                      Align(
                        alignment: Alignment.centerRight,
                        child: TextButton(
                          onPressed: () => Navigator.pop(dialogContext),
                          child: const Text('返回播放'),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
      ),
    ),
  );
  if (choice is SleepTimerChoice) onSelect(choice);
  if (choice is String) {
    onSelect(
      SleepTimerChoice.values.firstWhere((option) => option.name == choice),
    );
  }
}
