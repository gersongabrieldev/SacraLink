import 'dart:async';
import 'dart:io';
import 'dart:math';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:sacralink_device_caps/sacralink_device_caps.dart';
import 'package:wakelock_plus/wakelock_plus.dart';

import 'adaptive.dart';
import 'conflict.dart';
import 'control_channel.dart';
import 'discovery.dart';
import 'publisher.dart';
import 'server_api.dart';

/// Estado da transmissão exibido no celular.
enum StreamState { idle, connecting, live, reconnecting, error }

/// Tally recebido do servidor (derivado do estado REAL do OBS).
enum TallyState { onAir, preview, connected, unknown }

/// Sessão de câmera: captura, controle remoto, publicação e telemetria.
class CameraSession extends ChangeNotifier {
  CameraSession(this.creds) : api = ServerApi(creds.fingerprint);

  final ServerCredentials creds;
  final ServerApi api;
  final _priority = LocalPriority();
  final _results = ResultCache();

  // Estado público ------------------------------------------------------------
  final renderer = RTCVideoRenderer();
  String? baseUrl;
  bool controlConnected = false;
  String? error;
  StreamState streamState = StreamState.idle;
  TallyState tally = TallyState.unknown;
  bool obsLive = false;
  String serverName = '';
  String cameraName = '';
  QualityProfile profile = QualityProfile.balanced;
  SenderLimits? limits;
  SendStats stats = const SendStats();
  List<LensInfo> lenses = [];
  String? lensId;
  double zoom = 1;
  bool torch = false;
  bool micMuted = false;
  double? batteryLevel;
  bool charging = false;
  String thermal = 'unknown';
  String network = 'none';
  DateTime? liveSince;
  String? lastRemoteAction;

  /// Orientação da imagem escolhida no painel: 'landscape' ou 'portrait'.
  /// A tela da câmera trava nessa orientação (a imagem acompanha a tela).
  String orientation = 'landscape';

  // Interno -------------------------------------------------------------------
  MediaStream? _stream;
  ControlChannel? _control;
  WhipPublisher? _publisher;
  String _sessionId = '';
  Timer? _telemetry;
  Timer? _retry;
  int _retryMs = 1000;
  bool _wantLive = false;
  StreamSubscription<List<ConnectivityResult>>? _netSub;
  List<ConnectivityResult> _lastNet = const [];

  LensInfo? get lens => lenses.where((l) => l.id == lensId).firstOrNull;
  double get minZoom => lens?.minZoom ?? 1;
  double get maxZoom => max(lens?.maxZoom ?? 1, 1);

  /// Prepara câmera, renderizador e canal de controle.
  Future<void> open() async {
    cameraName = creds.cameraName;
    serverName = creds.serverName;
    await renderer.initialize();
    lenses = await DeviceCaps.lenses();
    final back = lenses.where((l) => l.facing == 'back').toList();
    lensId = (back.isNotEmpty ? back.first : lenses.firstOrNull)?.id;
    await _openCamera();
    await WakelockPlus.enable(); // a câmera para com a tela bloqueada (restrição do sistema)

    try {
      baseUrl = await _resolveBase();
      creds.lastBaseUrl = baseUrl;
      await CredentialStore().save(creds);
    } on ApiException catch (e) {
      error = e.message;
      notifyListeners();
    }
    final info = await PackageInfo.fromPlatform();
    _control = ControlChannel(creds, baseUrl ?? creds.baseUrls.first, onMessage: _onControl, onState: (ok, err) {
      controlConnected = ok;
      if (!ok && err != null && err.contains('credencial')) error = 'Este celular foi desautorizado no servidor. Pareie novamente.';
      notifyListeners();
    })
      ..hello = () => {
            'appVersion': info.version,
            'platform': Platform.isIOS ? 'ios' : 'android',
            'model': '',
            'osVersion': Platform.operatingSystemVersion,
            'capabilities': {
              'lenses': lenses.map((l) => l.toJson()).toList(),
              'torch': _hasTorch,
              'focusPoint': true,
              'exposurePoint': true,
              'maxWidth': 1920,
              'maxHeight': 1080,
              'maxFps': 60,
              'hardwareH264': true,
              'thermal': thermal != 'unknown',
            },
          };
    unawaited(_control!.connect());
    _telemetry = Timer.periodic(const Duration(seconds: 2), (_) => _tick());
    _netSub = Connectivity().onConnectivityChanged.listen(_onNetwork);
    notifyListeners();
  }

