// Política de conflito entre o operador do celular e o operador central.
//
// Regra (documentada em docs/mobile/controle-remoto.md):
//  * Os controles locais SEMPRE funcionam.
//  * Quando o operador do celular altera um parâmetro, ele ganha prioridade
//    sobre esse parâmetro por 3 segundos. Comandos remotos para o mesmo
//    parâmetro nesse intervalo são recusados com "conflict" (o painel avisa),
//    exceto se o operador central confirmar ("force").
//  * Comandos repetidos (mesmo commandId) devolvem o resultado já calculado
//    (idempotência); comandos de outra sessão ou expirados são recusados.

class LocalPriority {
  LocalPriority({this.window = const Duration(seconds: 3), DateTime Function()? clock}) : _now = clock ?? DateTime.now;

  final Duration window;
  final DateTime Function() _now;
  final Map<String, DateTime> _touched = {};

  /// Registra uma alteração local do parâmetro.
  void touch(String param) => _touched[param] = _now();

  /// Informa se o operador local tem prioridade sobre o parâmetro agora.
  bool locked(String param) {
    final t = _touched[param];
    return t != null && _now().difference(t) < window;
  }
}

/// Parâmetro afetado por cada comando (para a política de conflito).
String paramOf(String command) {
  switch (command) {
    case 'zoom.set':
    case 'zoom.step':
      return 'zoom';
    case 'lens.select':
    case 'camera.flip':
      return 'camera';
    case 'torch.set':
      return 'torch';
    case 'audio.mute':
      return 'mic';
    case 'quality.set':
    case 'preset.apply':
      return 'quality';
    case 'focus.point':
    case 'exposure.point':
      return 'focus';
  }
  return command;
}

/// Cache de resultados por commandId (idempotência).
class ResultCache {
  ResultCache({this.capacity = 64});
  final int capacity;
  final _map = <String, Map<String, dynamic>>{};

  Map<String, dynamic>? get(String id) => _map[id];

  void put(String id, Map<String, dynamic> result) {
    _map.remove(id);
    _map[id] = result;
    while (_map.length > capacity) {
      _map.remove(_map.keys.first);
    }
  }
}

/// Expiração robusta a diferença de relógio entre celular e servidor: usa a
/// validade relativa (expiresAt - issuedAt) a partir do recebimento.
bool commandExpired({required int issuedAt, required int expiresAt, required DateTime receivedAt, DateTime? now}) {
  final ttl = Duration(milliseconds: (expiresAt - issuedAt).clamp(0, 60000));
  return (now ?? DateTime.now()).isAfter(receivedAt.add(ttl));
}
