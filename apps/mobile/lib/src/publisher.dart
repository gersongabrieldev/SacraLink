import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import 'adaptive.dart';
import 'server_api.dart';

/// Estatísticas de envio calculadas a partir de getStats().
class SendStats {
  const SendStats({
    this.bitrateKbps = 0,
    this.fps = 0,
    this.width = 0,
    this.height = 0,
    this.packetsLost = 0,
    this.lossPercent = 0,
    this.rttMs = 0,
    this.jitterMs = 0,
    this.availableKbps = 0,
  });
  final double bitrateKbps;
  final double fps;
  final int width;
  final int height;
  final int packetsLost;
  final double lossPercent;
  final double rttMs;
  final double jitterMs;
  final double availableKbps;
}

/// Publica a câmera via WHIP. Somente a sinalização (SDP) passa pelo
/// servidor; a mídia segue direto para a porta ICE do MediaMTX da igreja.
class WhipPublisher {
  WhipPublisher({required this.api, required this.creds, required this.baseUrl, required this.onConnection});

  final ServerApi api;
  final ServerCredentials creds;
  final String baseUrl;
  final void Function(RTCPeerConnectionState state) onConnection;

  RTCPeerConnection? _pc;
  RTCRtpSender? _videoSender;
  String? _location;
  int _lastBytes = 0;
  DateTime _lastStatsAt = DateTime.now();
  int _lastLost = 0;
  int _lastPackets = 0;

  bool get active => _pc != null;

  /// Inicia a sessão WHIP com as faixas da câmera e do microfone.
  Future<void> start(MediaStream stream, QualityProfile profile) async {
    await stop();
    final pc = await createPeerConnection({
      'sdpSemantics': 'unified-plan',
      'bundlePolicy': 'max-bundle',
      // Sem servidores ICE externos: a conexão é direta com a igreja.
      'iceServers': <Map<String, dynamic>>[],
    });
    _pc = pc;
    pc.onConnectionState = onConnection;

    for (final track in stream.getAudioTracks()) {
      await pc.addTransceiver(track: track, kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
          init: RTCRtpTransceiverInit(direction: TransceiverDirection.SendOnly, streams: [stream]));
    }
    for (final track in stream.getVideoTracks()) {
      final t = await pc.addTransceiver(track: track, kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
          init: RTCRtpTransceiverInit(direction: TransceiverDirection.SendOnly, streams: [stream]));
      _videoSender = t.sender;
      await _preferH264(t);
    }

    final offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    await _waitIceGathering(pc);
    final local = await pc.getLocalDescription();

    final (status, headers, body) = await api.raw('POST', Uri.parse('$baseUrl/device/v1/whip'),
        body: local!.sdp, contentType: 'application/sdp', headers: {'authorization': creds.authorization},
        timeout: const Duration(seconds: 15));
    if (status != 201) {
      await stop();
      if (status == 503 && body.contains('stream_limit')) {
        throw ApiException(status, 'stream_limit', 'Limite de câmeras transmitindo atingido no servidor.');
      }
      if (status == 503) throw ApiException(status, 'media_starting', 'O servidor de mídia está iniciando; tentando novamente.');
      throw ApiException(status, 'whip', 'O servidor recusou a transmissão (HTTP $status).');
    }
    _location = headers['location'];
    await pc.setRemoteDescription(RTCSessionDescription(body, 'answer'));
    await applyLimits(adapt(profile, const LinkConditions()));
  }

  /// H.264 primeiro: codificação por hardware e compatível com o OBS.
  Future<void> _preferH264(RTCRtpTransceiver t) async {
    try {
      final caps = await getRtpSenderCapabilities('video');
      final codecs = caps.codecs ?? [];
      final h264 = codecs.where((c) => c.mimeType.toLowerCase() == 'video/h264').toList();
      if (h264.isEmpty) return;
      final rest = codecs.where((c) => c.mimeType.toLowerCase() != 'video/h264').toList();
      await t.setCodecPreferences([...h264, ...rest]);
    } catch (_) {
      // Plataforma sem suporte a setCodecPreferences: o servidor ainda
      // negocia H.264, que é o codec aceito pelo MediaMTX/OBS.
    }
  }

