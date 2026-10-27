import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:media_kit/media_kit.dart';

import 'package:duanju_app/playback_preferences.dart';
import 'package:duanju_app/widgets.dart';

class PlayerInteractions extends ChangeNotifier {
  PlayerInteractions({
    required this.player,
    required this.available,
    required this.baseSpeed,
    required this.holdSpeed,
    required this.swipeSeek,
    required this.onTogglePlayback,
    required this.onFullscreen,
    required this.onEpisode,
    this.onSeek,
    this.onBrightness,
    this.currentBrightness,
  }) {
    _playing = player.stream.playing.listen((playing) {
      if (!playing) cancel();
    });
  }

  final Player player;
  final bool Function() available;
  final double Function() baseSpeed;
  final double Function() holdSpeed;
  final int Function() swipeSeek;
  final VoidCallback onTogglePlayback;
  final VoidCallback onFullscreen;
  final String Function(int direction) onEpisode;
  final Future<void> Function(Duration)? onSeek;
  final Future<void> Function(double value)? onBrightness;
  final double? Function()? currentBrightness;
  late final StreamSubscription<bool> _playing;
  Timer? _holdTimer;
  Timer? _hintTimer;
  Future<void> _rates = Future<void>.value();
  final Set<int> _pointers = {};
  int? _pointer;
  Offset? _origin;
  Offset? _lastPosition;
  Duration _started = Duration.zero;
  double _swipeThreshold = 70;
  bool _swipeEnabled = false;
  bool _moved = false;
  bool _held = false;
  bool _boosting = false;
  bool _seeking = false;
  int _holdSeekDirection = 0;
  bool _keyboardHold = false;
  bool _cancelUntilRelease = false;
  bool _disposed = false;
  double _unmutedVolume = 100;
  String _feedback = '';
  DateTime _ignoreTapUntil = DateTime(2000);
  int _seekTarget = 0;
  int _dragStart = 0;
  double _dragWidth = 1;
  double _dragSpan = 0;
  double _paneHeight = 0;
  bool _originOnLeft = false;
  bool _brightnessDragging = false;
  double _brightnessAnchor = .5;

  String get feedback => _feedback;
  bool get boosting => _boosting;
  bool get seeking => _seeking;
  bool get suppressTap => DateTime.now().isBefore(_ignoreTapUntil);
  Future<void> get pendingRates => _rates;

  void hint(String message, {bool persistent = false}) {
    if (_disposed) return;
    _hintTimer?.cancel();
    if (_feedback != message) {
      _feedback = message;
      notifyListeners();
    }
    if (!persistent && message.isNotEmpty) {
      _hintTimer = Timer(const Duration(milliseconds: 1200), () {
        hint(
          _boosting ? '${speedLabel(holdSpeed())} 倍速 · 松开恢复' : '',
          persistent: true,
        );
      });
    }
  }

  Future<void> applySpeed() => _setRate(baseSpeed());

  Future<void> _setRate(double value) {
    _rates = _rates
        .catchError((Object _) {})
        .then((_) => player.setRate(value));
    unawaited(
      _rates.catchError((Object _) {
        hint('倍速调整失败，请重试');
      }),
    );
    return _rates;
  }

  void _beginHold({bool keyboard = false}) {
    if (!available() || _holdTimer != null || _boosting) return;
    if (!keyboard && _holdSeekDirection == 0) return;
    _keyboardHold = keyboard;
    _holdTimer = Timer(const Duration(milliseconds: 350), () {
      _holdTimer = null;
      if (_disposed ||
          !available() ||
          !player.state.playing ||
          player.state.completed) {
        return;
      }
      _boosting = true;
      _held = true;
      unawaited(_setRate(holdSpeed()));
      hint('${speedLabel(holdSpeed())} 倍速 · 松开恢复', persistent: true);
    });
  }

  void _beginDragSeek() {
    _seeking = true;
    _held = true;
    _ignoreTapUntil = DateTime.now().add(const Duration(seconds: 5));
    _seekTarget = _dragStart = player.state.position.inMilliseconds;
    _dragSpan = swipeSeek() * 1000.0;
    _updateDragSeek();
  }

  void _updateDragSeek() {
    final origin = _origin;
    if (origin == null || _dragWidth <= 0) return;
    final dx = (_lastPosition ?? origin).dx - origin.dx;
    final duration = player.state.duration.inMilliseconds;
    _seekTarget = (_dragStart + dx / _dragWidth * _dragSpan).round().clamp(
      0,
      duration,
    );
    hint(
      '${dx > 0 ? '快进至' : '后退至'} ${formatPosition(_seekTarget / 1000)}',
      persistent: true,
    );
  }

