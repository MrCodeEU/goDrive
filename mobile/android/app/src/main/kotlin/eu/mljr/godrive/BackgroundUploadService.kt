package eu.mljr.godrive

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.json.JSONTokener
import java.io.BufferedInputStream
import java.io.File
import java.io.FileInputStream
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.util.Collections
import kotlin.concurrent.thread

class BackgroundUploadService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent == null) {
            stopSelf(startId)
            return START_NOT_STICKY
        }

        val request = UploadRequest.from(intent)
        val notificationId = request.id.hashCode().let { if (it == Int.MIN_VALUE) 1 else kotlin.math.abs(it) }
        ensureChannel()
        startForeground(notificationId, notification(request.filename, "Preparing upload", 0, false))
        activeIds.add(request.id)

        thread(name = "godrive-background-upload-${request.id}") {
            val backoffMs = longArrayOf(15_000L, 45_000L, 90_000L)
            var lastError: Exception? = null

            try {
                for (attempt in 0..backoffMs.size) {
                    if (attempt > 0) {
                        updateQueue(
                            request.id,
                            status = "background",
                            error = "Network error, retrying ($attempt/${backoffMs.size})…",
                            finalPath = null,
                            tusUrl = request.tusUrl,
                        )
                        notify(notificationId, notification(request.filename, "Retrying upload…", 0, true))
                        Thread.sleep(backoffMs[attempt - 1])
                    }
                    try {
                        updateQueue(request.id, status = "background", error = null, finalPath = null, tusUrl = request.tusUrl)
                        val finalPath = upload(request, notificationId)
                        updateQueue(request.id, status = "done", error = null, finalPath = finalPath, tusUrl = request.tusUrl, progress = 1.0)
                        notify(notificationId, notification(request.filename, "Upload complete", 100, false))
                        lastError = null
                        break
                    } catch (e: java.io.IOException) {
                        lastError = e
                        if (attempt == backoffMs.size) break
                    } catch (e: Exception) {
                        lastError = e
                        break
                    }
                }
                if (lastError != null) {
                    updateQueue(request.id, status = "error", error = lastError!!.message ?: "Background upload failed", finalPath = null, tusUrl = request.tusUrl)
                    notify(notificationId, notification(request.filename, "Upload failed", 0, false))
                }
            } finally {
                activeIds.remove(request.id)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
                    stopForeground(STOP_FOREGROUND_DETACH)
                } else {
                    @Suppress("DEPRECATION")
                    stopForeground(false)
                }
                stopSelf(startId)
            }
        }

        return START_REDELIVER_INTENT
    }

    private fun upload(request: UploadRequest, notificationId: Int): String? {
        val file = File(request.filePath)
        if (!file.isFile) error("File is no longer available on this device")

        var tusUrl = request.tusUrl
        if (tusUrl.isNullOrBlank()) {
            tusUrl = createUpload(request)
            request.tusUrl = tusUrl
            updateQueue(request.id, status = "background", error = null, finalPath = null, tusUrl = tusUrl)
        }

        val offset = try {
            getOffset(request, tusUrl)
        } catch (e: UploadGoneException) {
            tusUrl = createUpload(request)
            request.tusUrl = tusUrl
            updateQueue(request.id, status = "background", error = null, finalPath = null, tusUrl = tusUrl)
            0L
        }

        if (offset >= request.fileSize) return null
        return patchUpload(request, tusUrl, offset, notificationId)
    }

    private fun createUpload(request: UploadRequest): String {
        val url = URL("${request.baseUrl.trimEnd('/')}/api/tus?path=${encode(request.targetPath)}")
        val conn = (url.openConnection() as HttpURLConnection).apply {
            requestMethod = "POST"
            connectTimeout = 15000
            readTimeout = 15000
            setRequestProperty("Authorization", "Bearer ${request.token}")
            setRequestProperty("Tus-Resumable", "1.0.0")
            setRequestProperty("Upload-Length", request.fileSize.toString())
            setRequestProperty("Upload-Metadata", "filename ${Base64.encodeToString(request.filename.toByteArray(StandardCharsets.UTF_8), Base64.NO_WRAP)}")
        }
        return conn.use {
            if (responseCode != HttpURLConnection.HTTP_CREATED) {
                error(readError("Upload create failed"))
            }
            getHeaderField("Location") ?: error("Upload endpoint did not return Location")
        }
    }

    private fun getOffset(request: UploadRequest, tusUrl: String): Long {
        val conn = (URL(resolveUrl(request.baseUrl, tusUrl)).openConnection() as HttpURLConnection).apply {
            requestMethod = "HEAD"
            connectTimeout = 15000
            readTimeout = 15000
            setRequestProperty("Authorization", "Bearer ${request.token}")
            setRequestProperty("Tus-Resumable", "1.0.0")
        }
        conn.use {
            if (responseCode == HttpURLConnection.HTTP_NOT_FOUND) throw UploadGoneException()
            if (responseCode != HttpURLConnection.HTTP_NO_CONTENT) error(readError("HEAD failed"))
            return getHeaderField("Upload-Offset")?.toLongOrNull() ?: 0L
        }
    }

    private fun patchUpload(request: UploadRequest, tusUrl: String, offset: Long, notificationId: Int): String? {
        val file = File(request.filePath)
        val remaining = request.fileSize - offset
        val conn = (URL(resolveUrl(request.baseUrl, tusUrl)).openConnection() as HttpURLConnection).apply {
            requestMethod = "PATCH"
            doOutput = true
            connectTimeout = 15000
            readTimeout = 300_000
            setRequestProperty("Authorization", "Bearer ${request.token}")
            setRequestProperty("Content-Type", "application/offset+octet-stream")
            setRequestProperty("Tus-Resumable", "1.0.0")
            setRequestProperty("Upload-Offset", offset.toString())
            setFixedLengthStreamingMode(remaining)
        }

        var sent = offset
        var lastPersistedProgress = -1
        BufferedInputStream(FileInputStream(file)).use { input ->
            if (offset > 0) {
                var skipped = 0L
                while (skipped < offset) {
                    val n = input.skip(offset - skipped)
                    if (n <= 0) error("Failed to seek upload offset")
                    skipped += n
                }
            }
            conn.outputStream.use { output ->
                val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                while (true) {
                    val read = input.read(buffer)
                    if (read < 0) break
                    output.write(buffer, 0, read)
                    sent += read
                    val progress = ((sent * 100) / request.fileSize.coerceAtLeast(1)).toInt()
                    notify(notificationId, notification(request.filename, "Uploading", progress, true))
                    if (progress >= lastPersistedProgress + 2 || progress == 100) {
                        lastPersistedProgress = progress
                        updateQueue(
                            request.id,
                            status = "background",
                            error = null,
                            finalPath = null,
                            tusUrl = request.tusUrl,
                            progress = progress / 100.0,
                        )
                    }
                }
            }
        }

        conn.use {
            if (responseCode in 200..299) {
                return getHeaderField("Upload-Final-Path")
            }
            error(readError("Upload chunk failed"))
        }
    }

    private fun notification(title: String, text: String, progress: Int, ongoing: Boolean): Notification {
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, CHANNEL_ID)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        builder
            .setSmallIcon(android.R.drawable.stat_sys_upload)
            .setContentTitle(title)
            .setContentText(text)
            .setOngoing(ongoing)
            .setOnlyAlertOnce(true)
        if (ongoing) builder.setProgress(100, progress.coerceIn(0, 100), false)
        return builder.build()
    }

    private fun notify(id: Int, notification: Notification) {
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(id, notification)
    }

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "goDrive uploads", NotificationManager.IMPORTANCE_LOW)
        )
    }

    private fun updateQueue(
        id: String,
        status: String,
        error: String?,
        finalPath: String?,
        tusUrl: String?,
        progress: Double? = null,
    ) {
        val prefs = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
        val key = "flutter.godrive_upload_queue"
        val array = loadQueueItems(prefs.getString(key, "[]"))
        for (i in 0 until array.length()) {
            val item = array.optJSONObject(i) ?: continue
            if (item.optString("id") != id) continue
            item.put("status", status)
            item.put("progress", progress ?: if (status == "done") 1.0 else item.optDouble("progress", 0.0))
            if (error == null) item.remove("error") else item.put("error", error)
            if (finalPath == null) item.remove("final_path") else item.put("final_path", finalPath)
            if (tusUrl == null) item.remove("tus_url") else item.put("tus_url", tusUrl)
            prefs.edit().putString(key, encodeQueueItems(array)).apply()
            return
        }
    }

    private fun loadQueueItems(raw: String?): JSONArray {
        val value = JSONTokener(raw ?: "[]").nextValue()
        return when (value) {
            is JSONArray -> value
            is JSONObject -> value.optJSONArray("items") ?: JSONArray()
            else -> JSONArray()
        }
    }

    private fun encodeQueueItems(items: JSONArray): String =
        JSONObject()
            .put("version", QUEUE_SCHEMA_VERSION)
            .put("items", items)
            .toString()

    private fun HttpURLConnection.readError(fallback: String): String {
        val stream = errorStream ?: inputStream ?: return fallback
        return stream.bufferedReader().use { it.readText() }.ifBlank { fallback }
    }

    private inline fun <T> HttpURLConnection.use(block: HttpURLConnection.() -> T): T {
        try {
            return block()
        } finally {
            disconnect()
        }
    }

    private fun resolveUrl(baseUrl: String, tusUrl: String): String {
        if (tusUrl.startsWith("http://") || tusUrl.startsWith("https://")) return tusUrl
        return "${baseUrl.trimEnd('/')}$tusUrl"
    }

    private fun encode(value: String): String = URLEncoder.encode(value, StandardCharsets.UTF_8.name())

    private class UploadGoneException : Exception()

    private data class UploadRequest(
        val id: String,
        val filePath: String,
        val filename: String,
        val fileSize: Long,
        val targetPath: String,
        var tusUrl: String?,
        val baseUrl: String,
        val token: String,
    ) {
        companion object {
            fun from(intent: Intent): UploadRequest = UploadRequest(
                id = requireNotNull(intent.getStringExtra("id")),
                filePath = requireNotNull(intent.getStringExtra("filePath")),
                filename = requireNotNull(intent.getStringExtra("filename")),
                fileSize = intent.getLongExtra("fileSize", 0L),
                targetPath = requireNotNull(intent.getStringExtra("targetPath")),
                tusUrl = intent.getStringExtra("tusUrl"),
                baseUrl = requireNotNull(intent.getStringExtra("baseUrl")),
                token = requireNotNull(intent.getStringExtra("token")),
            )
        }
    }

    companion object {
        private const val CHANNEL_ID = "godrive_uploads"
        private const val DEFAULT_BUFFER_SIZE = 256 * 1024
        private const val QUEUE_SCHEMA_VERSION = 1

        private val activeIds: MutableSet<String> = Collections.synchronizedSet(mutableSetOf())

        fun isActive(id: String): Boolean = activeIds.contains(id)
    }
}
