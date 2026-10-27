import 'package:duanju_app/app_layout.dart';
import 'package:duanju_app/remote_widgets.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('television remote leaves text field and reopens keyboard', (
    tester,
  ) async {
    final above = FocusNode();
    final field = FocusNode();
    final below = FocusNode();
    addTearDown(above.dispose);
    addTearDown(field.dispose);
    addTearDown(below.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: AppLayout(
          television: true,
          child: Scaffold(
            body: Column(
              children: [
                TextButton(
                  focusNode: above,
                  onPressed: () {},
                  child: const Text('上'),
                ),
                RemoteTextInput(child: TextField(focusNode: field)),
                TextButton(
                  focusNode: below,
                  onPressed: () {},
                  child: const Text('下'),
                ),
              ],
            ),
          ),
        ),
      ),
    );

    Future<void> press(LogicalKeyboardKey key) async {
      await tester.sendKeyEvent(key);
      await tester.pumpAndSettle();
    }

    field.requestFocus();
    await tester.pumpAndSettle();
    await press(LogicalKeyboardKey.arrowDown);
    expect(below.hasPrimaryFocus, isTrue);
    await press(LogicalKeyboardKey.arrowUp);
    expect(field.hasPrimaryFocus, isTrue);
    await press(LogicalKeyboardKey.arrowUp);
    expect(above.hasPrimaryFocus, isTrue);
    await press(LogicalKeyboardKey.arrowDown);
    expect(field.hasPrimaryFocus, isTrue);

    tester.testTextInput.hide();
    expect(tester.testTextInput.isVisible, isFalse);
    await press(LogicalKeyboardKey.select);
    expect(field.hasPrimaryFocus, isTrue);
    expect(tester.testTextInput.isVisible, isTrue);

    await press(LogicalKeyboardKey.escape);
    expect(below.hasPrimaryFocus, isTrue);
    expect(tester.testTextInput.isVisible, isFalse);
  });
}
