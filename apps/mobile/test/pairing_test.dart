import 'package:flutter_test/flutter_test.dart';
import 'package:sacralink_camera/src/pairing.dart';

void main() {
  const fp = 'ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12';
  final exp = DateTime.now().add(const Duration(minutes: 5)).millisecondsSinceEpoch ~/ 1000;

  test('lê o QR Code gerado pelo servidor', () {
    final info = parsePairingUri(
        'sacralink://pair?v=1&id=srv1&n=Par%C3%B3quia+S%C3%A3o+Jos%C3%A9&h=192.168.0.10%2C10.0.0.5&p=7443&x=igreja.example.org%3A7443&fp=$fp&t=tokentokentokentokentoken&exp=$exp');
    expect(info.serverName, 'Paróquia São José');
    expect(info.lanHosts, ['192.168.0.10', '10.0.0.5']);
    expect(info.baseUrls, ['https://192.168.0.10:7443', 'https://10.0.0.5:7443', 'https://igreja.example.org:7443']);
    expect(info.fingerprint, fp);
    expect(info.expired, isFalse);
  });

  test('IPv6 vira [endereço] na URL', () {
    final info = parsePairingUri('sacralink://pair?v=1&h=fd00::10&p=7443&fp=$fp&t=tokentokentokentokentoken&exp=$exp');
    expect(info.baseUrls.first, 'https://[fd00::10]:7443');
  });

  test('rejeita QR de outro aplicativo ou incompleto', () {
    for (final bad in [
      'https://example.org',
      'sacralink://pair?v=2&h=1.2.3.4&p=7443&fp=$fp&t=tokentokentokentokentoken',
      'sacralink://pair?v=1&h=1.2.3.4&p=7443&fp=curto&t=tokentokentokentokentoken',
      'sacralink://pair?v=1&h=1.2.3.4&p=7443&fp=$fp&t=curto',
      'sacralink://pair?v=1&p=7443&fp=$fp&t=tokentokentokentokentoken',
      'sacralink://pair?v=1&h=1.2.3.4&p=99999&fp=$fp&t=tokentokentokentokentoken',
    ]) {
      expect(() => parsePairingUri(bad), throwsA(isA<PairingFormatException>()), reason: bad);
    }
  });

  test('QR Code expirado', () {
    final info = parsePairingUri('sacralink://pair?v=1&h=1.2.3.4&p=7443&fp=$fp&t=tokentokentokentokentoken&exp=1');
    expect(info.expired, isTrue);
  });
}