  bool _hasTorch = false;
  bool _disposed = false;

  Future<void> _openCamera() async {
    final old = _stream;
    final constraints = {
      'audio': {'echoCancellation': true, 'noiseSuppression': true, 'autoGainControl': true},
      'video': {
        if (lensId != null) 'deviceId': lensId else 'facingMode': 'environment',
        'width': {'ideal': profile.width},
        'height': {'ideal': profile.height},
        'frameRate': {'ideal': profile.fps},
      },
    };
    final stream = await navigator.mediaDevices.getUserMedia(constraints);
    _stream = stream;
    renderer.srcObject = stream;
    final video = stream.getVideoTracks().first;
    _hasTorch = await video.hasTorch().catchError((_) => false);
    for (final t in stream.getAudioTracks()) {
      t.enabled = !micMuted;
    }
    if (_publisher?.active ?? false) {
      await _publisher!.replaceVideoTrack(video);
    }
    if (old != null && old != stream) {
      for (final t in old.getTracks()) {
        await t.stop();
      }
      await old.dispose();
    }
    zoom = minZoom > 1 ? minZoom : 1;
  }

  /// Encontra o servidor: endereços salvos e, se o IP mudou (DHCP), a
  /// descoberta mDNS pelo mesmo serverId. O pinning confirma a identidade.
  Future<String> _resolveBase() async {
    try {
      return await api.reachable(creds.baseUrls, preferred: creds.lastBaseUrl);
    } on ApiException {
      for (final s in await discoverServers()) {
        if (s.serverId != creds.serverId) continue;
        try {
          final base = await api.reachable(s.baseUrls);
          creds.baseUrls = [...s.baseUrls, ...creds.baseUrls.where((u) => !s.baseUrls.contains(u))];
          creds.lastBaseUrl = base;
          await CredentialStore().save(creds);
          return base;
        } on ApiException {
          // anúncio sem o certificado esperado: ignora
        }
      }
      rethrow;
    }
  }

  // Transmissão ---------------------------------------------------------------

  Future<void> startStream() async {
    _wantLive = true;
    await _publish();
  }

  Future<void> stopStream() async {
    _wantLive = false;
    _retry?.cancel();
    await _publisher?.stop();
    streamState = StreamState.idle;
    liveSince = null;
    notifyListeners();
  }

  Future<void> _publish() async {
    if (!_wantLive || _stream == null) return;
    _retry?.cancel();
    streamState = liveSince == null ? StreamState.connecting : StreamState.reconnecting;
    notifyListeners();
    try {
      baseUrl ??= await _resolveBase();
      _publisher ??= WhipPublisher(api: api, creds: creds, baseUrl: baseUrl!, onConnection: _onPeer);
      await _publisher!.start(_stream!, profile);
      error = null;
    } on ApiException catch (e) {
      error = e.message;
      _scheduleRetry();
    } catch (e) {
      error = 'Falha ao transmitir: $e';
      _scheduleRetry();
    }
    notifyListeners();
  }

  void _onPeer(RTCPeerConnectionState s) {
    switch (s) {
      case RTCPeerConnectionState.RTCPeerConnectionStateConnected:
        streamState = StreamState.live;
        liveSince ??= DateTime.now();
        _retryMs = 1000;
        error = null;
      case RTCPeerConnectionState.RTCPeerConnectionStateFailed:
      case RTCPeerConnectionState.RTCPeerConnectionStateDisconnected:
        if (_wantLive) {
          streamState = StreamState.reconnecting;
          _scheduleRetry();
        }
      default:
    }
    notifyListeners();
  }

  void _scheduleRetry() {
    if (!_wantLive) return;
    streamState = StreamState.reconnecting;
    _retry?.cancel();
    _retry = Timer(Duration(milliseconds: _retryMs), _publish);
    _retryMs = (_retryMs * 2).clamp(1000, 10000);
  }

