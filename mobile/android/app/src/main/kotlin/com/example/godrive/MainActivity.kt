package com.example.godrive

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private var notificationPermissionResult: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "godrive/background_uploads")
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "isSupported" -> result.success(true)
                    "ensureNotificationPermission" -> ensureNotificationPermission(result)
                    "refreshUploads" -> result.success(null)
                    "startUpload" -> {
                        val args = call.arguments as? Map<*, *>
                        if (args == null) {
                            result.error("bad_args", "Missing upload arguments", null)
                            return@setMethodCallHandler
                        }
                        val intent = Intent(this, BackgroundUploadService::class.java).apply {
                            putExtra("id", args["id"] as? String)
                            putExtra("filePath", args["filePath"] as? String)
                            putExtra("filename", args["filename"] as? String)
                            putExtra("fileSize", (args["fileSize"] as? Number)?.toLong() ?: 0L)
                            putExtra("targetPath", args["targetPath"] as? String)
                            putExtra("tusUrl", args["tusUrl"] as? String)
                            putExtra("baseUrl", args["baseUrl"] as? String)
                            putExtra("token", args["token"] as? String)
                        }
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                            startForegroundService(intent)
                        } else {
                            startService(intent)
                        }
                        result.success(null)
                    }
                    else -> result.notImplemented()
                }
            }
    }

    private fun ensureNotificationPermission(result: MethodChannel.Result) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
            result.success(true)
            return
        }
        if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
            result.success(true)
            return
        }
        if (notificationPermissionResult != null) {
            result.error("permission_pending", "Notification permission request is already active", null)
            return
        }
        notificationPermissionResult = result
        requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_NOTIFICATIONS)
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_NOTIFICATIONS) return
        val granted = grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED
        notificationPermissionResult?.success(granted)
        notificationPermissionResult = null
    }

    companion object {
        private const val REQUEST_NOTIFICATIONS = 4201
    }
}
