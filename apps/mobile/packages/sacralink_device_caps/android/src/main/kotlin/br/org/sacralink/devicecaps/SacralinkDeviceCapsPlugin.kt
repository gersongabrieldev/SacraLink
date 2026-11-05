package br.org.sacralink.devicecaps

import android.content.Context
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraManager
import android.os.BatteryManager
import android.os.Build
import android.os.PowerManager
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

/**
 * Lê as capacidades REAIS das câmeras via Camera2: orientação, distância focal
 * (para classificar ultra-wide/tele), faixa de zoom (CONTROL_ZOOM_RATIO_RANGE no
 * Android 11+, senão o zoom digital máximo) e o estado térmico do aparelho.
 * Nada é inventado: se a plataforma não informa, o campo fica ausente.
 */
class SacralinkDeviceCapsPlugin : FlutterPlugin, MethodChannel.MethodCallHandler {
    private lateinit var channel: MethodChannel
    private lateinit var context: Context

    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        context = binding.applicationContext
        channel = MethodChannel(binding.binaryMessenger, "br.org.sacralink/device_caps")
        channel.setMethodCallHandler(this)
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        channel.setMethodCallHandler(null)
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "lenses" -> try {
                result.success(lenses())
            } catch (e: Exception) {
                result.error("camera", e.message, null)
            }
            "thermal" -> result.success(thermal())
            "battery" -> result.success(battery())
            else -> result.notImplemented()
        }
    }

    private fun lenses(): List<Map<String, Any>> {
        val manager = context.getSystemService(Context.CAMERA_SERVICE) as CameraManager
        data class Raw(val id: String, val facing: String, val focal: Float, val min: Float, val max: Float)
        val raws = manager.cameraIdList.mapNotNull { id ->
            val c = manager.getCameraCharacteristics(id)
            val facing = when (c.get(CameraCharacteristics.LENS_FACING)) {
                CameraCharacteristics.LENS_FACING_FRONT -> "front"
                CameraCharacteristics.LENS_FACING_BACK -> "back"
                CameraCharacteristics.LENS_FACING_EXTERNAL -> "external"
                else -> return@mapNotNull null
            }
            val focal = c.get(CameraCharacteristics.LENS_INFO_AVAILABLE_FOCAL_LENGTHS)?.minOrNull() ?: 0f
            var min = 1f
            var max = c.get(CameraCharacteristics.SCALER_AVAILABLE_MAX_DIGITAL_ZOOM) ?: 1f
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                c.get(CameraCharacteristics.CONTROL_ZOOM_RATIO_RANGE)?.let {
                    min = it.lower
                    max = it.upper
                }
            }
            Raw(id, facing, focal, min, max)
        }
        val mainBackFocal = raws.firstOrNull { it.facing == "back" }?.focal ?: 0f
        return raws.map { r ->
            val kind = when {
                r.facing == "front" -> "front"
                r.facing != "back" || mainBackFocal <= 0f || r.focal <= 0f -> "unknown"
                r.focal < mainBackFocal * 0.8f -> "ultra_wide"
                r.focal > mainBackFocal * 1.5f -> "telephoto"
                else -> "wide"
            }
            val label = when (kind) {
                "front" -> "Frontal"
                "ultra_wide" -> "Ultra-angular"
                "telephoto" -> "Teleobjetiva"
                "wide" -> "Principal"
                else -> "Câmera ${r.id}"
            }
            val m = mutableMapOf<String, Any>(
                "id" to r.id, "facing" to r.facing, "kind" to kind, "label" to label,
                "minZoom" to r.min.toDouble(), "maxZoom" to r.max.toDouble(),
            )
            // Zoom abaixo de 1× indica câmera lógica multi-lente (troca óptica automática).
            if (r.min < 1f) m["opticalZoomFactors"] = listOf(r.min.toDouble(), 1.0)
            m
        }
    }

    private fun battery(): Map<String, Any> {
        val bm = context.getSystemService(Context.BATTERY_SERVICE) as BatteryManager
        val capacity = bm.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY)
        return mapOf("level" to (if (capacity in 0..100) capacity / 100.0 else -1.0), "charging" to bm.isCharging)
    }

    private fun thermal(): String {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return "unknown"
        val pm = context.getSystemService(Context.POWER_SERVICE) as PowerManager
        return when (pm.currentThermalStatus) {
            PowerManager.THERMAL_STATUS_NONE -> "nominal"
            PowerManager.THERMAL_STATUS_LIGHT -> "fair"
            PowerManager.THERMAL_STATUS_MODERATE -> "fair"
            PowerManager.THERMAL_STATUS_SEVERE -> "serious"
            else -> "critical"
        }
    }
}
