import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'core_bridge.dart';
import 'live_models.dart';
import 'live_repository.dart';
import 'live_sources.dart';
import 'screen_awake.dart';
import 'video_output_size.dart';
import 'sleep_timer.dart';

/// 直播播放状态：顶部内嵌播放器与全屏播放共用同一实例，换台不重建引擎。
class LivePlaybackController extends ChangeNotifier {
  LivePlaybackController({required this.repository}) {
    sleepTimer.addListener(_sleepChanged);
  }

  static const maxRetries = 3;
  static const _reconnectDelay = Duration(seconds: 2);

  /// 起播后迟迟没有画面的判定时限。列表型直播源的代理线路会返回正常清单却
  /// 一直卡在缓冲，不触发错误事件，只靠 error 回调永远等不到换线。
  static const _stallTimeout = Duration(seconds: 15);

  final LiveRepository repository;

  late final sleepTimer = SleepTimerController(
    onExpire: () => unawaited(pauseForSleep()),
  );
  bool _sleepPaused = false;

  void _sleepChanged() {
    if (!_disposed) notifyListeners();
  }

  Future<void> pauseForSleep() async {
    if (_disposed) return;
    _sleepPaused = true;
    _generation++;
    _loading = false;
    _reconnect?.cancel();
    _stall?.cancel();
    _screenAwake.disable();
    try {
      await _player?.pause();
    } catch (_) {}
    if (!_disposed) notifyListeners();
  }

  final _screenAwake = ScreenAwake();
  final _subscriptions = <StreamSubscription<dynamic>>[];
  Timer? _reconnect;
  Timer? _stall;
  bool _frames = false;
  double _aspectRatio = 16 / 9;

  Player? _player;
  VideoController? _controller;
  LiveSource? _source;
  List<LiveChannel> _channels = const [];
  int _index = 0;
  int _generation = 0;
  int _retries = 0;
  int _route = 0;
  bool _loading = false;
  bool _buffering = false;
  bool _disposed = false;
  String? _error;

  Player? get player => _player;
  VideoController? get controller => _controller;
  LiveSource? get source => _source;
  List<LiveChannel> get channels => _channels;
  LiveChannel? get channel => _channels.isEmpty
      ? null
      : _channels[_index.clamp(0, _channels.length - 1)];
  int get index => _index;
  int get count => _channels.length;
  bool get loading => _loading;
  bool get buffering => _buffering;
  bool get ready => _player != null;
  int get route => _route;
  String? get error => _error;
  double get aspectRatio => _aspectRatio;

  void _ensurePlayer() {
    if (_player != null || _disposed) return;
    final player = Player();
    _player = player;
    // 直播流对 mpv 是非可寻址（non-seekable）源，demuxer 打开时会在 OSD/日志里
    // 打印 “not seekable … you can force it with -force -seekable=yes”。设为 yes
    // 后 mpv 用缓存强行可寻址，既消除该提示也支持直播内拖动。仅原生 mpv 生效，
    // Web 端 platform 非 NativePlayer 时跳过。
    final platform = player.platform;
    if (platform is NativePlayer) {
      unawaited(
        platform
            .setProperty('force-seekable', 'yes', waitForInitialization: true)
            .catchError((Object _) {}),
      );
    }
    _controller = VideoController(player);
    _subscriptions.add(player.stream.error.listen(_onError));
    _subscriptions.add(
      player.stream.buffering.listen((value) {
        if (_disposed || value == _buffering) return;
        _buffering = value;
        if (!value && player.state.playing) {
          _stall?.cancel();
        }
        notifyListeners();
      }),
    );
    _subscriptions.add(
      player.stream.playing.listen((value) {
        if (_disposed || !value) return;
        _stall?.cancel();
        if (_error == null) return;
        _error = null;
        notifyListeners();
      }),
    );
    _subscriptions.add(
      player.stream.videoParams.listen((parameters) {
        final size = videoDisplaySize(parameters);
        if (_disposed || size == null) return;
        _frames = true;
        _stall?.cancel();
        final ratio = size.width / size.height;
        if (ratio.isFinite &&
            ratio > 0 &&
            (ratio - _aspectRatio).abs() > 0.01) {
          _aspectRatio = ratio;
          notifyListeners();
        }
      }),
    );
  }

  bool get _hasPlayback {
    final player = _player;
    if (player == null) return false;
    if (_frames) return true;
    return player.state.playing && !player.state.buffering;
  }

  void _armStallWatch() {
    _stall?.cancel();
    _stall = Timer(_stallTimeout, () {
      if (_disposed || _error != null) return;
      // playing 事件与布防存在竞态：事件先到再布防时计时器无人取消，
      // 必须在触发时复核真实播放状态，避免正常播放被误判断流。
      if (_hasPlayback) return;
      _onError('当前线路长时间没有画面');
    });
  }

