import 'dart:async';

import 'package:bonsoir/bonsoir.dart';

/// Servidor SacraLink anunciado na rede local (mDNS/DNS-SD).
///
/// A descoberta NÃO pareia: ela só ajuda celulares já pareados a reencontrar
/// o computador da igreja quando o IP muda. A identidade é sempre confirmada
/// pelo pinning do certificado (um anúncio mDNS pode ser falsificado).
class DiscoveredServer {
  DiscoveredServer({required this.name, required this.serverId, required this.hosts, required this.port});

  final String name;
  final String serverId;
  final List<String> hosts;
  final int port;

  List<String> get baseUrls => [for (final h in hosts) 'https://${h.contains(':') ? '[$h]' : h}:$port'];
}

const serviceType = '_sacralink._tcp';

/// Procura servidores por [timeout] e retorna os encontrados.
Future<List<DiscoveredServer>> discoverServers({Duration timeout = const Duration(seconds: 4)}) async {
  final found = <String, DiscoveredServer>{};
  final discovery = BonsoirDiscovery(type: serviceType);
  StreamSubscription<BonsoirDiscoveryEvent>? sub;
  try {
    await discovery.initialize();
    sub = discovery.eventStream?.listen((event) {
      switch (event) {
        case BonsoirDiscoveryServiceFoundEvent():
          event.service.resolve(discovery.serviceResolver);
        case BonsoirDiscoveryServiceResolvedEvent():
          final s = event.service;
          final id = s.attributes['id'] ?? '';
          if (id.isEmpty || s.attributes['v'] != '1' || s.hostAddresses.isEmpty) return;
          final name = s.name.startsWith('SacraLink ') ? s.name.substring(10) : s.name;
          found[id] = DiscoveredServer(name: name, serverId: id, hosts: s.hostAddresses, port: s.port);
        default:
      }
    });
    await discovery.start();
    await Future<void>.delayed(timeout);
  } catch (_) {
    // Rede sem multicast ou permissão negada: descoberta indisponível.
  } finally {
    await sub?.cancel();
    try {
      await discovery.stop();
    } catch (_) {}
  }
  return found.values.toList();
}
