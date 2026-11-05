import AVFoundation
import Flutter
import UIKit

/// Lê as capacidades REAIS das câmeras via AVFoundation: lentes físicas
/// (grande-angular, ultra-angular, teleobjetiva, frontal), faixa de zoom e,
/// em câmeras virtuais, os fatores de troca óptica. Também o estado térmico.
public class SacralinkDeviceCapsPlugin: NSObject, FlutterPlugin {
  public static func register(with registrar: FlutterPluginRegistrar) {
    let channel = FlutterMethodChannel(name: "br.org.sacralink/device_caps", binaryMessenger: registrar.messenger())
    registrar.addMethodCallDelegate(SacralinkDeviceCapsPlugin(), channel: channel)
  }

  public func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "lenses":
      result(lenses())
    case "thermal":
      result(thermal())
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private func lenses() -> [[String: Any]] {
    let types: [AVCaptureDevice.DeviceType] = [
      .builtInWideAngleCamera, .builtInUltraWideCamera, .builtInTelephotoCamera,
    ]
    let session = AVCaptureDevice.DiscoverySession(deviceTypes: types, mediaType: .video, position: .unspecified)
    return session.devices.map { d in
      var kind = "unknown"
      var label = d.localizedName
      switch (d.position, d.deviceType) {
      case (.front, _): kind = "front"; label = "Frontal"
      case (_, .builtInUltraWideCamera): kind = "ultra_wide"; label = "Ultra-angular"
      case (_, .builtInTelephotoCamera): kind = "telephoto"; label = "Teleobjetiva"
      case (_, .builtInWideAngleCamera): kind = "wide"; label = "Principal"
      default: break
      }
      var m: [String: Any] = [
        "id": d.uniqueID,
        "facing": d.position == .front ? "front" : "back",
        "kind": kind,
        "label": label,
        "minZoom": Double(d.minAvailableVideoZoomFactor),
        "maxZoom": Double(d.maxAvailableVideoZoomFactor),
      ]
      let switchOver = d.virtualDeviceSwitchOverVideoZoomFactors.map { $0.doubleValue }
      if !switchOver.isEmpty { m["opticalZoomFactors"] = [1.0] + switchOver }
      return m
    }
  }

  private func thermal() -> String {
    switch ProcessInfo.processInfo.thermalState {
    case .nominal: return "nominal"
    case .fair: return "fair"
    case .serious: return "serious"
    case .critical: return "critical"
    @unknown default: return "unknown"
    }
  }
}
