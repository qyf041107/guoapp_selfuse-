import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'app_layout.dart';
import 'app_notice.dart';
import 'app_orientation.dart';
import 'app_theme.dart';
import 'live_playback.dart';
import 'live_sources.dart';
import 'live_store.dart';
import 'package:duanju_app/xigua/player_controls.dart';
import 'package:duanju_app/xigua/player_interactions.dart';
import 'package:duanju_app/xigua/television_controls.dart';
import 'widgets.dart';
import 'sleep_timer.dart';

/// 直播全屏：复用点播播放器的控件层，保证与点播一致的观感与操作。
class LivePlayerScreen extends StatefulWidget {
  const LivePlayerScreen({
    super.key,
    required this.playback,
    required this.source,
    required this.store,
    required this.onExit,
  });

  final LivePlaybackController playback;
  final LiveSource source;
  final LiveStore store;
  final VoidCallback onExit;

  @override
  State<LivePlayerScreen> createState() => _LivePlayerScreenState();
}

class _LivePlayerScreenState extends State<LivePlayerScreen> {
  late final PlayerInteractions _interactions;
  final _focus = FocusNode();
  bool _panel = false;
  AppOrientationController? _orientationController;
  bool _television = false;
  bool _landscape = true;
  bool _rotated = false;

  LivePlaybackController get _playback => widget.playback;

  @override
  void initState() {
    super.initState();
    _playback.addListener(_changed);
    _interactions = PlayerInteractions(
      player: _playback.player!,
      available: () => !_panel && _playback.error == null,
      baseSpeed: () => 1,
      holdSpeed: () => 1,
      swipeSeek: () => 60,
      onTogglePlayback: _toggle,
      onFullscreen: _exit,
      onEpisode: _switchHint,
      onSeek: (duration) async {},
    );
    if (Platform.isAndroid) {
      SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    }
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _television = AppLayout.isTelevision(context);
    _orientationController = AppOrientationScope.maybeOf(context);
    _followVideoRatio();
    unawaited(_applyOrientation());
  }

  /// 全屏方向跟随画面比例：竖屏流全屏仍是竖屏，横屏流全屏是横屏。
  /// 用户按过旋转按钮后以手动选择为准，不再自动跟随。
  bool _followVideoRatio() {
    if (_television || _rotated) return false;
    final landscape = _playback.aspectRatio >= 1;
    if (landscape == _landscape) return false;
    _landscape = landscape;
    return true;
  }

  /// 进页面即按画面比例锁方向并全屏；旋转按钮在横竖屏之间切换。
  Future<void> _applyOrientation() async {
    if (_television) return;
    try {
      await _orientationController?.setPlayback(
        this,
        fullscreen: true,
        aspectRatio: _landscape ? 16 / 9 : 9 / 16,
      );
    } catch (_) {
      // 方向被系统策略拒绝时保持当前方向，不影响播放。
    }
  }

  Future<void> _toggleRotate() async {
    if (_television) return;
    _rotated = true;
    setState(() => _landscape = !_landscape);
    await _applyOrientation();
    if (Platform.isAndroid) {
      await SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    }
  }

  @override
  void dispose() {
    _playback.removeListener(_changed);
    _interactions.dispose();
    _focus.dispose();
    unawaited(
      (_orientationController?.releasePlayback(this) ?? Future<void>.value())
          .catchError((Object _) {}),
    );
    if (Platform.isAndroid) {
      SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    }
    super.dispose();
  }

