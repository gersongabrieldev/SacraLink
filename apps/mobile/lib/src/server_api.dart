import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'pairing.dart';

/// Credenciais de um servidor pareado (guardadas no Keychain/Keystore).
class ServerCredentials {
  ServerCredentials({
    required this.serverId,
    required this.serverName,
    required this.baseUrls,
    required this.fingerprint,
    required this.deviceId,
    required this.secret,
    required this.cameraId,
    required this.cameraName,
    this.lastBaseUrl,
    this.externalUrl,
  });

  factory ServerCredentials.fromJson(Map<String, dynamic> j) => ServerCredentials(
        serverId: j['serverId'] as String,
        serverName: j['serverName'] as String,
        baseUrls: (j['baseUrls'] as List).cast<String>(),
        fingerprint: j['fingerprint'] as String,
        deviceId: j['deviceId'] as String,
        secret: j['secret'] as String,
        cameraId: j['cameraId'] as String,
        cameraName: j['cameraName'] as String,
        lastBaseUrl: j['lastBaseUrl'] as String?,
        externalUrl: j['externalUrl'] as String?,
      );

  final String serverId;
  final String serverName;
  List<String> baseUrls;
  final String fingerprint;
  final String deviceId;
  final String secret;
  final String cameraId;
  String cameraName;
  String? lastBaseUrl;

  /// URL de acesso externo (4G/5G), se o servidor a informou no QR Code.
  final String? externalUrl;

  String get authorization => 'Bearer $deviceId.$secret';

  Map<String, dynamic> toJson() => {
        'serverId': serverId,
        'serverName': serverName,
        'baseUrls': baseUrls,
        'fingerprint': fingerprint,
        'deviceId': deviceId,
        'secret': secret,
        'cameraId': cameraId,
        'cameraName': cameraName,
        'lastBaseUrl': lastBaseUrl,
        'externalUrl': externalUrl,
      };
}

class ApiException implements Exception {
  ApiException(this.status, this.code, this.message);
  final int status;
  final String code;
  final String message;
  @override
  String toString() => message;
}

/// Cria um HttpClient que aceita SOMENTE o certificado cuja impressão digital
/// SHA-256 foi recebida no QR Code (pinning; o certificado é autoassinado).
HttpClient pinnedHttpClient(String fingerprintHex) {
  final ctx = SecurityContext(withTrustedRoots: false);
  final client = HttpClient(context: ctx)
    ..connectionTimeout = const Duration(seconds: 4)
    ..idleTimeout = const Duration(seconds: 15)
    ..userAgent = 'SacraLinkCamera';
  client.badCertificateCallback = (cert, host, port) => sha256.convert(cert.der).toString() == fingerprintHex;
  return client;
}

/// Cliente da Device API do SacraLink.
class ServerApi {
  ServerApi(this.fingerprint) : _client = pinnedHttpClient(fingerprint);

  final String fingerprint;
  final HttpClient _client;

  void close() => _client.close(force: true);

  Future<(int, Map<String, String>, String)> raw(
    String method,
    Uri url, {
    String? body,
    String contentType = 'application/json',
    Map<String, String> headers = const {},
    Duration timeout = const Duration(seconds: 8),
  }) async {
    final req = await _client.openUrl(method, url).timeout(timeout);
    headers.forEach(req.headers.set);
    if (body != null) {
      req.headers.contentType = ContentType.parse(contentType);
      req.add(utf8.encode(body));
    }
    final resp = await req.close().timeout(timeout);
    final text = await resp.transform(utf8.decoder).join().timeout(timeout);
    final h = <String, String>{};
    resp.headers.forEach((k, v) => h[k.toLowerCase()] = v.join(','));
    return (resp.statusCode, h, text);
  }

  Future<Map<String, dynamic>> json(String method, Uri url, {Object? body, String? authorization}) async {
    final (status, _, text) = await raw(method, url,
        body: body == null ? null : jsonEncode(body), headers: {if (authorization != null) 'authorization': authorization});
    final data = text.isEmpty ? <String, dynamic>{} : jsonDecode(text) as Map<String, dynamic>;
    if (status >= 300) {
      final err = (data['error'] as Map<String, dynamic>?) ?? const {};
      throw ApiException(status, err['code'] as String? ?? 'error', err['message'] as String? ?? 'Erro HTTP $status');
    }
    return data;
  }

  /// Descobre qual endereço responde (rede local primeiro, depois externo).
  Future<String> reachable(List<String> baseUrls, {String? preferred}) async {
    final ordered = [if (preferred != null && baseUrls.contains(preferred)) preferred, ...baseUrls.where((u) => u != preferred)];
    for (final base in ordered) {
      try {
        final info = await json('GET', Uri.parse('$base/device/v1/info'));
        if (info['product'] == 'SacraLink') return base;
      } catch (_) {
        // tenta o próximo
      }
    }
    throw ApiException(0, 'unreachable',
        'Servidor SacraLink não encontrado. Verifique se o celular está no mesmo Wi-Fi do computador da igreja e se o SacraLink está aberto.');
  }

  /// Pareia este aparelho usando o token do QR Code.
  Future<ServerCredentials> pair(PairingInfo info, Map<String, String> device) async {
    if (info.expired) throw ApiException(0, 'expired', 'O QR Code expirou. Gere outro no computador.');
    final base = await reachable(info.baseUrls);
    final r = await json('POST', Uri.parse('$base/device/v1/pair'), body: {'token': info.token, 'device': device});
    return ServerCredentials(
      serverId: r['serverId'] as String? ?? info.serverId,
      serverName: r['serverName'] as String? ?? info.serverName,
      baseUrls: info.baseUrls,
      fingerprint: info.fingerprint,
      deviceId: r['deviceId'] as String,
      secret: r['secret'] as String,
      cameraId: r['cameraId'] as String,
      cameraName: r['cameraName'] as String,
      lastBaseUrl: base,
      externalUrl: info.externalHost.isEmpty ? null : 'https://${info.externalHost}',
    );
  }

  /// Teste de alcance externo: deve ser feito com o Wi-Fi desligado.
  Future<String> reachability(String base, ServerCredentials c, String network) async {
    final r = await json('POST', Uri.parse('$base/device/v1/reachability'), body: {'network': network}, authorization: c.authorization);
    return r['seenIp'] as String? ?? '';
  }
}

/// Armazenamento seguro das credenciais pareadas.
class CredentialStore {
  static const _key = 'sacralink.servers.v1';
  final _storage = const FlutterSecureStorage();

  Future<List<ServerCredentials>> list() async {
    final raw = await _storage.read(key: _key);
    if (raw == null || raw.isEmpty) return [];
    return (jsonDecode(raw) as List).map((e) => ServerCredentials.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> save(ServerCredentials c) async {
    final all = await list();
    all.removeWhere((e) => e.serverId == c.serverId);
    all.insert(0, c);
    await _storage.write(key: _key, value: jsonEncode(all.map((e) => e.toJson()).toList()));
  }

  Future<void> remove(String serverId) async {
    final all = await list()..removeWhere((e) => e.serverId == serverId);
    await _storage.write(key: _key, value: jsonEncode(all.map((e) => e.toJson()).toList()));
  }
}