  /// Troca Wi-Fi ↔ 4G/5G: nova sessão WHIP imediata (o servidor substitui a
  /// anterior pela mesma câmera) e reconexão do controle.
  void _onNetwork(List<ConnectivityResult> r) {
    network = r.contains(ConnectivityResult.wifi)
        ? 'wifi'
        : r.contains(ConnectivityResult.mobile)
            ? 'cellular'
            : r.contains(ConnectivityResult.ethernet)
                ? 'ethernet'
                : 'none';
    final changed = _lastNet.isNotEmpty && !listEquals(_lastNet, r);
    _lastNet = r;
    if (changed && network != 'none') {
      baseUrl = null; // a rota pode ter mudado (LAN ↔ externo)
      _publisher = null;
      _control?.reconnectNow();
      if (_wantLive) {
        _retryMs = 1000;
        unawaited(_publish());
      }
    }
    notifyListeners();
  }

  // Controles locais (sempre funcionam e ganham prioridade por 3 s) ------------

  Future<void> setZoom(double v, {bool local = true}) async {
    final track = _stream?.getVideoTracks().firstOrNull;
    if (track == null) return;
    zoom = v.clamp(minZoom, maxZoom).toDouble();
    await Helper.setZoom(track, zoom);
    if (local) _localChange('zoom', zoom);
    notifyListeners();
  }

  Future<void> selectLens(String id, {bool local = true}) async {
    if (!lenses.any((l) => l.id == id)) throw StateError('lente indisponível');
    lensId = id;
    await _openCamera();
    if (local) _localChange('camera', id);
    notifyListeners();
  }

  Future<void> flipCamera({bool local = true}) async {
    final current = lens?.facing ?? 'back';
    final target = lenses.where((l) => l.facing != current).firstOrNull;
    if (target != null) {
      await selectLens(target.id, local: local);
    } else {
      await _stream?.getVideoTracks().firstOrNull?.switchCamera();
    }
    if (local) _localChange('camera', 'flip');
  }

  Future<void> setTorch(bool on, {bool local = true}) async {
    final track = _stream?.getVideoTracks().firstOrNull;
    if (track == null || !_hasTorch) throw StateError('sem lanterna');
    await track.setTorch(on);
    torch = on;
    if (local) _localChange('torch', on);
    notifyListeners();
  }

  void setMicMuted(bool muted, {bool local = true}) {
    micMuted = muted;
    for (final t in _stream?.getAudioTracks() ?? const <MediaStreamTrack>[]) {
      t.enabled = !muted;
    }
    if (local) _localChange('mic', muted);
    notifyListeners();
  }

  Future<void> focusAt(double x, double y, {bool local = true}) async {
    final track = _stream?.getVideoTracks().firstOrNull;
    if (track == null) return;
    await Helper.setFocusPoint(track, Point(x, y));
    await Helper.setExposurePoint(track, Point(x, y));
    if (local) _localChange('focus', [x, y]);
  }

  Future<void> setProfile(QualityProfile p, {bool local = true}) async {
    profile = p;
    await _openCamera();
    limits = adapt(profile, _conditions());
    await _publisher?.applyLimits(limits!);
    if (local) _localChange('quality', p.id);
    notifyListeners();
  }

  void _localChange(String param, Object value) {
    _priority.touch(param);
    _control?.send('local_change', {'param': param, 'value': value});
  }

  // Mensagens do servidor -----------------------------------------------------

  void _onControl(String type, Map<String, dynamic> data) {
    switch (type) {
      case 'hello_ack':
        _sessionId = data['sessionId'] as String? ?? '';
        cameraName = data['cameraName'] as String? ?? cameraName;
        orientation = orientationOf(data['orientation']) ?? orientation;
        serverName = data['serverName'] as String? ?? serverName;
        if (data['profile'] is Map<String, dynamic>) {
          profile = QualityProfile.fromJson(data['profile'] as Map<String, dynamic>);
        }
      case 'tally':
        final connected = data['obsConnected'] == true;
        obsLive = data['live'] == true;
        tally = switch (data['state']) {
          'on_air' when connected => TallyState.onAir,
          'preview' when connected => TallyState.preview,
          'connected' when connected => TallyState.connected,
          _ => TallyState.unknown, // OBS fora: nunca exibir NO AR desatualizado
        };
      case 'config':
        cameraName = data['cameraName'] as String? ?? cameraName;
        final o = orientationOf(data['orientation']);
        if (o != null && o != orientation) {
          orientation = o;
          lastRemoteAction = o == 'portrait' ? 'Operador escolheu VERTICAL' : 'Operador escolheu HORIZONTAL';
        }
        if (data['profile'] is Map<String, dynamic>) {
          unawaited(setProfile(QualityProfile.fromJson(data['profile'] as Map<String, dynamic>), local: false));
        }
      case 'command':
        unawaited(_runCommand(data, DateTime.now()));
    }
    notifyListeners();
  }

