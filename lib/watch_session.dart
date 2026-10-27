class WatchSession {
  final Stopwatch _clock = Stopwatch();
  double _pendingSeconds = 0;
  int _pendingEpisodes = 0;
  bool _played = false;
  bool _completed = false;

  void setPlaying(bool playing) {
    _collect();
    if (playing) {
      _played = true;
      _clock.start();
    } else {
      _clock.stop();
    }
  }

  void _collect() {
    _pendingSeconds += _clock.elapsedMicroseconds / 1000000;
    _clock.reset();
  }

  void beginEpisode() {
    setPlaying(false);
    _played = false;
    _completed = false;
  }

  void completeEpisode() {
    if (_completed || !_played) return;
    _completed = true;
    _pendingEpisodes++;
  }

  ({double seconds, int episodes}) take() {
    _collect();
    final sample = (seconds: _pendingSeconds, episodes: _pendingEpisodes);
    _pendingSeconds = 0;
    _pendingEpisodes = 0;
    return sample;
  }

  void restore(({double seconds, int episodes}) sample) {
    _pendingSeconds += sample.seconds;
    _pendingEpisodes += sample.episodes;
  }
}
