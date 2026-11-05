import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../adaptive.dart';
import '../server_api.dart';
import '../session.dart';
import 'theme.dart';

/// Tela da câmera: vídeo em tela cheia, tally do OBS e controles.
class CameraScreen extends StatefulWidget {
  const CameraScreen({super.key, required this.creds});
  final ServerCredentials creds;

  @override
  State<CameraScreen> createState() => _CameraScreenState();
}

class _CameraScreenState extends State<CameraScreen> {
  late final CameraSession s = CameraSession(widget.creds);
  double _pinchStart = 1;
  Offset? _focus;
  Timer? _clock;

  static const _profiles = [
    QualityProfile(id: 'economico', width: 854, height: 480, fps: 30, maxBitrateKbps: 1000, minBitrateKbps: 250),
    QualityProfile(id: 'equilibrado', width: 1280, height: 720, fps: 30, maxBitrateKbps: 2500, minBitrateKbps: 500),
    QualityProfile(id: 'alta', width: 1920, height: 1080, fps: 30, maxBitrateKbps: 4500, minBitrateKbps: 800),
    QualityProfile(id: 'alta60', width: 1920, height: 1080, fps: 60, maxBitrateKbps: 6500, minBitrateKbps: 1200),
  ];

  @override
  void initState() {
    super.initState();
    SystemChrome.setPreferredOrientations([DeviceOrientation.landscapeLeft, DeviceOrientation.landscapeRight]);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    s.addListener(_refresh);
    s.open().catchError((Object e) {
      if (mounted) _snack('Não foi possível abrir a câmera: $e');
    });
    _clock = Timer.periodic(const Duration(seconds: 1), (_) => _refresh());
  }