  void _finishDragSeek({bool silent = false}) {
    if (!_seeking) return;
    _seeking = false;
    if (silent) return;
    final target = _seekTarget;
    unawaited((onSeek ?? player.seek)(Duration(milliseconds: target)));
    hint('已定位 ${formatPosition(target / 1000)}');
  }

  void _endHold({bool tap = false, bool silent = false}) {
    final wasKeyboard = _keyboardHold;
    final boosted = _boosting;
    _holdTimer?.cancel();
    _holdTimer = null;
    _keyboardHold = false;
    _boosting = false;
    _finishDragSeek(silent: silent);
    if (boosted) {
      unawaited(_setRate(baseSpeed()));
      if (!silent) hint('恢复 ${speedLabel(baseSpeed())} 倍速');
    } else if (tap && wasKeyboard) {
      seek(5);
    }
  }

  void cancel() {
    if (_disposed) return;
    if (_pointers.isNotEmpty) {
      _cancelUntilRelease = true;
      _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
    }
    _pointer = null;
    _origin = null;
    _lastPosition = null;
    _brightnessDragging = false;
    _endHold(silent: true);
    hint('');
  }

  void pointerDown(
    PointerDownEvent event, {
    required bool swipeEnabled,
    required double height,
    double width = 0,
  }) {
    _pointers.add(event.pointer);
    if (_pointers.length != 1 || _cancelUntilRelease) {
      cancel();
      return;
    }
    if (!available() || event.buttons != kPrimaryButton) return;
    _pointer = event.pointer;
    _origin = _lastPosition = event.localPosition;
    _started = event.timeStamp;
    _swipeEnabled = swipeEnabled && event.kind == PointerDeviceKind.touch;
    _swipeThreshold = math.max(56, math.min(100, height * .1));
    _dragWidth = width > 0 ? width : 1;
    _paneHeight = height;
    _originOnLeft = event.localPosition.dx < width / 2;
    _brightnessDragging = false;
    _brightnessAnchor = (currentBrightness?.call() ?? .5).clamp(.01, 1.0);
    _holdSeekDirection = _swipeEnabled && width > 0
        ? event.localPosition.dx < width * .38
              ? -1
              : event.localPosition.dx > width * .62
              ? 1
              : 0
        : 0;
    _moved = _held = false;
    _beginHold();
  }

  void pointerMove(PointerMoveEvent event) {
    if (_pointer != event.pointer || _origin == null) return;
    _lastPosition = event.localPosition;
    if (_seeking) {
      _updateDragSeek();
      return;
    }
    final delta = event.localPosition - _origin!;
    if (delta.distance > 12) {
      _moved = true;
      _holdSeekDirection = 0;
      _endHold();
      // 进度拖拽仲裁与 guo 版对齐：水平位移必须同时满足「绝对位移 > 24」与
      // 「明显占优」。旧写法只看比例，竖直慢拖起手的触摸水平噪声（dx≈15、dy≈8）
      // 会误进进度拖拽并永久锁死 _seeking，导致亮度分支再也进不去、拖半屏无反应。
      if (!_seeking &&
          !_brightnessDragging &&
          _swipeEnabled &&
          delta.dx.abs() > 24 &&
          delta.dx.abs() > delta.dy.abs() * 1.5 &&
          player.state.duration > Duration.zero) {
        _beginDragSeek();
        return;
      }
      // 左半屏“慢速”垂直拖动 = 系统亮度临时调整（按住超 260ms 才激活，
      // 快速竖直 flick 仍走原有上下滑动切集；右半屏手势不改动）。
      if (!_brightnessDragging &&
          _swipeEnabled &&
          _originOnLeft &&
          onBrightness != null &&
          delta.dy.abs() > 24 &&
          delta.dy.abs() > delta.dx.abs() * 1.2 &&
          event.timeStamp - _started > const Duration(milliseconds: 260)) {
        _brightnessDragging = true;
        _brightnessAnchor = (currentBrightness?.call() ?? .5).clamp(.01, 1.0);
      }
    }
    if (_brightnessDragging && _paneHeight > 0) {
      final value = (_brightnessAnchor - delta.dy / _paneHeight).clamp(
        .01,
        1.0,
      );
      unawaited(onBrightness?.call(value));
      hint('亮度 ${(value * 100).round()}%', persistent: true);
    }
  }

