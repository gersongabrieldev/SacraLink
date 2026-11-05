import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'src/ui/home_screen.dart';
import 'src/ui/theme.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setSystemUIOverlayStyle(SystemUiOverlayStyle.light);
  runApp(const SacraLinkCameraApp());
}

class SacraLinkCameraApp extends StatelessWidget {
  const SacraLinkCameraApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'SacraLink Camera',
      debugShowCheckedModeBanner: false,
      theme: sacraTheme(),
      home: const HomeScreen(),
    );
  }
}