  Future<void> _runCommand(Map<String, dynamic> cmd, DateTime receivedAt) async {
    final id = cmd['commandId'] as String? ?? '';
    final cached = _results.get(id);
    if (cached != null) return _control?.send('command_result', cached);

    final name = cmd['name'] as String? ?? '';
    final p = (cmd['params'] as Map<String, dynamic>?) ?? const {};
    Map<String, dynamic> result(String status, [String message = '']) =>
        {'commandId': id, 'status': status, if (message.isNotEmpty) 'message': message, 'state': _stateSnapshot()};

    Map<String, dynamic> r;
    if (cmd['sessionId'] != _sessionId) {
      r = result('rejected', 'sessão diferente');
    } else if (commandExpired(issuedAt: (cmd['issuedAt'] as num?)?.toInt() ?? 0, expiresAt: (cmd['expiresAt'] as num?)?.toInt() ?? 0, receivedAt: receivedAt)) {
      r = result('expired');
    } else if (paramsOf(name).any(_priority.locked) && cmd['force'] != true) {
      r = result('conflict', 'o operador do celular está ajustando este controle');
    } else {
      try {
        r = await _execute(name, p) ? result('executed') : result('unsupported', 'recurso não disponível neste aparelho');
      } catch (e) {
        r = result('failed', '$e');
      }
    }
    if (r['status'] == 'executed') lastRemoteAction = _describe(name);
    _results.put(id, r);
    _control?.send('command_result', r);
    notifyListeners();
  }

  /// Executa o comando validando as capacidades REAIS do aparelho.
  Future<bool> _execute(String name, Map<String, dynamic> p) async {
    double d(String k) => (p[k] as num).toDouble();
    switch (name) {
      case 'zoom.set':
        await setZoom(d('value'), local: false);
        return true;
      case 'zoom.step':
        await setZoom(zoom + d('delta'), local: false);
        return true;
      case 'lens.select':
        if (!lenses.any((l) => l.id == p['lensId'])) return false;
        await selectLens(p['lensId'] as String, local: false);
        return true;
      case 'camera.flip':
        await flipCamera(local: false);
        return true;
      case 'torch.set':
        if (!_hasTorch) return false;
        await setTorch(p['on'] == true, local: false);
        return true;
      case 'audio.mute':
        setMicMuted(p['muted'] == true, local: false);
        return true;
      case 'focus.point':
      case 'exposure.point':
        await focusAt(d('x'), d('y'), local: false);
        return true;
      case 'quality.set':
        await setProfile(QualityProfile.fromJson(p), local: false);
        return true;
      case 'preset.apply':
        {
          final presetLens = p['lensId'];
          if (presetLens is String && lenses.any((l) => l.id == presetLens) && presetLens != lensId) {
            await selectLens(presetLens, local: false);
          }
          final presetProfile = p['profile'];
          if (presetProfile is Map<String, dynamic>) {
            await setProfile(QualityProfile.fromJson(presetProfile), local: false);
          }
          final presetZoom = p['zoom'];
          if (presetZoom is num) await setZoom(presetZoom.toDouble(), local: false);
          final presetTorch = p['torch'];
          if (presetTorch is bool && _hasTorch) await setTorch(presetTorch, local: false);
          final presetMic = p['micMuted'];
          if (presetMic is bool) setMicMuted(presetMic, local: false);
          return true;
        }
      case 'stream.reconnect':
        if (_wantLive) {
          await _publisher?.stop();
          _publisher = null;
          await _publish();
        }
        return true;
    }
    return false;
  }

