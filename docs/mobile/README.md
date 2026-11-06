# SacraLink Camera (Android e iOS)

App Flutter que transforma o celular em câmera sem fio do SacraLink.

## Recursos

- Pareamento por QR Code (ou código manual), com pinning do certificado do
  servidor e credenciais no Keychain/Keystore.
- Transmissão WebRTC/WHIP: H.264 (hardware) + Opus, direto para a igreja.
- Tally do OBS em tempo real: **● NO AR**, **PREVIEW**, **CONECTADA**.
- Zoom por pinça, controle deslizante e botões; lentes reais do aparelho;
  foco/exposição por toque; lanterna; microfone; qualidade.
- Controle remoto pelo operador central com confirmação e política de
  conflito (ver [contrato](../../packages/contracts/control-v1.md)).
- **Localizar servidor na rede** (mDNS): celulares já pareados reencontram o
  computador da igreja se o IP mudar; a identidade é confirmada pelo pinning.
  O pareamento em si é sempre pelo QR Code.
- Adaptação a Wi-Fi congestionado, 4G/5G, aquecimento e bateria; reconexão
  automática na troca de rede; teste de conexão externa.

## Limites honestos

- **A câmera para em segundo plano ou com a tela bloqueada.** Android e iOS
  suspendem a captura; o app mantém a tela ligada enquanto aberto.
- Zoom óptico só é anunciado quando a plataforma informa fatores ópticos
  reais; lentes são exibidas apenas se o sistema as expõe à câmera.
- Temperatura: Android 10+ e iOS informam estados (nominal → crítico).

## Requisitos

- Android 7.0+ (API 24), iOS 15.5+.

## Desenvolvimento

```bash
# 1. Flutter estável (testado no CI com 3.47) + Android SDK/JDK 17; Xcode no macOS para iOS
# 2. Gerar as pastas de plataforma (flutter create + permissões e alvos mínimos)
scripts/mobile/bootstrap.sh
cd apps/mobile
flutter pub get
flutter analyze && flutter test
flutter run            # aparelho conectado
```

As pastas `android/` e `ios/` são geradas pelo `bootstrap.sh` (não
versionadas no momento). O código nativo do SacraLink fica no plugin local
`packages/sacralink_device_caps` (Kotlin/Camera2 e Swift/AVFoundation).

## Distribuição

### Android

1. Crie uma chave: `keytool -genkey -v -keystore sacralink.jks -keyalg RSA -keysize 2048 -validity 10000 -alias sacralink`
   (nunca versione a chave).
2. Configure `android/key.properties` e o `signingConfig` de release.
3. `flutter build apk --release` (instalação direta) ou
   `flutter build appbundle --release` (Google Play).

O CI gera um **APK de depuração** a cada commit (artefato
`sacralink-camera-android-debug`) para testes internos.

### iOS

1. Conta Apple Developer, Bundle ID `br.org.sacralink.sacralinkCamera`
   (ajuste no Xcode), perfil de provisionamento.
2. `flutter build ipa --release` e envio pelo Transporter/TestFlight.

O CI compila o iOS **sem assinatura** apenas para validar o código.

## Permissões

| Plataforma | Permissão | Uso |
|---|---|---|
| Ambas | Câmera, microfone | Capturar e transmitir |
| iOS | Rede local | Alcançar o computador da igreja no Wi-Fi |
| Android | Internet, estado da rede, Wi-Fi, wake lock, Bluetooth (áudio) | Transmissão e tela ligada |
