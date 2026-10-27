import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:media_kit/media_kit.dart';

import 'app_layout.dart';
import 'widgets.dart';

enum SwipeAction { none, brightness, episode, seek }

/// 一次触摸的滑动方向。首次越过判定距离后锁定，之后不再改判，
/// 避免斜着划时在「切集」和「快进」之间来回跳。
enum _SwipeAxis { none, horizontal, vertical }

class GestureHudState {
  const GestureHudState({
    this.type = SwipeAction.none,
    this.value = 0.0,
    this.label = '',
  });
  final SwipeAction type;

  /// 亮度档取值 0.0 ~ 1.0。
  final double value;

  /// 快进/快退这类只需要一句话的操作提示。
  final String label;
}

class PlayerInteractions extends ChangeNotifier {
  PlayerInteractions({
    required this.player,
    required this.available,
    required this.baseSpeed,
    required this.onTogglePlayback,
    required this.onFullscreen,
    required this.onEpisode,
    this.onSeek,
    this.holdRate = 3,
    this.seekStep = 15,
  }) {
    _playing = player.stream.playing.listen((playing) {
      if (!playing) cancel();
    });
    AppDevice.getBrightness()
        .then((val) {
          if (!_disposed) {
            _brightness = val;
            notifyListeners();
          }
        })
        .catchError((_) {});
  }

  final Player player;
  final bool Function() available;
  final double Function() baseSpeed;
  final VoidCallback onTogglePlayback;
  final VoidCallback onFullscreen;
  final String Function(int direction) onEpisode;
  final Future<void> Function(Duration)? onSeek;

  /// 长按画面时的临时倍速，来自「长按倍速」偏好（默认 3 倍）。
  final double holdRate;

  /// 左右滑动一次的快进/快退秒数，来自「滑动快进步长」偏好（默认 15 秒）。
  final int seekStep;
  String get _holdLabel {
    final rate = holdRate == holdRate.roundToDouble()
        ? holdRate.toInt().toString()
        : holdRate.toString();
    return '$rate 倍速 · 松开恢复';
  }

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
  double _horizontalThreshold = 60;
  bool _swipeEnabled = false;
  bool _moved = false;
  bool _leftZone = false;
  bool _brightnessActive = false;
  _SwipeAxis _axis = _SwipeAxis.none;
  bool _held = false;
  bool _boosting = false;
  bool _keyboardHold = false;
  bool _cancelUntilRelease = false;
  bool _disposed = false;
  double _unmutedVolume = 100;
  String _feedback = '';
  bool _feedbackRevealsControls = false;
  DateTime _ignoreTapUntil = DateTime(2000);
  GestureHudState _hudState = const GestureHudState();
  double _brightness = 0.5;
  double _initialBrightness = 0.5;
  double _viewHeight = 0.0;
  Timer? _hudTimer;

  GestureHudState get hudState => _hudState;
  double get brightness => _brightness;
  bool get isBrightnessActive => _hudState.type == SwipeAction.brightness;

  void setBrightnessDirect(double value) {
    if (_disposed) return;
    _brightness = value.clamp(0.01, 1.0);
    AppDevice.setBrightness(_brightness);
    _showHud(SwipeAction.brightness, _brightness);
  }

  void dismissBrightnessHud() {
    _scheduleDismissHud();
  }

  String get feedback => _feedback;

  /// 当前提示是否应该把控制条叫出来。
  ///
  /// 长按快进、切集、亮度这些“看画面就能懂”的操作只在画面中央留一行提示，
  /// 不应该把底部控制条顶出来；否则沉浸播放时每划一下都会闪一次控制条。
  bool get feedbackRevealsControls => _feedbackRevealsControls;
  bool get boosting => _boosting;
  bool get suppressTap => DateTime.now().isBefore(_ignoreTapUntil);
  Future<void> get pendingRates => _rates;