  void pointerUp(PointerUpEvent event) {
    _pointers.remove(event.pointer);
    if (_cancelUntilRelease) {
      _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
      if (_pointers.isEmpty) _cancelUntilRelease = false;
      return;
    }
    if (_pointer != event.pointer || _origin == null) return;
    final delta = (_lastPosition ?? event.localPosition) - _origin!;
    final swipe =
        _swipeEnabled &&
        !_held &&
        !_brightnessDragging &&
        _moved &&
        delta.dy.abs() >= _swipeThreshold &&
        delta.dy.abs() > delta.dx.abs() * 1.5 &&
        event.timeStamp - _started < const Duration(milliseconds: 1500);
    if (_moved || _held) {
      _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
    }
    _pointer = null;
    _origin = null;
    _holdSeekDirection = 0;
    if (_brightnessDragging) {
      _brightnessDragging = false;
      hint('');
    }
    _endHold();
    if (swipe && available()) hint(onEpisode(delta.dy < 0 ? 1 : -1));
  }

  void pointerCancel(PointerCancelEvent event) {
    _pointers.remove(event.pointer);
    cancel();
    _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
    if (_pointers.isEmpty) _cancelUntilRelease = false;
  }

  void seek(int seconds) {
    if (!available() || player.state.duration <= Duration.zero) return;
    _endHold(silent: true);
    final target = (player.state.position.inMilliseconds + seconds * 1000)
        .clamp(0, player.state.duration.inMilliseconds);
    unawaited((onSeek ?? player.seek)(Duration(milliseconds: target)));
    hint('${seconds > 0 ? '快进至' : '后退至'} ${formatPosition(target / 1000)}');
  }

  void changeVolume(double delta) {
    if (!available()) return;
    final volume = (player.state.volume + delta).clamp(0.0, 100.0);
    unawaited(player.setVolume(volume));
    if (volume > 0) _unmutedVolume = volume;
    hint(volume == 0 ? '已静音' : '音量 ${volume.round()}%');
  }

  void toggleMute() {
    if (!available()) return;
    final current = player.state.volume;
    if (current > 0) _unmutedVolume = current;
    final target = current > 0 ? 0.0 : _unmutedVolume;
    unawaited(player.setVolume(target));
    hint(target == 0 ? '已静音' : '音量 ${target.round()}%');
  }

  KeyEventResult key(KeyEvent event) {
    final key = event.logicalKey;
    if (event is KeyUpEvent) {
      if (key == LogicalKeyboardKey.arrowRight && _keyboardHold) {
        _endHold(tap: available());
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    }
    final hardware = HardwareKeyboard.instance;
    if (hardware.isAltPressed ||
        hardware.isMetaPressed ||
        hardware.isShiftPressed) {
      cancel();
      return KeyEventResult.ignored;
    }
    if (key == LogicalKeyboardKey.f11 || key == LogicalKeyboardKey.keyF) {
      if (event is KeyDownEvent) {
        cancel();
        onFullscreen();
      }
      return KeyEventResult.handled;
    }
    if (hardware.isControlPressed || !available()) {
      return KeyEventResult.ignored;
    }
    if (key != LogicalKeyboardKey.arrowRight) _endHold();
    if (key == LogicalKeyboardKey.arrowRight) {
      if (event is KeyDownEvent) _beginHold(keyboard: true);
    } else if (key == LogicalKeyboardKey.arrowLeft) {
      seek(-5);
    } else if (key == LogicalKeyboardKey.arrowUp) {
      changeVolume(5);
    } else if (key == LogicalKeyboardKey.arrowDown) {
      changeVolume(-5);
    } else if (key == LogicalKeyboardKey.space ||
        key == LogicalKeyboardKey.mediaPlayPause) {
      if (event is KeyDownEvent) onTogglePlayback();
    } else if (key == LogicalKeyboardKey.keyM) {
      if (event is KeyDownEvent) toggleMute();
    } else if (key == LogicalKeyboardKey.mediaTrackNext ||
        key == LogicalKeyboardKey.mediaTrackPrevious) {
      if (event is KeyDownEvent) {
        hint(onEpisode(key == LogicalKeyboardKey.mediaTrackNext ? 1 : -1));
      }
    } else {
      return KeyEventResult.ignored;
    }
    return KeyEventResult.handled;
  }

  @override
  void dispose() {
    _disposed = true;
    _holdTimer?.cancel();
    _hintTimer?.cancel();
    _seeking = false;
    if (_boosting) unawaited(_setRate(baseSpeed()));
    _boosting = false;
    _playing.cancel();
    _pointers.clear();
    super.dispose();
  }
}
