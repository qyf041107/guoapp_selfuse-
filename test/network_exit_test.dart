import 'package:duanju_app/core_bridge.dart';
import 'package:duanju_app/xigua/network_exit.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('duanju/device');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  setUp(() {
    NetworkExit.resetForTest();
    NetworkExit.debugSupportedOverride = true;
    messenger.setMockMethodCallHandler(channel, (call) async {
      if (call.method == 'vpnActive') return true;
      if (call.method == 'bindPhysicalNetwork') return true;
      return null;
    });
  });

  tearDown(() {
    NetworkExit.resetForTest();
    messenger.setMockMethodCallHandler(channel, null);
  });

  test('worthBypassing recognizes egress signals but not browser challenges', () {
    expect(
      NetworkExit.worthBypassing(AppFailure('connection reset by peer')),
      isTrue,
    );
    expect(NetworkExit.worthBypassing(AppFailure('HTTP 403')), isTrue);
    expect(NetworkExit.worthBypassing(AppFailure('连接被中断')), isTrue);
    expect(
      NetworkExit.worthBypassing(AppFailure('站源要求浏览器验证')),
      isFalse,
    );
    expect(NetworkExit.worthBypassing(AppFailure('just a moment')), isFalse);
    expect(NetworkExit.worthBypassing(AppFailure('cloudflare')), isFalse);
  });

  test('borrow and release use the platform channel with a cooldown', () async {
    expect(await NetworkExit.borrow(), isTrue);
    await NetworkExit.release();
    expect(await NetworkExit.borrow(), isFalse);
  });
}
