import 'dart:io';

import 'package:device_info_plus/device_info_plus.dart';
import 'package:flutter/material.dart';
import 'package:package_info_plus/package_info_plus.dart';

import '../discovery.dart';
import '../pairing.dart';
import '../server_api.dart';
import 'camera_screen.dart';
import 'scan_screen.dart';
import 'theme.dart';

/// Tela inicial: servidores pareados, leitura do QR Code e conexão manual.
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  final _store = CredentialStore();
  List<ServerCredentials> _servers = [];
  bool _busy = false;
  String? _status;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final s = await _store.list();
    if (mounted) setState(() => _servers = s);
  }

  Future<Map<String, String>> _deviceInfo() async {
    final pkg = await PackageInfo.fromPlatform();
    final info = DeviceInfoPlugin();
    if (Platform.isAndroid) {
      final a = await info.androidInfo;
      return {'name': '${a.manufacturer} ${a.model}', 'platform': 'android', 'model': a.model, 'osVersion': 'Android ${a.version.release}', 'appVersion': pkg.version};
    }
    final i = await info.iosInfo;
    return {'name': i.name, 'platform': 'ios', 'model': i.utsname.machine, 'osVersion': 'iOS ${i.systemVersion}', 'appVersion': pkg.version};
  }

  Future<void> _pairWith(String raw) async {
    setState(() {
      _busy = true;
      _status = 'Conectando ao servidor…';
    });
    try {
      final info = parsePairingUri(raw);
      final api = ServerApi(info.fingerprint);
      try {
        final creds = await api.pair(info, await _deviceInfo());
        await _store.save(creds);
        await _load();
        if (mounted) await _open(creds);
      } finally {
        api.close();
      }
    } catch (e) {
      if (mounted) setState(() => _status = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _scan() async {
    // A permissão da câmera é pedida pelo próprio leitor (mobile_scanner).
    final raw = await Navigator.of(context).push<String>(MaterialPageRoute(builder: (_) => const ScanScreen()));
    if (raw != null) await _pairWith(raw);
  }

  Future<void> _manual() async {
    final ctrl = TextEditingController();
    final raw = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Conexão manual'),
        content: TextField(
          controller: ctrl,
          maxLines: 4,
          decoration: const InputDecoration(hintText: 'Cole o código sacralink://pair?… exibido no painel'),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancelar')),
          FilledButton(onPressed: () => Navigator.pop(ctx, ctrl.text), child: const Text('Conectar')),
        ],
      ),
    );
    if (raw != null && raw.trim().isNotEmpty) await _pairWith(raw);
  }

  Future<void> _open(ServerCredentials c) async {
    // Câmera e microfone são pedidos pelo flutter_webrtc ao abrir a câmera.
    await Navigator.of(context).push(MaterialPageRoute(builder: (_) => CameraScreen(creds: c)));
    await _load();
  }

  /// Procura servidores SacraLink na rede local. Servidores já pareados
  /// podem ser abertos (atualizando o IP); os demais exigem o QR Code.
  Future<void> _locate() async {
    setState(() {
      _busy = true;
      _status = 'Procurando servidores na rede local…';
    });
    final found = await discoverServers();
    if (!mounted) return;
    setState(() {
      _busy = false;
      _status = found.isEmpty ? 'Nenhum servidor encontrado. Confira se o celular está no mesmo Wi-Fi do computador da igreja.' : null;
    });
    if (found.isEmpty) return;
    final paired = {for (final s in _servers) s.serverId: s};
    await showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: ListView(shrinkWrap: true, padding: const EdgeInsets.all(16), children: [
          const Text('Servidores na rede', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
          for (final d in found)
            ListTile(
              leading: const Icon(Icons.church_outlined, color: SacraColors.accent),
              title: Text(d.name),
              subtitle: Text(paired.containsKey(d.serverId)
                  ? 'Pareado como "${paired[d.serverId]!.cameraName}" · ${d.hosts.first}'
                  : 'Não pareado: leia o QR Code no painel'),
              enabled: paired.containsKey(d.serverId),
              onTap: () async {
                final c = paired[d.serverId]!;
                c.baseUrls = [...d.baseUrls, ...c.baseUrls.where((u) => !d.baseUrls.contains(u))];
                await _store.save(c);
                if (ctx.mounted) Navigator.pop(ctx);
                if (mounted) await _open(c);
              },
            ),
        ]),
      ),
    );
  }

  Future<void> _forget(ServerCredentials c) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Esquecer servidor?'),
        content: Text('Este celular deixará de ser a "${c.cameraName}" de ${c.serverName}. Para voltar será preciso parear novamente.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancelar')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Esquecer')),
        ],
      ),
    );
    if (ok == true) {
      await _store.remove(c.serverId);
      await _load();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            const SizedBox(height: 12),
            Row(children: [
              Image.asset('assets/logo.png', width: 52, height: 52),
              const SizedBox(width: 12),
              const Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('SacraLink Camera', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                Text('Câmera sem fio para transmissões', style: TextStyle(color: SacraColors.muted)),
              ]),
            ]),
            const SizedBox(height: 28),
            if (_servers.isNotEmpty) ...[
              const Text('SERVIDORES PAREADOS', style: TextStyle(color: SacraColors.muted, fontSize: 12, fontWeight: FontWeight.w600)),
              const SizedBox(height: 8),
              for (final s in _servers)
                Card(
                  child: ListTile(
                    leading: const Icon(Icons.church_outlined, color: SacraColors.accent),
                    title: Text(s.serverName),
                    subtitle: Text(s.cameraName),
                    trailing: IconButton(icon: const Icon(Icons.delete_outline), tooltip: 'Esquecer', onPressed: () => _forget(s)),
                    onTap: _busy ? null : () => _open(s),
                  ),
                ),
              const SizedBox(height: 24),
            ],
            FilledButton.icon(
              onPressed: _busy ? null : _scan,
              icon: const Icon(Icons.qr_code_scanner),
              label: const Text('Ler QR Code do servidor'),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              onPressed: _busy ? null : _locate,
              style: OutlinedButton.styleFrom(minimumSize: const Size(48, 52)),
              icon: const Icon(Icons.wifi_find),
              label: const Text('Localizar servidor na rede'),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              onPressed: _busy ? null : _manual,
              style: OutlinedButton.styleFrom(minimumSize: const Size(48, 52)),
              icon: const Icon(Icons.keyboard),
              label: const Text('Conexão manual'),
            ),
            const SizedBox(height: 20),
            if (_busy) const LinearProgressIndicator(),
            if (_status != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_status!, style: const TextStyle(color: SacraColors.warn))),
            const SizedBox(height: 28),
            const Text(
              'No computador da igreja, abra o SacraLink → Câmeras e aparelhos → Parear celular. '
              'O celular precisa estar no mesmo Wi-Fi (ou com acesso externo configurado pelo administrador).',
              style: TextStyle(color: SacraColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}