  void _refresh() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _clock?.cancel();
    s.removeListener(_refresh);
    s.dispose();
    SystemChrome.setPreferredOrientations(DeviceOrientation.values);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    super.dispose();
  }

  void _snack(String m) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));

  Future<void> _guard(Future<void> Function() f) async {
    try {
      await f();
    } catch (e) {
      _snack('$e');
    }
  }

  // Tally ---------------------------------------------------------------------

  Widget _tallyBanner() {
    final (Color color, String text) = switch (s.tally) {
      TallyState.onAir => (SacraColors.onAir, s.obsLive ? '● NO AR' : '● NO AR (transmissão parada)'),
      TallyState.preview => (SacraColors.preview, 'PREVIEW'),
      TallyState.connected => (Colors.black54, 'CONECTADA'),
      TallyState.unknown => (Colors.black54, s.controlConnected ? 'OBS SEM CONEXÃO' : 'SERVIDOR DESCONECTADO'),
    };
    return AnimatedContainer(
      duration: const Duration(milliseconds: 150),
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
      decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(8)),
      child: Text(text, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, letterSpacing: 0.5)),
    );
  }

  String _duration() {
    final since = s.liveSince;
    if (since == null) return '00:00';
    final d = DateTime.now().difference(since);
    String two(int n) => n.toString().padLeft(2, '0');
    return d.inHours > 0 ? '${d.inHours}:${two(d.inMinutes % 60)}:${two(d.inSeconds % 60)}' : '${two(d.inMinutes)}:${two(d.inSeconds % 60)}';
  }

  Widget _chip(IconData icon, String text, {Color? color}) => Container(
        margin: const EdgeInsets.only(right: 6),
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(color: Colors.black54, borderRadius: BorderRadius.circular(6)),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Icon(icon, size: 14, color: color),
          const SizedBox(width: 4),
          Text(text, style: TextStyle(fontSize: 12, color: color)),
        ]),
      );

  @override
  Widget build(BuildContext context) {
    final live = s.streamState == StreamState.live;
    final border = switch (s.tally) {
      TallyState.onAir => SacraColors.onAir,
      TallyState.preview => SacraColors.preview,
      _ => Colors.transparent,
    };
    final quality = s.networkQuality;
    final qColor = quality == 'good' ? SacraColors.preview : quality == 'fair' ? SacraColors.warn : quality == 'poor' ? SacraColors.onAir : SacraColors.muted;

    return Scaffold(
      backgroundColor: Colors.black,
      body: Container(
        decoration: BoxDecoration(border: Border.all(color: border, width: 6)),
        child: Stack(children: [
          // Vídeo + gestos (pinça para zoom, toque para foco).
          Positioned.fill(
            child: GestureDetector(
              onScaleStart: (_) => _pinchStart = s.zoom,
              onScaleUpdate: (d) {
                if (d.pointerCount >= 2) s.setZoom(_pinchStart * d.scale);
              },
              onTapUp: (d) {
                final size = context.size ?? const Size(1, 1);
                setState(() => _focus = d.localPosition);
                s.focusAt(d.localPosition.dx / size.width, d.localPosition.dy / size.height).catchError((_) {});
                Future.delayed(const Duration(milliseconds: 900), () => mounted ? setState(() => _focus = null) : null);
              },
              child: RTCVideoView(s.renderer, objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain),
            ),
          ),
          if (_focus != null)
            Positioned(
              left: _focus!.dx - 30,
              top: _focus!.dy - 30,
              child: Container(width: 60, height: 60, decoration: BoxDecoration(border: Border.all(color: SacraColors.accent, width: 2))),
            ),

          // Barra superior: tally, nome, estado e indicadores.
          Positioned(
            left: 12,
            right: 12,
            top: 10,
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              IconButton.filledTonal(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.arrow_back)),
              const SizedBox(width: 8),
              _tallyBanner(),
              const SizedBox(width: 10),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(s.cameraName, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16, shadows: [Shadow(blurRadius: 6)])),
                  Text(s.serverName, style: const TextStyle(color: Colors.white70, fontSize: 12, shadows: [Shadow(blurRadius: 6)])),
                ]),
              ),
              Wrap(children: [
                if (s.batteryLevel != null)
                  _chip(s.charging ? Icons.battery_charging_full : Icons.battery_std, '${(s.batteryLevel! * 100).round()}%',
                      color: s.batteryLevel! < 0.15 && !s.charging ? SacraColors.onAir : null),
                if (s.thermal == 'serious' || s.thermal == 'critical') _chip(Icons.thermostat, 'quente', color: SacraColors.onAir),
                _chip(s.network == 'cellular' ? Icons.signal_cellular_alt : Icons.wifi, quality == 'unknown' ? s.network : quality, color: qColor),
                if (live) _chip(Icons.timer_outlined, _duration()),
                if (live) _chip(Icons.speed, '${s.stats.bitrateKbps.round()} kbps'),
                if (live && s.stats.lossPercent > 0.5) _chip(Icons.warning_amber, 'perda ${s.stats.lossPercent.toStringAsFixed(1)}%', color: SacraColors.warn),
                if (live && s.stats.rttMs > 0) _chip(Icons.av_timer, '${s.stats.rttMs.round()} ms'),
              ]),
            ]),
          ),

          // Mensagens de estado (erros, degradação, ação remota).
          Positioned(
            left: 0,
            right: 0,
            top: 64,
            child: Column(children: [
              if (s.streamState == StreamState.reconnecting) _notice('Reconectando…', SacraColors.warn),
              if (s.error != null) _notice(s.error!, SacraColors.onAir),
              if (s.limits?.degraded ?? false) _notice('Qualidade reduzida: ${s.limits!.reason}', SacraColors.warn),
              if (s.lastRemoteAction != null) _notice(s.lastRemoteAction!, Colors.black54),
            ]),
          ),

          // Zoom (controle deslizante + botões) à direita.
          Positioned(
            right: 12,
            top: 70,
            bottom: 90,
            child: Column(children: [
              IconButton.filledTonal(onPressed: () => s.setZoom(s.zoom + 0.5), icon: const Icon(Icons.zoom_in)),
              Expanded(
                child: RotatedBox(
                  quarterTurns: 3,
                  child: Slider(
                    min: s.minZoom,
                    max: s.maxZoom > s.minZoom ? s.maxZoom : s.minZoom + 0.01,
                    value: s.zoom.clamp(s.minZoom, s.maxZoom > s.minZoom ? s.maxZoom : s.minZoom + 0.01).toDouble(),
                    onChanged: (v) => s.setZoom(v),
                  ),
                ),
              ),
              Text('${s.zoom.toStringAsFixed(1)}×', style: const TextStyle(fontWeight: FontWeight.w700, shadows: [Shadow(blurRadius: 6)])),
              IconButton.filledTonal(onPressed: () => s.setZoom(s.zoom - 0.5), icon: const Icon(Icons.zoom_out)),
            ]),
          ),

          // Lentes reais (somente as informadas pelo aparelho).
          if (s.lenses.where((l) => l.facing == 'back').length > 1)
            Positioned(
              right: 70,
              bottom: 90,
              child: Row(children: [
                for (final l in s.lenses.where((l) => l.facing == 'back'))
                  Padding(
                    padding: const EdgeInsets.only(left: 6),
                    child: ChoiceChip(
                      label: Text(l.kind == 'ultra_wide' ? '0,5×' : l.kind == 'telephoto' ? 'Tele' : '1×'),
                      tooltip: l.label,
                      selected: s.lensId == l.id,
                      onSelected: (_) => _guard(() => s.selectLens(l.id)),
                    ),
                  ),
              ]),
            ),

          // Barra inferior: transmitir e controles.
          Positioned(
            left: 12,
            right: 12,
            bottom: 12,
            child: Row(children: [
              FilledButton.icon(
                style: FilledButton.styleFrom(backgroundColor: live || s.streamState != StreamState.idle ? SacraColors.onAir : SacraColors.accent),
                onPressed: () => _guard(() => s.streamState == StreamState.idle ? s.startStream() : s.stopStream()),
                icon: Icon(s.streamState == StreamState.idle ? Icons.play_arrow : Icons.stop),
                label: Text(s.streamState == StreamState.idle
                    ? 'Iniciar transmissão'
                    : s.streamState == StreamState.connecting
                        ? 'Conectando…'
                        : 'Encerrar'),
              ),
              const Spacer(),
              _round(Icons.cameraswitch, 'Frontal/traseira', () => _guard(s.flipCamera)),
              _round(s.torch ? Icons.flashlight_off : Icons.flashlight_on, 'Lanterna', () => _guard(() => s.setTorch(!s.torch))),
              _round(s.micMuted ? Icons.mic_off : Icons.mic, s.micMuted ? 'Ativar microfone' : 'Silenciar microfone', () => s.setMicMuted(!s.micMuted),
                  color: s.micMuted ? SacraColors.onAir : null),
              _round(Icons.tune, 'Qualidade e ajustes', _settings),
            ]),
          ),
        ]),
      ),
    );
  }

  Widget _notice(String text, Color color) => Container(
        margin: const EdgeInsets.only(bottom: 6),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        decoration: BoxDecoration(color: color.withValues(alpha: 0.85), borderRadius: BorderRadius.circular(8)),
        child: Text(text, style: const TextStyle(fontWeight: FontWeight.w600)),
      );

  Widget _round(IconData icon, String tip, VoidCallback onTap, {Color? color}) => Padding(
        padding: const EdgeInsets.only(left: 8),
        child: IconButton.filledTonal(onPressed: onTap, tooltip: tip, icon: Icon(icon, color: color)),
      );

  Future<void> _settings() async {
    await showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: ListView(shrinkWrap: true, padding: const EdgeInsets.all(16), children: [
          const Text('Qualidade', style: TextStyle(fontWeight: FontWeight.w700)),
          for (final p in _profiles)
            ListTile(
              leading: Icon(s.profile.id == p.id ? Icons.radio_button_checked : Icons.radio_button_unchecked, color: SacraColors.accent),
              title: Text('${p.height}p ${p.fps} FPS'),
              subtitle: Text('até ${(p.maxBitrateKbps / 1000).toStringAsFixed(1)} Mbps'),
              onTap: () {
                Navigator.pop(ctx);
                _guard(() => s.setProfile(p));
              },
            ),
          const Divider(),
          ListTile(
            leading: const Icon(Icons.public),
            title: const Text('Testar conexão externa (4G/5G)'),
            subtitle: const Text('Desligue o Wi-Fi antes de testar'),
            onTap: () async {
              Navigator.pop(ctx);
              try {
                final msg = await s.testExternal();
                if (!mounted) return;
                _snack(msg);
              } catch (e) {
                if (!mounted) return;
                _snack('Falhou: $e');
              }
            },
          ),
          const ListTile(
            leading: Icon(Icons.info_outline),
            title: Text('Mantenha o app aberto e a tela ligada'),
            subtitle: Text('Android e iOS suspendem a câmera em segundo plano ou com a tela bloqueada.'),
          ),
        ]),
      ),
    );
  }
}
