import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'server_api.dart';

/// Canal de controle v1 (contrato em packages/contracts/control-v1.md):
/// WebSocket autenticado sobre TLS com pinning, heartbeat e reconexão.
class ControlChannel {
  ControlChannel(this.creds, this.baseUrl, {required this.onMessage, required this.onState});

  final ServerCredentials creds;
  final String baseUrl;
  final void Function(String type, Map<String, dynamic> data) onMessage;
  final void Function(bool connected, String? error) onState;

  /// Construtor do hello (capacidades atualizadas a cada conexão).
  Map<String, dynamic> Function()? hello;

  WebSocket? _ws;
  Timer? _ping;
  Timer? _retry;
  bool _closed = false;
  int _backoffMs = 1000;

  bool get connected => _ws != null;

  Future<void> connect() async {
    _closed = false;
    _retry?.cancel();
    try {
      final url = '${baseUrl.replaceFirst('https', 'wss')}/device/v1/ws';
      final ws = await WebSocket.connect(url,
              protocols: ['sacralink.control.v1'],
              headers: {'authorization': creds.authorization},
              customClient: pinnedHttpClient(creds.fingerprint))
          .timeout(const Duration(seconds: 8));
      _ws = ws;
      _backoffMs = 1000;
      send('hello', hello?.call() ?? const {});
      onState(true, null);
      _ping = Timer.periodic(const Duration(seconds: 5), (_) => send('ping', const {}));
      ws.listen(_onData, onDone: () => _onClosed(ws.closeReason), onError: (Object e) => _onClosed('$e'), cancelOnError: true);
    } on WebSocketException catch (e) {
      _onClosed(e.message.contains('401') ? 'credencial recusada pelo servidor' : e.message);
    } catch (e) {
      _onClosed('$e');
    }
  }

  void _onData(dynamic raw) {
    if (raw is! String) return;
    try {
      final env = jsonDecode(raw) as Map<String, dynamic>;
      if (env['v'] != 1) return;
      onMessage(env['type'] as String, (env['data'] as Map<String, dynamic>?) ?? const {});
    } catch (_) {
      // mensagem inválida: ignora
    }
  }

  void _onClosed(String? reason) {
    _ping?.cancel();
    _ws = null;
    onState(false, reason);
    if (_closed) return;
    _retry = Timer(Duration(milliseconds: _backoffMs), connect);
    _backoffMs = (_backoffMs * 2).clamp(1000, 10000);
  }

  void send(String type, Map<String, dynamic> data) {
    final ws = _ws;
    if (ws == null) return;
    ws.add(jsonEncode({'v': 1, 'type': type, 'ts': DateTime.now().millisecondsSinceEpoch, 'data': data}));
  }

  /// Reconecta imediatamente (ex.: troca de rede Wi-Fi → 5G).
  void reconnectNow() {
    _ws?.close();
  }

  Future<void> close() async {
    _closed = true;
    _retry?.cancel();
    _ping?.cancel();
    await _ws?.close(WebSocketStatus.normalClosure);
    _ws = null;
  }
}
