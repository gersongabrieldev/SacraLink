// Adaptação de qualidade para Wi-Fi congestionado, 4G/5G e aquecimento.
// Função pura (testada em test/adaptive_test.dart): recebe o perfil e as
// condições medidas e devolve os limites a aplicar no RTCRtpSender.

class QualityProfile {
  const QualityProfile({
    required this.id,
    required this.width,
    required this.height,
    required this.fps,
    required this.maxBitrateKbps,
    required this.minBitrateKbps,
    this.adaptive = true,
  });

  factory QualityProfile.fromJson(Map<String, dynamic> j) => QualityProfile(
        id: j['id'] as String? ?? 'auto',
        width: (j['width'] as num?)?.toInt() ?? 1280,
        height: (j['height'] as num?)?.toInt() ?? 720,
        fps: (j['fps'] as num?)?.toInt() ?? 30,
        maxBitrateKbps: (j['maxBitrateKbps'] as num?)?.toInt() ?? 2500,
        minBitrateKbps: (j['minBitrateKbps'] as num?)?.toInt() ?? 300,
        adaptive: j['adaptive'] as bool? ?? true,
      );

  static const balanced = QualityProfile(id: 'equilibrado', width: 1280, height: 720, fps: 30, maxBitrateKbps: 2500, minBitrateKbps: 500);

  final String id;
  final int width;
  final int height;
  final int fps;
  final int maxBitrateKbps;
  final int minBitrateKbps;
  final bool adaptive;
}

class LinkConditions {
  const LinkConditions({
    this.availableKbps = 0,
    this.lossPercent = 0,
    this.rttMs = 0,
    this.thermal = 'nominal',
    this.batteryLevel = 1,
    this.charging = true,
    this.cellular = false,
  });

  final double availableKbps;
  final double lossPercent;
  final double rttMs;
  final String thermal;
  final double batteryLevel;
  final bool charging;
  final bool cellular;
}

class SenderLimits {
  const SenderLimits({required this.maxBitrateKbps, required this.scaleDownBy, required this.maxFps, required this.reason});
  final int maxBitrateKbps;
  final double scaleDownBy;
  final int maxFps;

  /// Motivo legível (exibido no celular quando há degradação).
  final String reason;

  bool get degraded => reason.isNotEmpty;
}

SenderLimits adapt(QualityProfile p, LinkConditions c) {
  var bitrate = p.maxBitrateKbps.toDouble();
  var scale = 1.0;
  var fps = p.fps;
  final reasons = <String>[];

  if (p.adaptive) {
    if (c.availableKbps > 0 && c.availableKbps * 0.85 < bitrate) {
      bitrate = c.availableKbps * 0.85;
      reasons.add('banda limitada');
    }
    if (c.lossPercent > 8) {
      bitrate *= 0.7;
      reasons.add('perda de pacotes alta');
    } else if (c.lossPercent > 3) {
      bitrate *= 0.85;
      reasons.add('perda de pacotes');
    }
    if (c.cellular && bitrate > 2500) {
      bitrate = 2500;
    }
  }
  switch (c.thermal) {
    case 'serious':
      scale = 1.5;
      fps = fps > 24 ? 24 : fps;
      reasons.add('aparelho quente');
    case 'critical':
      scale = 2.0;
      fps = 15;
      reasons.add('aparelho muito quente');
  }
  if (c.batteryLevel < 0.15 && !c.charging && scale < 1.5) {
    scale = 1.5;
    reasons.add('bateria baixa');
  }
  if (bitrate < p.minBitrateKbps * 1.2 && scale < 1.5 && p.adaptive) {
    scale = 1.5; // menos pixels por bit: imagem mais nítida em banda baixa
  }
  final minB = p.minBitrateKbps.toDouble();
  if (bitrate < minB) bitrate = minB;
  return SenderLimits(maxBitrateKbps: bitrate.round(), scaleDownBy: scale, maxFps: fps, reason: reasons.join(', '));
}
