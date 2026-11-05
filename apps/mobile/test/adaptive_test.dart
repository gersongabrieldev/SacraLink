import 'package:flutter_test/flutter_test.dart';
import 'package:sacralink_camera/src/adaptive.dart';

void main() {
  const p = QualityProfile.balanced; // 720p30, 500–2500 kbps

  test('rede boa: perfil completo sem degradação', () {
    final l = adapt(p, const LinkConditions(availableKbps: 20000));
    expect(l.maxBitrateKbps, 2500);
    expect(l.scaleDownBy, 1.0);
    expect(l.maxFps, 30);
    expect(l.degraded, isFalse);
  });

  test('banda limitada reduz o bitrate para 85% do disponível', () {
    final l = adapt(p, const LinkConditions(availableKbps: 1000));
    expect(l.maxBitrateKbps, 850);
    expect(l.reason, contains('banda'));
  });

  test('perda alta reduz mais e respeita o mínimo do perfil', () {
    final l = adapt(p, const LinkConditions(availableKbps: 400, lossPercent: 12));
    expect(l.maxBitrateKbps, 500);
    expect(l.scaleDownBy, 1.5, reason: 'banda muito baixa: menos pixels');
  });

  test('aparelho muito quente reduz resolução e FPS', () {
    final l = adapt(p, const LinkConditions(thermal: 'critical'));
    expect(l.scaleDownBy, 2.0);
    expect(l.maxFps, 15);
    expect(l.reason, contains('quente'));
  });

  test('bateria baixa sem carregar reduz resolução', () {
    final l = adapt(p, const LinkConditions(batteryLevel: 0.1, charging: false));
    expect(l.scaleDownBy, 1.5);
  });

  test('rede móvel limita a 2,5 Mbps mesmo em perfil alto', () {
    const high = QualityProfile(id: 'alta', width: 1920, height: 1080, fps: 30, maxBitrateKbps: 4500, minBitrateKbps: 800);
    expect(adapt(high, const LinkConditions(cellular: true)).maxBitrateKbps, 2500);
  });
}
