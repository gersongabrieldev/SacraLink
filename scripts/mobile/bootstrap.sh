#!/usr/bin/env bash
# Gera as pastas de plataforma (android/ e ios/) do SacraLink Camera com o
# `flutter create` e aplica as configurações do projeto: permissões, nome,
# minSdk 24 (Android) e iOS 15.5. Idempotente; usado pelo CI e por quem
# for desenvolver o app.
#
# Uso: scripts/mobile/bootstrap.sh
set -euo pipefail
cd "$(dirname "$0")/../../apps/mobile"

flutter create --platforms=android,ios --org br.org.sacralink --project-name sacralink_camera \
  --description "SacraLink Camera" . >/dev/null
echo "plataformas geradas"

sedi() { sed -i.bak "$@" && rm -f "${@: -1}.bak"; }

# --- Android -----------------------------------------------------------------
MANIFEST=android/app/src/main/AndroidManifest.xml
if ! grep -q 'android.permission.CAMERA' "$MANIFEST"; then
  PERMS='    <uses-permission android:name="android.permission.INTERNET"/>\
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE"/>\
    <uses-permission android:name="android.permission.ACCESS_WIFI_STATE"/>\
    <uses-permission android:name="android.permission.CAMERA"/>\
    <uses-permission android:name="android.permission.RECORD_AUDIO"/>\
    <uses-permission android:name="android.permission.MODIFY_AUDIO_SETTINGS"/>\
    <uses-permission android:name="android.permission.BLUETOOTH_CONNECT"/>\
    <uses-permission android:name="android.permission.WAKE_LOCK"/>\
    <uses-feature android:name="android.hardware.camera" android:required="true"/>'
  sedi "/<manifest xmlns:android=/a\\
$PERMS" "$MANIFEST"
fi
sedi 's/android:label="sacralink_camera"/android:label="SacraLink Camera"/' "$MANIFEST"
for g in android/app/build.gradle.kts android/app/build.gradle; do
  [ -f "$g" ] && sedi -E 's/minSdk( =|Version) flutter\.minSdkVersion/minSdk = 24/' "$g"
done
echo "android configurado (minSdk 24)"

# --- iOS ---------------------------------------------------------------------
PLIST=ios/Runner/Info.plist
if [ -x /usr/libexec/PlistBuddy ]; then
  pb() { /usr/libexec/PlistBuddy -c "$1" "$PLIST" >/dev/null 2>&1 || /usr/libexec/PlistBuddy -c "${1/Add /Set }" "$PLIST" >/dev/null 2>&1 || true; }
  pb "Add :NSCameraUsageDescription string O SacraLink usa a câmera para transmitir o vídeo ao computador da igreja."
  pb "Add :NSMicrophoneUsageDescription string O SacraLink usa o microfone para transmitir o áudio da celebração."
  pb "Add :NSLocalNetworkUsageDescription string O SacraLink se conecta ao computador da igreja na rede local."
  pb "Set :CFBundleDisplayName SacraLink"
  pb "Add :UIRequiresFullScreen bool true"
  pb "Add :UIApplicationSupportsIndirectInputEvents bool true"
  echo "ios configurado (Info.plist)"
else
  # Fora do macOS: aplica as chaves com sed (o Info.plist é XML simples).
  if ! grep -q NSCameraUsageDescription "$PLIST"; then
    sedi 's#<dict>#<dict>\
	<key>NSCameraUsageDescription</key>\
	<string>O SacraLink usa a câmera para transmitir o vídeo ao computador da igreja.</string>\
	<key>NSMicrophoneUsageDescription</key>\
	<string>O SacraLink usa o microfone para transmitir o áudio da celebração.</string>\
	<key>NSLocalNetworkUsageDescription</key>\
	<string>O SacraLink se conecta ao computador da igreja na rede local.</string>#' "$PLIST"
  fi
  echo "ios configurado (Info.plist via sed)"
fi
PBX=ios/Runner.xcodeproj/project.pbxproj
[ -f "$PBX" ] && sedi -E 's/IPHONEOS_DEPLOYMENT_TARGET = [0-9.]+;/IPHONEOS_DEPLOYMENT_TARGET = 15.5;/g' "$PBX"
if [ -f ios/Podfile ]; then
  sedi -E "s/^#? *platform :ios.*/platform :ios, '15.5'/" ios/Podfile
fi
echo "ios: alvo mínimo 15.5"
