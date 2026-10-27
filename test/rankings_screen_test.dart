import 'package:duanju_app/app_layout.dart';
import 'package:duanju_app/models.dart';
import 'package:duanju_app/ranking_models.dart';
import 'package:duanju_app/rankings_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'fixtures.dart';

class _RankingRepository extends FixtureRepository {
  @override
  Future<List<RankingBoard>> rankingBoards() async => const [
    RankingBoard(id: 'hongguo-hot', source: 'hongguo', name: '热播'),
  ];

  @override
  Future<RankingPage> rankings(
    String board, {
    int page = 1,
    bool force = false,
  }) async => RankingPage(
    page: page,
    hasMore: page < 2,
    items: [
      for (var i = 1; i <= 10; i++)
        RankingItem(
          (page - 1) * 10 + i,
          Drama(
            id: 'hongguo:rank-${(page - 1) * 10 + i}',
            source: 'hongguo',
            title: '合成榜单${(page - 1) * 10 + i}',
            episodes: 1,
          ),
        ),
    ],
  );
}

void main() {
  testWidgets('television remote moves focus through ranking list', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final store = testStore(await SharedPreferences.getInstance());
    addTearDown(store.dispose);
    await tester.binding.setSurfaceSize(const Size(960, 540));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    FocusManager.instance.highlightStrategy =
        FocusHighlightStrategy.alwaysTraditional;
    addTearDown(
      () => FocusManager.instance.highlightStrategy =
          FocusHighlightStrategy.automatic,
    );
    await tester.pumpWidget(
      MaterialApp(
        builder: (context, child) => AppLayout(
          television: true,
          child: Theme(
            data: televisionTheme(Theme.of(context)),
            child: FocusTraversalGroup(child: child!),
          ),
        ),
        home: RankingsScreen(
          repository: _RankingRepository(),
          store: store,
          initialGroup: 'hongguo',
        ),
      ),
    );
    await tester.pumpAndSettle();

    String? focusedRank() {
      String? rank;
      FocusManager.instance.primaryFocus?.context?.visitAncestorElements((
        element,
      ) {
        final key = element.widget.key;
        if (key is ValueKey<String> && key.value.startsWith('rank-')) {
          rank = key.value.split('-')[1];
          return false;
        }
        return true;
      });
      return rank;
    }

    Future<void> press(LogicalKeyboardKey key) async {
      await tester.sendKeyEvent(key);
      await tester.pumpAndSettle();
    }

    await press(LogicalKeyboardKey.arrowDown);
    await press(LogicalKeyboardKey.arrowDown);
    expect(focusedRank(), '1');
    for (var rank = 2; rank <= 14; rank++) {
      await press(LogicalKeyboardKey.arrowDown);
      expect(focusedRank(), '$rank');
      final rect = tester.getRect(
        find.byKey(ValueKey('rank-$rank-hongguo:rank-$rank')),
      );
      expect(rect.top, greaterThanOrEqualTo(0));
      expect(rect.bottom, lessThanOrEqualTo(540));
    }
    await press(LogicalKeyboardKey.arrowUp);
    expect(focusedRank(), '13');
  });
}
