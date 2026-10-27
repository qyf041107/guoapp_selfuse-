import 'package:duanju_app/core_bridge.dart';
import 'package:duanju_app/follow_state.dart';
import 'package:duanju_app/follow_updater.dart';
import 'package:duanju_app/local_store.dart';
import 'package:duanju_app/models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'fixtures.dart';

class FollowRepository extends FixtureRepository {
  FollowRepository({this.managed = true});
  final bool managed;
  final checked = <String>[];
  final failing = <String>{};
  final episodes = <String, int>{};
  Future<void> Function(Drama drama)? onCheck;

  @override
  bool get supportsSourceManagement => managed;

  @override
  Future<Drama> followCheck(Drama drama) async {
    checked.add(drama.id);
    await onCheck?.call(drama);
    if (failing.contains(drama.id)) throw AppFailure('合成网络错误');
    return Drama(
      id: drama.id,
      source: drama.source,
      sourceId: drama.sourceId,
      title: drama.title,
      episodes: episodes[drama.id] ?? drama.episodes,
    );
  }
}

Drama _drama(int id, {String releaseStatus = ''}) => Drama(
  id: 'hongguo:$id',
  source: 'hongguo',
  sourceId: '$id',
  title: '合成剧$id',
  episodes: 2,
  releaseStatus: releaseStatus,
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final start = DateTime(2026, 10, 1, 8);
  late DateTime now;
  late LocalStore store;
  late FollowRepository repository;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    store = testStore(await SharedPreferences.getInstance());
    addTearDown(store.dispose);
    repository = FollowRepository();
    now = start;
  });

  Future<void> watching(Drama drama) async {
    await store.toggleFavorite(drama);
    await store.saveWatch(
      WatchEntry(
        drama: drama,
        episode: 1,
        position: 10,
        duration: 100,
        updatedAt: start,
      ),
    );
  }

  FollowUpdater updater({int limit = 10}) {
    final value = FollowUpdater(
      repository,
      store,
      limit: limit,
      clock: () => now,
    );
    addTearDown(value.dispose);
    return value;
  }

  test('只检查在看和自动已看的连载剧，并把新集数写回追剧记录', () async {
    final active = _drama(1);
    final finished = _drama(2);
    final planned = _drama(3);
    final manual = _drama(4);
    final completed = _drama(5, releaseStatus: 'completed');
    await watching(active);
    await store.toggleFavorite(finished);
    await store.saveWatch(
      WatchEntry(
        drama: finished,
        episode: 2,
        position: 100,
        duration: 100,
        updatedAt: start,
      ),
    );
    await store.toggleFavorite(planned);
    await store.toggleFavorite(manual);
    await store.setFollowStatus(manual, FollowStatus.watched);
    await watching(completed);
    expect(store.following(finished.id)!.status, FollowStatus.watched);
    repository.episodes
      ..[active.id] = 5
      ..[finished.id] = 4;

    await updater().run();

    expect(repository.checked.toSet(), {active.id, finished.id});
    expect(store.following(active.id)!.newEpisodes, 3);
    expect(store.following(finished.id)!.status, FollowStatus.watching);
    expect(store.following(finished.id)!.newEpisodes, 2);
    expect(store.followChecks.keys.toSet(), {active.id, finished.id});
  });

  test('每轮限量，按上次检查时间轮转', () async {
    for (final id in [1, 2, 3]) {
      await watching(_drama(id));
    }
    final follow = updater(limit: 2);

    await follow.run();
    expect(repository.checked, hasLength(2));
    final first = repository.checked.toSet();

    now = start.add(const Duration(minutes: 1));
    repository.checked.clear();
    await follow.run();
    expect(repository.checked, hasLength(1));
    expect(first.contains(repository.checked.single), isFalse);

    repository.checked.clear();
    await follow.run();
    expect(repository.checked, isEmpty);

    now = start.add(const Duration(hours: 6));
    await follow.run();
    expect(repository.checked.toSet(), first);
  });

  test('单部剧失败按指数退避，不影响其他剧，重启后退避仍然有效', () async {
    final broken = _drama(1);
    final healthy = _drama(2);
    await watching(broken);
    await watching(healthy);
    repository.failing.add(broken.id);
    repository.episodes[healthy.id] = 6;

    await updater().run();
    expect(repository.checked.toSet(), {broken.id, healthy.id});
    expect(store.following(healthy.id)!.newEpisodes, 4);
    expect(store.followChecks[broken.id]!.failures, 1);
    expect(store.followChecks[healthy.id]!.failures, 0);

    repository.checked.clear();
    now = start.add(const Duration(hours: 6));
    await updater().run();
    expect(repository.checked, [healthy.id]);

    repository.checked.clear();
    now = start.add(const Duration(hours: 12));
    await updater().run();
    expect(repository.checked, [broken.id, healthy.id]);
    expect(store.followChecks[broken.id]!.failures, 2);

    repository.failing.clear();
    repository.checked.clear();
    now = start.add(const Duration(hours: 36));
    await updater().run();
    expect(repository.checked, contains(broken.id));
    expect(store.followChecks[broken.id]!.failures, 0);
  });

  test('关闭站源后不再检查该站源的追剧', () async {
    await watching(_drama(1));
    await store.setSourceEnabled('hongguo', false);

    await updater().run();

    expect(repository.checked, isEmpty);
    expect(store.followChecks, isEmpty);
  });

  test('切换用户后中止本轮，不写回结果', () async {
    final first = _drama(1);
    final second = _drama(2);
    await watching(first);
    await watching(second);
    repository.episodes[first.id] = 9;
    repository.onCheck = (_) async {
      repository.onCheck = null;
      await store.switchProfile('default');
    };

    await updater().run();

    expect(repository.checked, hasLength(1));
    expect(store.following(first.id)!.newEpisodes, 0);
    expect(store.followChecks, isEmpty);
  });

  test('启动后延迟执行，释放后不再执行，不支持站源管理时不启动', () async {
    await watching(_drama(1));
    final follow = FollowUpdater(
      repository,
      store,
      startDelay: const Duration(milliseconds: 30),
      clock: () => now,
    )..start();
    expect(repository.checked, isEmpty);
    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(repository.checked, hasLength(1));
    follow.dispose();

    repository.checked.clear();
    final disposed = FollowUpdater(
      repository,
      store,
      startDelay: const Duration(milliseconds: 30),
      recheck: Duration.zero,
    )..start();
    disposed.dispose();
    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(repository.checked, isEmpty);

    repository = FollowRepository(managed: false);
    final unmanaged = FollowUpdater(
      repository,
      store,
      startDelay: Duration.zero,
      recheck: Duration.zero,
    )..start();
    addTearDown(unmanaged.dispose);
    await Future<void>.delayed(const Duration(milliseconds: 30));
    expect(repository.checked, isEmpty);
  });
}
