/// Capacidades reais das câmeras e estado térmico do aparelho.
library;

import 'package:flutter/services.dart';

/// Uma câmera/lente física informada pela plataforma.
class LensInfo {
  LensInfo({
    required this.id,
    required this.facing,
    required this.kind,
    required this.label,
    required this.minZoom,
    required this.maxZoom,
    this.opticalZoomFactors = const [],
  });

  factory LensInfo.fromMap(Map<dynamic, dynamic> m) => LensInfo(
        id: m['id'] as String,
        facing: m['facing'] as String? ?? 'back',
        kind: m['kind'] as String? ?? 'unknown',
        label: m['label'] as String? ?? 'Câmera',
        minZoom: (m['minZoom'] as num?)?.toDouble() ?? 1,
        maxZoom: (m['maxZoom'] as num?)?.toDouble() ?? 1,
        opticalZoomFactors: ((m['opticalZoomFactors'] as List?) ?? const []).map((e) => (e as num).toDouble()).toList(),
      );

  final String id;
  final String facing;
  final String kind;
  final String label;
  final double minZoom;
  final double maxZoom;

  /// Fatores ópticos reais (vazio = desconhecido; nunca prometer zoom óptico).
  final List<double> opticalZoomFactors;

  Map<String, dynamic> toJson() => {
        'id': id,
        'facing': facing,
        'kind': kind,
        'label': label,
        'minZoom': minZoom,
        'maxZoom': maxZoom,
        if (opticalZoomFactors.isNotEmpty) 'opticalZoomFactors': opticalZoomFactors,
      };
}

/// Acesso ao canal nativo.
class DeviceCaps {
  static const _channel = MethodChannel('br.org.sacralink/device_caps');

  /// Lentes físicas disponíveis (lista vazia se a plataforma não informar).
  static Future<List<LensInfo>> lenses() async {
    try {
      final raw = await _channel.invokeListMethod<dynamic>('lenses') ?? const [];
      return raw.map((e) => LensInfo.fromMap(e as Map<dynamic, dynamic>)).toList();
    } on PlatformException {
      return const [];
    } on MissingPluginException {
      return const [];
    }
  }

  /// Bateria: (nível 0..1 ou null se desconhecido, carregando).
  static Future<(double?, bool)> battery() async {
    try {
      final m = await _channel.invokeMapMethod<String, dynamic>('battery') ?? const {};
      final level = (m['level'] as num?)?.toDouble() ?? -1;
      return (level < 0 ? null : level, m['charging'] as bool? ?? false);
    } on PlatformException {
      return (null, false);
    } on MissingPluginException {
      return (null, false);
    }
  }

  /// Estado térmico: nominal | fair | serious | critical | unknown.
  static Future<String> thermal() async {
    try {
      return await _channel.invokeMethod<String>('thermal') ?? 'unknown';
    } on PlatformException {
      return 'unknown';
    } on MissingPluginException {
      return 'unknown';
    }
  }
}