  void _changed() {
    if (!mounted) return;
    if (_followVideoRatio()) {
      unawaited(_applyOrientation());
      if (Platform.isAndroid) {
        unawaited(
          SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky),
        );
      }
    }
    setState(() {});
  }

  void _toggle() {
    final player = _playback.player;
    if (player == null) return;
    unawaited(player.playOrPause());
  }

  void _exit() {
    widget.onExit();
    Navigator.maybePop(context);
  }

  String _switchHint(int direction) {
    _playback.switchBy(direction);
    return direction > 0 ? '下一个频道' : '上一个频道';
  }

  Future<void> _showSleepTimer() async {
    if (_panel) return;
    _interactions.cancel();
    setState(() => _panel = true);
    try {
      await showSleepTimerSheet(
        context,
        controller: _playback.sleepTimer,
        showFinishEpisode: false,
        onSelect: (choice) {
          if (mounted) _playback.sleepTimer.select(choice);
        },
      );
    } finally {
      if (mounted) setState(() => _panel = false);
    }
  }

  Future<void> _selectChannel() async {
    final channels = _playback.channels;
    if (channels.isEmpty) return;
    setState(() => _panel = true);
    final index = await showModalBottomSheet<int>(
      context: context,
      showDragHandle: true,
      backgroundColor: const Color(0xFF1B1B1F),
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      '${widget.source.name} · ${channels.length} 个频道',
                      style: const TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            Flexible(
              child: ListView.builder(
                shrinkWrap: true,
                itemCount: channels.length,
                itemBuilder: (context, index) {
                  final selected = index == _playback.index;
                  return ListTile(
                    dense: true,
                    selected: selected,
                    title: Text(
                      channels[index].label,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(color: Colors.white),
                    ),
                    onTap: () => Navigator.pop(context, index),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
    if (!mounted) return;
    setState(() => _panel = false);
    if (index != null) await _playback.play(index);
  }

  Future<void> _selectRoute() async {
    final channel = _playback.channel;
    if (channel == null || channel.urls.length < 2) {
      _toast('当前频道只有一条线路');
      return;
    }
    final index = await showModalBottomSheet<int>(
      context: context,
      showDragHandle: true,
      backgroundColor: const Color(0xFF1B1B1F),
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (var index = 0; index < channel.urls.length; index++)
              ListTile(
                dense: true,
                leading: const Icon(Icons.route_rounded),
                title: Text(
                  '线路 ${index + 1}',
                  style: const TextStyle(color: Colors.white),
                ),
                onTap: () => Navigator.pop(context, index),
              ),
          ],
        ),
      ),
    );
    if (index == null || !mounted) return;
    await _playback.playRoute(index);
  }

  void _toast(String message) {
    if (!mounted) return;
    AppNotice.show(context, message);
  }

  @override
  Widget build(BuildContext context) {
    final television = AppLayout.isTelevision(context);
    final controller = _playback.controller;
    final channel = _playback.channel;
    final error = _playback.error;
    final player = _playback.player;
    final title = channel == null
        ? widget.source.name
        : '${channel.name} · ${widget.source.name}';
    final Widget controls = player == null
        ? const SizedBox.shrink()
        : television
        ? TelevisionControls(
            player: player,
            title: title,
            enabled: error == null,
            showOnPlaybackReady: false,
            onTogglePlayback: _toggle,
            onSeek: (_) {},
            onPrevious: () => _playback.switchBy(-1),
            onNext: () => _playback.switchBy(1),
            onEpisodes: _selectChannel,
            onSettings: _selectRoute,
            onSleepTimer: _showSleepTimer,
            sleepTimerLabel: _playback.sleepTimer.label,
            onBack: _exit,
          )
        : PlayerControls(
            player: player,
            interactions: _interactions,
            enabled: error == null,
            panelOpen: _panel,
            fullscreen: true,
            showOnPlaybackReady: false,
            title: title,
            onTogglePlayback: _toggle,
            swipeEnabled: true,
            onRotate: _toggleRotate,
            onFullscreen: _exit,
            onBack: _exit,
            onFocusSurface: _focus.requestFocus,
            onSeek: (duration) async {},
            speed: 1,
            qualityLabel: channel != null && channel.urls.length > 1
                ? '线路 ${channel.urls.length}'
                : '线路',
            showBufferingMessage: true,
            onEpisodes: _selectChannel,
            onSpeed: () async => _toast('直播不支持变速'),
            onQuality: _selectRoute,
            onSleepTimer: _showSleepTimer,
            sleepTimerLabel: _playback.sleepTimer.label,
            onPrevious: () => _playback.switchBy(-1),
            onNext: () => _playback.switchBy(1),
          );
    return Scaffold(
      backgroundColor: Colors.black,
      body: Theme(
        data: AppTheme.dark,
        child: Stack(
          fit: StackFit.expand,
          children: [
            if (controller != null)
              Video(
                controller: controller,
                fit: BoxFit.contain,
                wakelock: false,
                controls: (_) => const SizedBox.shrink(),
              ),
            if (error != null)
              ColoredBox(
                color: Colors.black.withValues(alpha: .82),
                child: StatusPanel(
                  title: '直播暂时中断',
                  message: error,
                  icon: Icons.wifi_off_rounded,
                  action: '重新连接',
                  onRetry: () => unawaited(_playback.retry()),
                  secondaryAction: TextButton(
                    onPressed: _selectChannel,
                    child: const Text('换个频道'),
                  ),
                ),
              )
            else if (_playback.loading)
              const Center(
                child: SizedBox(
                  width: 38,
                  height: 38,
                  child: CircularProgressIndicator(strokeWidth: 3),
                ),
              ),
            if (error == null) Focus(focusNode: _focus, child: controls),
          ],
        ),
      ),
    );
  }
}
