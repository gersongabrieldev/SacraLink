import 'package:flutter_test/flutter_test.dart';
import 'package:sacralink_camera/src/conflict.dart';

void main() {
  test('alteração local dá prioridade por 3 s ao operador do celular', () {
    var now = DateTime(2026, 11, 5, 10);
    final p = LocalPriority(clock: () => now);
    expect(p.locked('zoom'), isFalse);
    p.touch('zoom');
    expect(p.locked('zoom'), isTrue);
    expect(p.locked('torch'), isFalse, reason: 'outros parâmetros não são afetados');
    now = now.add(const Duration(seconds: 3));
    expect(p.locked('zoom'), isFalse);
  });

  test('comandos mapeiam para o parâmetro afetado', () {
    expect(paramOf('zoom.set'), 'zoom');
    expect(paramOf('zoom.step'), 'zoom');
    expect(paramOf('camera.flip'), 'camera');
    expect(paramOf('lens.select'), 'camera');
    expect(paramOf('quality.set'), 'quality');
  });

  test('cache de resultados é idempotente e limitado', () {
    final c = ResultCache(capacity: 2);
    c.put('a', {'status': 'executed'});
    c.put('b', {'status': 'executed'});
    c.put('c', {'status': 'failed'});
    expect(c.get('a'), isNull);
    expect(c.get('c')!['status'], 'failed');
  });

  test('expiração usa validade relativa (robusta a relógio dessincronizado)', () {
    final received = DateTime(2026, 11, 5, 10);
    // Relógio do servidor 1 h adiantado: issuedAt/expiresAt "no futuro".
    const issued = 1793906400000 + 3600000;
    const expires = issued + 5000;
    expect(commandExpired(issuedAt: issued, expiresAt: expires, receivedAt: received, now: received.add(const Duration(seconds: 4))), isFalse);
    expect(commandExpired(issuedAt: issued, expiresAt: expires, receivedAt: received, now: received.add(const Duration(seconds: 6))), isTrue);
  });
}
