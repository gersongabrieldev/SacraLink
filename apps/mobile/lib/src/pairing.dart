// Leitura do QR Code de pareamento do SacraLink.
//
// Formato: sacralink://pair?v=1&id=..&n=..&h=ip1,ip2&p=7443&x=host:port&fp=..&t=..&exp=..
// O QR nunca contém senha do administrador ou credencial reutilizável: o token
// `t` é de uso único e expira em 5 minutos.

class PairingInfo {
  PairingInfo({
    required this.serverId,
    required this.serverName,
    required this.lanHosts,
    required this.port,
    required this.externalHost,
    required this.fingerprint,
    required this.token,
    required this.expiresAt,
  });

  final String serverId;
  final String serverName;
  final List<String> lanHosts;
  final int port;

  /// "host:porta" para acesso externo (4G/5G) ou vazio se desabilitado.
  final String externalHost;

  /// SHA-256 (hex minúsculo) do certificado TLS do servidor (pinning).
  final String fingerprint;
  final String token;
  final DateTime expiresAt;

  bool get expired => DateTime.now().isAfter(expiresAt);

  /// URLs base candidatas, na ordem de tentativa (rede local primeiro).
  List<String> get baseUrls => [
        for (final h in lanHosts) 'https://${_hostForUrl(h)}:$port',
        if (externalHost.isNotEmpty) 'https://$externalHost',
      ];
}

String _hostForUrl(String h) => h.contains(':') && !h.startsWith('[') ? '[$h]' : h;

class PairingFormatException implements Exception {
  PairingFormatException(this.message);
  final String message;
  @override
  String toString() => message;
}

final _hex64 = RegExp(r'^[0-9a-f]{64}$');

/// Interpreta e valida o conteúdo do QR Code.
PairingInfo parsePairingUri(String raw) {
  final uri = Uri.tryParse(raw.trim());
  if (uri == null || uri.scheme != 'sacralink' || uri.host != 'pair') {
    throw PairingFormatException('Este QR Code não é de pareamento do SacraLink.');
  }
  final q = uri.queryParameters;
  if (q['v'] != '1') {
    throw PairingFormatException('Versão do QR Code incompatível. Atualize o aplicativo.');
  }
  final fp = (q['fp'] ?? '').toLowerCase();
  if (!_hex64.hasMatch(fp)) throw PairingFormatException('QR Code sem a identidade do servidor.');
  final token = q['t'] ?? '';
  if (token.length < 20) throw PairingFormatException('QR Code sem token de pareamento.');
  final port = int.tryParse(q['p'] ?? '');
  if (port == null || port <= 0 || port > 65535) throw PairingFormatException('Porta inválida no QR Code.');
  final hosts = (q['h'] ?? '').split(',').map((e) => e.trim()).where((e) => e.isNotEmpty).toList();
  final ext = q['x'] ?? '';
  if (hosts.isEmpty && ext.isEmpty) throw PairingFormatException('QR Code sem endereço do servidor.');
  final exp = int.tryParse(q['exp'] ?? '') ?? 0;
  return PairingInfo(
    serverId: q['id'] ?? '',
    serverName: q['n'] ?? 'SacraLink',
    lanHosts: hosts,
    port: port,
    externalHost: ext,
    fingerprint: fp,
    token: token,
    expiresAt: DateTime.fromMillisecondsSinceEpoch(exp * 1000),
  );
}
