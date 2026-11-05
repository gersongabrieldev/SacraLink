import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

/// Leitor do QR Code de pareamento. Retorna o texto lido.
class ScanScreen extends StatefulWidget {
  const ScanScreen({super.key});

  @override
  State<ScanScreen> createState() => _ScanScreenState();
}

class _ScanScreenState extends State<ScanScreen> {
  final _controller = MobileScannerController(formats: const [BarcodeFormat.qrCode]);
  bool _done = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Ler QR Code')),
      body: Stack(children: [
        MobileScanner(
          controller: _controller,
          onDetect: (capture) {
            if (_done) return;
            final value = capture.barcodes.firstOrNull?.rawValue;
            if (value != null && value.startsWith('sacralink://')) {
              _done = true;
              Navigator.of(context).pop(value);
            }
          },
        ),
        const Align(
          alignment: Alignment.bottomCenter,
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text('Aponte para o QR Code exibido no painel do SacraLink',
                textAlign: TextAlign.center, style: TextStyle(fontSize: 16, shadows: [Shadow(blurRadius: 8)])),
          ),
        ),
      ]),
    );
  }
}
