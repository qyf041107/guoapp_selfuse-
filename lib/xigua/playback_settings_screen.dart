import 'dart:io';

import 'package:flutter/material.dart';

import 'package:duanju_app/app_layout.dart';
import 'package:duanju_app/glass_panel.dart';
import 'package:duanju_app/local_store.dart';
import 'package:duanju_app/playback_preferences.dart';
import 'package:duanju_app/widgets.dart';

class PlaybackSettingsScreen extends StatelessWidget {
  const PlaybackSettingsScreen({super.key, required this.store});
  final LocalStore store;

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: store,
    builder: (context, _) {
      final preferences = store.playbackPreferences;
      final enabled = !store.locked;
      return Scaffold(
        appBar: AppBar(title: const Text('播放设置')),
        body: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(16),
              children: [
                ListTile(
                  title: const Text('长按倍速'),
                  subtitle: const Text('长按画面左右两侧临时倍速播放，松开恢复；中央区域无长按动作。'),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: Wrap(
                    spacing: 8,
                    children: [
                      for (final value in holdSpeeds)
                        ChoiceChip(
                          key: ValueKey('hold-speed-$value'),
                          label: Text('${speedLabel(value)}x'),
                          selected: preferences.holdSpeed == value,
                          onSelected: !enabled
                              ? null
                              : (selected) {
                                  if (selected) {
                                    saveUserChange(
                                      context,
                                      () => store.setPlaybackPreferences(
                                        preferences.copyWith(holdSpeed: value),
                                      ),
                                    );
                                  }
                                },
                        ),
                    ],
                  ),
                ),
                const Divider(height: 32),
                ListTile(
                  title: const Text('左右滑动快进/快退'),
                  subtitle: const Text('按住画面左右拖动的定位速度：拖满一屏的秒数。'),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: Wrap(
                    spacing: 8,
                    children: [
                      for (final value in swipeSeekChoices)
                        ChoiceChip(
                          key: ValueKey('swipe-seek-$value'),
                          label: Text('$value 秒'),
                          selected: preferences.swipeSeekSeconds == value,
                          onSelected: !enabled
                              ? null
                              : (selected) {
                                  if (selected) {
                                    saveUserChange(
                                      context,
                                      () => store.setPlaybackPreferences(
                                        preferences.copyWith(
                                          swipeSeekSeconds: value,
                                        ),
                                      ),
                                    );
                                  }
                                },
                        ),
                    ],
                  ),
                ),
                const Divider(height: 32),
                ListTile(
                  title: const Text('默认播放倍速'),
                  subtitle: const Text('打开视频时的初始倍速，播放中仍可在倍速面板调整。'),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: Wrap(
                    spacing: 8,
                    children: [
                      for (final value in playbackSpeeds)
                        ChoiceChip(
                          key: ValueKey('play-speed-$value'),
                          label: Text('${speedLabel(value)}x'),
                          selected: preferences.speed == value,
                          onSelected: !enabled
                              ? null
                              : (selected) {
                                  if (selected) {
                                    saveUserChange(
                                      context,
                                      () => store.setPlaybackPreferences(
                                        preferences.copyWith(speed: value),
                                      ),
                                    );
                                  }
                                },
                        ),
                    ],
                  ),
                ),
                const Divider(height: 32),
                SwitchListTile(
                  value: preferences.autoAdvance,
                  title: const Text('自动连播'),
                  subtitle: const Text('当前集播完自动播放下一集。'),
                  onChanged: !enabled
                      ? null
                      : (value) => saveUserChange(
                          context,
                          () => store.setPlaybackPreferences(
                            preferences.copyWith(autoAdvance: value),
                          ),
                        ),
                ),
                SwitchListTile(
                  value: preferences.preload,
                  title: const Text('预加载下一集'),
                  subtitle: const Text('后台提前缓冲下一集，加快切换速度，消耗更多流量。'),
                  onChanged: !enabled
                      ? null
                      : (value) => saveUserChange(
                          context,
                          () => store.setPlaybackPreferences(
                            preferences.copyWith(preload: value),
                          ),
                        ),
                ),
                SwitchListTile(
                  value: preferences.danmaku,
                  title: const Text('显示弹幕'),
                  subtitle: const Text('打开视频时默认开启弹幕，播放中可随时开关。'),
                  onChanged: !enabled
                      ? null
                      : (value) => saveUserChange(
                          context,
                          () => store.setPlaybackPreferences(
                            preferences.copyWith(danmaku: value),
                          ),
                        ),
                ),
                if (Platform.isAndroid && !AppLayout.isTelevision(context)) ...[
                  const Divider(height: 32),
                  SwitchListTile(
                    key: const ValueKey('auto-picture-in-picture'),
                    value: preferences.autoPictureInPicture,
                    title: const Text('回到桌面自动开启小窗'),
                    subtitle: const Text(
                      '仅在播放页面回到桌面时进入小窗播放；关闭后只能用播放栏的小窗按钮进入，'
                      '小窗画面只包含视频本体并跟随视频横竖屏比例。',
                    ),
                    onChanged: !enabled
                        ? null
                        : (value) => saveUserChange(
                            context,
                            () => store.setPlaybackPreferences(
                              preferences.copyWith(autoPictureInPicture: value),
                            ),
                          ),
                  ),
                ],
                const Divider(height: 32),
                ListTile(
                  title: const Text('优先清晰度'),
                  subtitle: const Text('指定画质不可用时，使用源站提供的可用版本。'),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: GlassChoiceField<int>(
                    value: preferences.quality,
                    label: '清晰度偏好',
                    enabled: enabled,
                    entries: const [
                      (0, '自动 · 优先高清'),
                      (1080, '1080P'),
                      (720, '720P'),
                      (480, '480P'),
                    ],
                    onChanged: (value) => saveUserChange(
                      context,
                      () => store.setPlaybackPreferences(
                        preferences.copyWith(quality: value),
                      ),
                    ),
                  ),
                ),
                const Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('偏好保存在当前用户中，应用于在线播放与手势交互。'),
                ),
              ],
            ),
          ),
        ),
      );
    },
  );
}