  String _describe(String name) => switch (name) {
        'zoom.set' || 'zoom.step' => 'Zoom ajustado pelo operador',
        'lens.select' || 'camera.flip' => 'Câmera trocada pelo operador',
        'quality.set' => 'Qualidade alterada pelo operador',
        'preset.apply' => 'Preset aplicado pelo operador',
        'audio.mute' => 'Microfone alterado pelo operador',
        _ => 'Ajuste remoto aplicado',
      };

  Map<String, dynamic> _stateSnapshot() => {
        'zoom': double.parse(zoom.toStringAsFixed(2)),
        'lensId': lensId,
        'torch': torch,
        'micMuted': micMuted,
        'profileId': profile.id,
      };

  // Telemetria e adaptação ----------------------------------------------------

  LinkConditions _conditions() => LinkConditions(
        availableKbps: stats.availableKbps,
        lossPercent: stats.lossPercent,
        rttMs: stats.rttMs,
        thermal: thermal,
        batteryLevel: batteryLevel ?? 1,
        charging: charging,
        cellular: network == 'cellular',
      );

  Future<void> _tick() async {
    final (level, isCharging) = await DeviceCaps.battery();
    batteryLevel = level;
    charging = isCharging;
    thermal = await DeviceCaps.thermal();
    if (_publisher?.active ?? false) {
      stats = await _publisher!.stats();
      final next = adapt(profile, _conditions());
      if (limits == null || next.maxBitrateKbps != limits!.maxBitrateKbps || next.scaleDownBy != limits!.scaleDownBy || next.maxFps != limits!.maxFps) {
        limits = next;
        await _publisher!.applyLimits(next);
      }
    }
    _control?.send('telemetry', {
      if (batteryLevel != null) 'battery': {'level': batteryLevel, 'charging': charging},
      'thermal': thermal,
      'stream': {
        'state': streamState.name,
        'bitrateKbps': stats.bitrateKbps,
        'fps': stats.fps,
        'width': stats.width,
        'height': stats.height,
        'packetsLost': stats.packetsLost,
        'rttMs': stats.rttMs,
        'jitterMs': stats.jitterMs,
        'availableKbps': stats.availableKbps,
      },
      'camera': {'lensId': lensId ?? '', 'facing': lens?.facing ?? 'back', 'zoom': zoom, 'torch': torch, 'micMuted': micMuted},
      'network': {'type': network, 'quality': _quality()},
    });
    notifyListeners();
  }

  /// Qualidade da rede para o indicador do celular.
  String _quality() {
    if (streamState != StreamState.live) return 'unknown';
    if (stats.lossPercent > 8 || stats.rttMs > 400) return 'poor';
    if (stats.lossPercent > 3 || stats.rttMs > 150) return 'fair';
    return 'good';
  }

  String get networkQuality => _quality();

  /// Teste de alcance externo (com o Wi-Fi desligado).
  Future<String> testExternal() async {
    final ext = creds.externalUrl;
    if (ext == null || ext.isEmpty) {
      throw ApiException(0, 'no_external', 'O acesso externo não está configurado no servidor. Habilite-o no painel (Rede) e pareie novamente.');
    }
    final ip = await api.reachability(ext, creds, network);
    return 'Servidor alcançado pela internet. IP visto pelo servidor: $ip';
  }

  @override
  void notifyListeners() {
    if (!_disposed) super.notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _wantLive = false;
    _telemetry?.cancel();
    _retry?.cancel();
    unawaited(_shutdown());
    super.dispose();
  }

  Future<void> _shutdown() async {
    await _netSub?.cancel();
    await _publisher?.stop();
    await _control?.close();
    for (final t in _stream?.getTracks() ?? const <MediaStreamTrack>[]) {
      await t.stop();
    }
    await _stream?.dispose();
    await renderer.dispose();
    await WakelockPlus.disable();
    api.close();
  }
}

/// Valida a orientação recebida do servidor ('landscape' ou 'portrait').
String? orientationOf(Object? v) => v == 'landscape' || v == 'portrait' ? v as String : null;
