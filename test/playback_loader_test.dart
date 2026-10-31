import 'dart:async';

import 'package:duanju_app/core_bridge.dart';
import 'package:duanju_app/models.dart';
import 'package:duanju_app/playback_loader.dart';
import 'package:duanju_app/xigua/network_exit.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'fixtures.dart';

class DeferredRepository extends FixtureRepository {
  final pending = <int, Completer<PlaybackPlan>>{};
  final released = <String>[];
  Completer<PlaybackPlan>? alternate;
  int cancellations = 0;

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
    bool adBlock = true,
  }) {
    final request = Completer<PlaybackPlan>();
    pending[episode.number] = request;
    return request.future;
  }

  @override
  Future<PlaybackPlan> fallback(PlaybackPlan current) {
    alternate = Completer<PlaybackPlan>();
    return alternate!.future;
  }

  @override
  Future<void> cancelPlayback() async {
    cancellations++;
  }

  @override
  Future<void> release(String session) async => released.add(session);
}

class NetworkRetryRepository extends FixtureRepository {
  NetworkRetryRepository(this.failures);

  final int failures;
  int resolveCalls = 0;
  int resets = 0;

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
    bool adBlock = true,
  }) async {
    resolveCalls++;
    if (resolveCalls <= failures) {
      throw AppFailure('connection reset by peer');
    }
    return const PlaybackPlan(
      url: 'https://example.test/retry.mp4',
      session: 'retry',
    );
  }

  @override
  Future<void> resetNetwork() async {
    resets++;
  }
}

class BrowserChallengeRepository extends FixtureRepository {
  int resolveCalls = 0;
  int resets = 0;

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
    bool adBlock = true,
  }) async {
    resolveCalls++;
    throw AppFailure('站源要求浏览器验证');
  }

  @override
  Future<void> resetNetwork() async {
    resets++;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const deviceChannel = MethodChannel('duanju/device');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  setUp(() {
    NetworkExit.resetForTest();
    NetworkExit.debugSupportedOverride = true;
    messenger.setMockMethodCallHandler(deviceChannel, (call) async {
      if (call.method == 'vpnActive') return true;
      if (call.method == 'bindPhysicalNetwork') return true;
      return null;
    });
  });

  tearDown(() {
    NetworkExit.resetForTest();
    messenger.setMockMethodCallHandler(deviceChannel, null);
  });

  Episode episode(int number) =>
      Episode({'id': '$number', 'currentEpisode': number}, number);
  Future<void> tick() => Future<void>.delayed(Duration.zero);

  test('a later episode can start before an old resolver completes', () async {
    final repository = DeferredRepository();
    final loader = PlaybackLoader(repository);
    final old = loader.load(FixtureRepository.free, episode(1));
    await tick();
    final latest = loader.load(FixtureRepository.free, episode(2));
    await tick();
    repository.pending[2]!.complete(
      const PlaybackPlan(url: 'https://example.test/2.mp4', session: 'second'),
    );
    expect((await latest)?.session, 'second');
    repository.pending[1]!.complete(
      const PlaybackPlan(url: 'https://example.test/1.mp4', session: 'first'),
    );
    expect(await old, isNull);
    expect(repository.released, ['first']);
    expect(repository.cancellations, 2);
    await loader.close();
  });

  test('a stale failure does not replace the new playback result', () async {
    final repository = DeferredRepository();
    final loader = PlaybackLoader(repository);
    final old = loader.load(FixtureRepository.free, episode(1));
    await tick();
    final latest = loader.load(FixtureRepository.free, episode(2));
    await tick();
    repository.pending[1]!.completeError(AppFailure('old request failed'));
    expect(await old, isNull);
    repository.pending[2]!.completeError(AppFailure('current request failed'));
    await expectLater(latest, throwsA(isA<AppFailure>()));
    await loader.close();
  });

  test(
    'leaving playback releases a plan that arrives after disposal',
    () async {
      final repository = DeferredRepository();
      final loader = PlaybackLoader(repository);
      final result = loader.load(FixtureRepository.free, episode(1));
      await tick();
      await loader.close();
      repository.pending[1]!.complete(
        const PlaybackPlan(url: 'https://example.test/1.mp4', session: 'late'),
      );
      expect(await result, isNull);
      expect(repository.released, ['late']);
      expect(await loader.load(FixtureRepository.free, episode(2)), isNull);
      expect(repository.pending.length, 1);
    },
  );

  test(
    'changing episode cancels an in-flight fallback and releases its late session',
    () async {
      final repository = DeferredRepository();
      final loader = PlaybackLoader(repository);
      final pending = loader.fallback(
        const PlaybackPlan(
          url: 'https://example.test/first.mp4',
          session: 'first',
          routeCount: 2,
        ),
      );
      await tick();
      final current = loader.load(FixtureRepository.free, episode(2));
      await tick();
      repository.pending[2]!.complete(
        const PlaybackPlan(
          url: 'https://example.test/current.mp4',
          session: 'current',
        ),
      );
      expect((await current)?.session, 'current');
      repository.alternate!.complete(
        const PlaybackPlan(
          url: 'https://example.test/late.mp4',
          session: 'late',
        ),
      );
      expect(await pending, isNull);
      expect(repository.released, ['late']);
      await loader.close();
    },
  );

  test('retryable resolve failure switches exit and retries once', () async {
    final repository = NetworkRetryRepository(1);
    final loader = PlaybackLoader(repository);

    final plan = await loader.load(FixtureRepository.free, episode(1));

    expect(plan?.session, 'retry');
    expect(repository.resolveCalls, 2);
    expect(repository.resets, 1);
    await loader.close();
  });

  test('browser verification never switches network exit', () async {
    final repository = BrowserChallengeRepository();
    final loader = PlaybackLoader(repository);

    await expectLater(
      loader.load(FixtureRepository.free, episode(1)),
      throwsA(isA<AppFailure>()),
    );
    expect(repository.resolveCalls, 1);
    expect(repository.resets, 0);
    await loader.close();
  });

  test('a second failure is not retried again', () async {
    final repository = NetworkRetryRepository(2);
    final loader = PlaybackLoader(repository);

    await expectLater(
      loader.load(FixtureRepository.free, episode(1)),
      throwsA(isA<AppFailure>()),
    );
    expect(repository.resolveCalls, 2);
    expect(repository.resets, 1);
    await loader.close();
  });
}
