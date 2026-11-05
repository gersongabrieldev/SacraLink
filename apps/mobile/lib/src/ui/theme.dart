import 'package:flutter/material.dart';

/// Cores da identidade do SacraLink (iguais ao painel desktop).
class SacraColors {
  static const background = Color(0xFF0B0E14);
  static const surface = Color(0xFF131823);
  static const surface2 = Color(0xFF1A2030);
  static const accent = Color(0xFFF0A83A);
  static const onAir = Color(0xFFE5484D);
  static const preview = Color(0xFF30A46C);
  static const warn = Color(0xFFF5B942);
  static const muted = Color(0xFF8B94A6);
}

ThemeData sacraTheme() {
  final scheme = ColorScheme.fromSeed(
    seedColor: SacraColors.accent,
    brightness: Brightness.dark,
    surface: SacraColors.surface,
  ).copyWith(primary: SacraColors.accent, onPrimary: Colors.black, error: SacraColors.onAir);
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: SacraColors.background,
    appBarTheme: const AppBarTheme(backgroundColor: SacraColors.background, elevation: 0),
    cardTheme: CardThemeData(color: SacraColors.surface, shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14))),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(minimumSize: const Size(48, 52), textStyle: const TextStyle(fontWeight: FontWeight.w600, fontSize: 16)),
    ),
  );
}