  Future<void> _waitIceGathering(RTCPeerConnection pc) async {
    if (pc.iceGatheringState == RTCIceGatheringState.RTCIceGatheringStateComplete) return;
    final done = Completer<void>();
    pc.onIceGatheringState = (s) {
      if (s == RTCIceGatheringState.RTCIceGatheringStateComplete && !done.isCompleted) done.complete();
    };
    await done.future.timeout(const Duration(seconds: 3), onTimeout: () {});
  }

  /// Aplica limites de bitrate/resolução/FPS sem renegociar a sessão.
  Future<void> applyLimits(SenderLimits l) async {
    final sender = _videoSender;
    if (sender == null) return;
    final params = sender.parameters;
    final encodings = params.encodings;
    if (encodings == null || encodings.isEmpty) return;
    for (final e in encodings) {
      e.maxBitrate = l.maxBitrateKbps * 1000;
      e.maxFramerate = l.maxFps;
      e.scaleResolutionDownBy = l.scaleDownBy;
    }
    await sender.setParameters(params);
  }

  /// Troca a faixa de vídeo (lente/câmera) mantendo a sessão WebRTC.
  Future<void> replaceVideoTrack(MediaStreamTrack track) async {
    await _videoSender?.replaceTrack(track);
  }

  Future<SendStats> stats() async {
    final pc = _pc;
    if (pc == null) return const SendStats();
    final reports = await pc.getStats();
    var bytes = 0, width = 0, height = 0, lost = 0, packets = 0;
    double fps = 0, rtt = 0, jitter = 0, available = 0;
    for (final r in reports) {
      final v = r.values;
      num n(String k) => (v[k] as num?) ?? 0;
      switch (r.type) {
        case 'outbound-rtp':
          if (v['kind'] == 'video') {
            bytes += n('bytesSent').toInt();
            packets += n('packetsSent').toInt();
            fps = n('framesPerSecond').toDouble();
            width = n('frameWidth').toInt();
            height = n('frameHeight').toInt();
          }
        case 'remote-inbound-rtp':
          if (v['kind'] == 'video') {
            lost += n('packetsLost').toInt();
            jitter = n('jitter').toDouble() * 1000;
            rtt = n('roundTripTime').toDouble() * 1000;
          }
        case 'candidate-pair':
          if (v['state'] == 'succeeded' && (v['nominated'] == true || v['selected'] == true)) {
            if (n('currentRoundTripTime') > 0) rtt = n('currentRoundTripTime').toDouble() * 1000;
            available = n('availableOutgoingBitrate').toDouble() / 1000;
          }
      }
    }
    final now = DateTime.now();
    final dt = now.difference(_lastStatsAt).inMilliseconds / 1000;
    final kbps = dt > 0 && bytes >= _lastBytes ? (bytes - _lastBytes) * 8 / dt / 1000 : 0.0;
    final dPackets = packets - _lastPackets;
    final dLost = lost - _lastLost;
    final lossPct = dPackets > 0 && dLost > 0 ? dLost * 100 / (dPackets + dLost) : 0.0;
    _lastBytes = bytes;
    _lastStatsAt = now;
    _lastLost = lost;
    _lastPackets = packets;
    return SendStats(
      bitrateKbps: kbps, fps: fps, width: width, height: height, packetsLost: lost,
      lossPercent: lossPct, rttMs: rtt, jitterMs: jitter, availableKbps: available,
    );
  }

  /// Encerra a sessão (DELETE na sessão WHIP para liberar o servidor).
  Future<void> stop() async {
    final loc = _location;
    _location = null;
    if (loc != null) {
      unawaited(api
          .raw('DELETE', Uri.parse('$baseUrl$loc'), headers: {'authorization': creds.authorization}, timeout: const Duration(seconds: 3))
          .then((_) {}, onError: (_) {}));
    }
    final pc = _pc;
    _pc = null;
    _videoSender = null;
    _lastBytes = 0;
    _lastLost = 0;
    _lastPackets = 0;
    await pc?.close();
  }
}