  void hint(
    String message, {
    bool persistent = false,
    bool revealControls = true,
  }) {
    if (_disposed) return;
    _hintTimer?.cancel();
    _feedbackRevealsControls = revealControls;
    if (_feedback != message) {
      _feedback = message;
      notifyListeners();
    }
    if (!persistent && message.isNotEmpty) {
      _hintTimer = Timer(const Duration(milliseconds: 1200), () {
        hint(
          _boosting ? _holdLabel : '',
          persistent: true,
          revealControls: false,
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
    // 键盘长按（方向键右）不落点，必须清掉上一次触摸留下的左侧判定，
    // 否则「先碰过左半屏、再用键盘长按」会误进亮度档。
    if (keyboard) _leftZone = false;
    _keyboardHold = keyboard;
    _holdTimer = Timer(const Duration(milliseconds: 350), () {
      _holdTimer = null;
      if (_disposed || !available()) return;
      // 左侧长按 = 亮度调节：先弹出亮度条，之后上下滑动才改变亮度。
      // 亮度档不要求正在播放，暂停时也要能调。
      if (_leftZone) {
        _held = true;
        _brightnessActive = true;
        AppDevice.getBrightness()
            .then((val) {
              if (!_disposed && _brightnessActive) {
                _initialBrightness = _brightness = val;
                _showHud(SwipeAction.brightness, _brightness);
              }
            })
            .catchError((_) {});
        return;
      }
      if (!player.state.playing || player.state.completed) return;
      _held = true;
      _boosting = true;
      unawaited(_setRate(holdRate));
      hint(_holdLabel, persistent: true, revealControls: false);
    });
  }

  void _endHold({bool tap = false, bool silent = false}) {
    final wasKeyboard = _keyboardHold;
    final boosted = _boosting;
    _holdTimer?.cancel();
    _holdTimer = null;
    _keyboardHold = false;
    _boosting = false;
    if (boosted) {
      unawaited(_setRate(baseSpeed()));
      if (!silent) {
        hint('恢复 ${baseSpeed()} 倍速', revealControls: false);
      }
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
    _axis = _SwipeAxis.none;
    if (_brightnessActive) {
      _brightnessActive = false;
      _scheduleDismissHud();
    } else if (_hudState.type == SwipeAction.seek) {
      // 手势被系统打断（多指、来电等）时，快进/快退预览同样要立刻收起，
      // 否则残影会一直挂在画面中央。
      _dismissHud();
    }
    _endHold(silent: true);
    hint('', revealControls: false);
  }

  void pointerDown(
    PointerDownEvent event, {
    required bool swipeEnabled,
    double width = 0.0,
    required double height,
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
    _swipeThreshold = math.max(30, math.min(80, height * .08));
    _horizontalThreshold = width > 0
        ? math.max(40, math.min(120, width * .12))
        : 60;
    _viewHeight = height;
    _moved = _held = false;
    _axis = _SwipeAxis.none;
    _brightnessActive = false;
    // 落点只决定「长按之后干什么」：左侧 35% 长按出来的是红果风格亮度条，
    // 短促的上下滑动一律走切集，左右滑动一律走快进/快退。
    _leftZone =
        _swipeEnabled &&
        width > 0 &&
        AppDevice.brightnessSupported &&
        event.localPosition.dx < width * 0.35;
    _beginHold();
  }

  void pointerMove(PointerMoveEvent event) {
    if (_pointer != event.pointer || _origin == null) return;
    _lastPosition = event.localPosition;
    final diff = event.localPosition - _origin!;
    if (diff.distance > 12) {
      _moved = true;
      _endHold();
    }
    if (!_swipeEnabled || !_moved || _viewHeight <= 0) return;

    final dx = event.localPosition.dx - _origin!.dx;
    // 向上滑动为增加，向下滑动为减少
    final dy = _origin!.dy - event.localPosition.dy;

    // 左侧长按已进入亮度档：只认上下滑动，左右滑动不再改判成快进。
    if (_brightnessActive) {
      final deltaRatio = dy / (_viewHeight * 0.6);
      _brightness = (_initialBrightness + deltaRatio).clamp(0.01, 1.0);
      AppDevice.setBrightness(_brightness);
      _showHud(SwipeAction.brightness, _brightness);
      return;
    }

    if (_axis == _SwipeAxis.none) {
      if (diff.distance < 16) return;
      _axis = dx.abs() > dy.abs() ? _SwipeAxis.horizontal : _SwipeAxis.vertical;
    }

    if (_axis == _SwipeAxis.horizontal && dx.abs() >= _horizontalThreshold) {
      // 左右滑动按固定步长快进/快退，滑动途中只给预览，抬手才真正跳转，
      // 免得一次滑动里反复 seek 把解码器拖死。
      final forward = dx > 0;
      _showHud(
        SwipeAction.seek,
        0,
        label: '${forward ? '快进' : '后退'} $seekStep 秒',
      );
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
    final vertical =
        delta.dy.abs() >= _swipeThreshold &&
        delta.dy.abs() > delta.dx.abs() * 1.5;
    final horizontal =
        delta.dx.abs() >= _horizontalThreshold &&
        delta.dx.abs() > delta.dy.abs() * 1.5;

    if (_brightnessActive) {
      // 亮度档在长按退出时就已生效，抬手只收起亮度条。
      _scheduleDismissHud();
    } else if (_swipeEnabled &&
        _moved &&
        horizontal &&
        _axis == _SwipeAxis.horizontal) {
      // 左右滑动：一次固定步长快进/快退。预览 HUD 只在该手势期间有效，
      // 抬手完成 seek 后立刻隐藏，不留残影；确认提示只在中上部留一行并
      // 自动淡出（immersive 不唤出控制条），避免那个黑框滞留几秒才消失。
      seek(delta.dx > 0 ? seekStep : -seekStep, immersive: true);
      _dismissHud();
    } else if (_swipeEnabled &&
        _moved &&
        vertical &&
        _axis == _SwipeAxis.vertical &&
        !_held &&
        event.timeStamp - _started < const Duration(milliseconds: 1500)) {
      // 短促的上下滑动：切换上一集 / 下一集（上滑下一集，下滑上一集）。
      if (available()) {
        hint(onEpisode(delta.dy < 0 ? 1 : -1), revealControls: false);
      }
    }

    // 兜底：滑动途中已弹出快进/快退预览，但本次抬手既未达水平判定
    // 也未达垂直判定（典型是斜滑），上面几个分支都不会收起预览，
    // 这里统一立即隐藏，避免 HUD 残留在屏幕上。
    if (_hudState.type == SwipeAction.seek) {
      _dismissHud();
    }

    if (_moved || _held) {
      _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
    }
    _pointer = null;
    _origin = null;
    _axis = _SwipeAxis.none;
    _brightnessActive = false;
    _endHold();
  }

  void pointerCancel(PointerCancelEvent event) {
    _pointers.remove(event.pointer);
    cancel();
    _ignoreTapUntil = DateTime.now().add(const Duration(milliseconds: 600));
    if (_pointers.isEmpty) _cancelUntilRelease = false;
  }

  void _showHud(SwipeAction action, double value, {String label = ''}) {
    if (_disposed) return;
    _hudTimer?.cancel();
    _hudState = GestureHudState(type: action, value: value, label: label);
    notifyListeners();
  }

  void _scheduleDismissHud() {
    _hudTimer?.cancel();
    _hudTimer = Timer(const Duration(milliseconds: 1000), () {
      if (_disposed) return;
      _dismissHud();
    });
  }

  /// 立即收起手势 HUD（左右滑动快进/快退抬手时用，不留残影）。
  void _dismissHud() {
    if (_disposed) return;
    _hudTimer?.cancel();
    _hudTimer = null;
    if (_hudState.type != SwipeAction.none) {
      _hudState = const GestureHudState();
      notifyListeners();
    }
  }

  void seek(int seconds, {bool immersive = false}) {
    if (!available() || player.state.duration <= Duration.zero) return;
    _endHold();
    final target = (player.state.position.inMilliseconds + seconds * 1000)
        .clamp(0, player.state.duration.inMilliseconds);
    unawaited((onSeek ?? player.seek)(Duration(milliseconds: target)));
    hint(
      '${seconds > 0 ? '快进至' : '后退至'} ${formatPosition(target / 1000)}',
      // 左右滑动这类沉浸式手势的确认提示只在中上部留一行字并自动淡出，
      // 不能把控制条唤出来重新计时，否则那个确认框会滞留几秒才消失。
      revealControls: !immersive,
    );
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
    _hudTimer?.cancel();
    AppDevice.resetBrightness(); // 离开播放器时自动恢复手机/平板系统默认亮度
    if (_boosting) unawaited(_setRate(baseSpeed()));
    _boosting = false;
    _playing.cancel();
    _pointers.clear();
    super.dispose();
  }
}
