import 'package:flutter_test/flutter_test.dart';
import 'package:sacralink_camera/src/session.dart';

void main() {
  test('aceita somente as orientações do painel', () {
    expect(orientationOf('landscape'), 'landscape');
    expect(orientationOf('portrait'), 'portrait');
    expect(orientationOf('diagonal'), isNull);
    expect(orientationOf(null), isNull);
    expect(orientationOf(1), isNull);
  });
}