  /// 切换直播源或分类：重置频道表并按需播放第一条。
  Future<void> load(
    LiveSource source,
    List<LiveChannel> channels, {
    int index = 0,
    bool autoplay = true,
  }) async {
    _source = source;
    _channels = List.of(channels);
    _index = channels.isEmpty ? 0 : index.clamp(0, channels.length - 1);
    _retries = 0;
    _route = 0;
    _error = null;
    if (!autoplay || channels.isEmpty) {
      notifyListeners();
      return;
    }
    try {
      _ensurePlayer();
    } catch (error) {
      _error = '直播播放器无法启动：$error';
      notifyListeners();
      return;
    }
    await play(_index);
  }

  Future<void> play(int index, {bool reconnect = false}) async {
    if (_disposed || _channels.isEmpty) return;
    if (reconnect && _sleepPaused) return;
    if (!reconnect) _sleepPaused = false;
    final channel = _channels[index.clamp(0, _channels.length - 1)];
    final source = _source;
    if (source == null) return;
    if (!reconnect) _retries = 0;
    if (!reconnect && index != _index) _route = 0;
    final token = ++_generation;
    _index = index;
    _loading = true;
    if (!reconnect) _error = null;
    notifyListeners();
    try {
      _ensurePlayer();
      final player = _player;
      if (player == null) throw AppFailure('直播播放器无法启动');
      final plan = await _resolve(source, channel);
      if (_disposed || token != _generation) return;
      _frames = false;
      await player.open(
        Media(
          plan.url,
          httpHeaders: plan.headers.isEmpty ? null : plan.headers,
        ),
      );
      if (_sleepPaused) {
        await player.pause();
        return;
      }
      if (_disposed || token != _generation) return;
      _loading = false;
      _error = null;
      _screenAwake.enable();
      _armStallWatch();
      notifyListeners();
    } catch (error) {
      if (_disposed || token != _generation) return;
      _loading = false;
      _error = '$error';
      notifyListeners();
    }
  }

  /// 直连型直播源（面板 / 列表）支持在同一频道内切换线路。
  Future<LivePlayback> _resolve(LiveSource source, LiveChannel channel) {
    final multiRoute =
        source.protocol == LiveProtocol.pingtai ||
        source.protocol == LiveProtocol.list;
    if (_route > 0 && multiRoute && channel.urls.length > _route) {
      return Future.value(
        LivePlayback(url: channel.urls[_route], headers: channel.headers),
      );
    }
    return repository.playback(source, channel);
  }

  /// 切换到指定线路并重新播放。
  Future<void> playRoute(int route) {
    _route = route < 0 ? 0 : route;
    return play(_index);
  }

  /// 上一台 / 下一台，越界时环绕。
  void switchBy(int delta) {
    if (_channels.length < 2) return;
    final next = (_index + delta + _channels.length) % _channels.length;
    unawaited(play(next));
  }

  Future<void> retry() => play(_index, reconnect: true);

  void _onError(String message) {
    if (_sleepPaused) return;
    if (_disposed) return;
    _loading = false;
    _error = message.isEmpty ? '直播中断' : message;
    notifyListeners();
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    if (_sleepPaused) return;
    final channel = this.channel;
    // 线路数多于固定重试次数时必须走完整批线路：列表型源的可用线路可能排在
    // 最后一条，只允许推进 3 次就永远够不到它，用户只能手动一路切过去。
    final routes = channel?.urls.length ?? 0;
    final limit = routes > 1 ? routes : maxRetries;
    if (_retries >= limit) return;
    _retries++;
    // 实测列表型直播源的代理线路质量波动很大（同一批前缀会交替返回
    // 404/500 或掉到十几 KB/s），重连必须换线路，否则几次重试全部
    // 打在同一条已经劣化的线路上，表现为长时间转圈。
    if (channel != null && channel.urls.length > 1) {
      _route = (_route + 1) % channel.urls.length;
    }
    _reconnect?.cancel();
    _reconnect = Timer(_reconnectDelay, () {
      if (!_disposed) unawaited(play(_index, reconnect: true));
    });
  }

  @override
  void dispose() {
    _disposed = true;
    sleepTimer.removeListener(_sleepChanged);
    sleepTimer.dispose();
    _reconnect?.cancel();
    _stall?.cancel();
    for (final subscription in _subscriptions) {
      subscription.cancel();
    }
    _subscriptions.clear();
    _screenAwake.disable();
    _player?.dispose();
    _player = null;
    _controller = null;
    super.dispose();
  }
}
