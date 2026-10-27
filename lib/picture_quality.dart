/// 三态画质档：原画 / 锐化 / Anime4K。
///
/// 与 [VideoEnhancementPreferences]（自适应增强档位）并存，互不影响。
/// 该偏好随 [PlaybackPreferences] 持久化，guo / xigua 两个播放器共享，
/// 切换播放器时选择不丢失。
enum PictureQualityMode {
  original('原画'),
  sharpen('锐化'),
  anime4k('Anime4K');

  const PictureQualityMode(this.label);
  final String label;
}

class PictureQualityPreferences {
  const PictureQualityPreferences({
    this.mode = PictureQualityMode.original,
    this.strength = 50,
  });

  final PictureQualityMode mode;

  /// 锐化强度 0–100，仅 [PictureQualityMode.sharpen] 时使用。
  final int strength;

  bool get isOriginal => mode == PictureQualityMode.original;

  PictureQualityPreferences copyWith({
    PictureQualityMode? mode,
    int? strength,
  }) {
    return PictureQualityPreferences(
      mode: mode ?? this.mode,
      strength: strength ?? this.strength,
    );
  }

  Map<String, dynamic> toJson() => {'mode': mode.name, 'strength': strength};

  factory PictureQualityPreferences.fromJson(Object? value) {
    if (value is! Map) return const PictureQualityPreferences();
    final strength = (value['strength'] as num?)?.round() ?? 50;
    return PictureQualityPreferences(
      mode:
          PictureQualityMode.values
              .where((mode) => mode.name == value['mode'])
              .firstOrNull ??
          PictureQualityMode.original,
      strength: strength.clamp(0, 100),
    );
  }

  @override
  bool operator ==(Object other) =>
      other is PictureQualityPreferences &&
      other.mode == mode &&
      other.strength == strength;

  @override
  int get hashCode => Object.hash(mode, strength);
}
