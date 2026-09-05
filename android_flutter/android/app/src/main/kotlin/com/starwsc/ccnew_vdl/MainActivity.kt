package com.starwsc.ccnew_vdl

import android.content.Context
import android.content.Intent
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Bundle
import android.util.Log
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {

    companion object {
        private const val TAG = "MainActivity"
        private const val CHANNEL = "ccnew/native"
    }

    private var methodChannel: MethodChannel? = null
    private var pendingShareUrl: String? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        methodChannel = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL)
        methodChannel?.setMethodCallHandler { call, result ->
            when (call.method) {
                "getPendingShare" -> {
                    val url = pendingShareUrl
                    pendingShareUrl = null
                    result.success(url)
                }
                "saveToGallery" -> {
                    val path = call.argument<String>("path") ?: ""
                    val file = File(path)
                    if (file.exists()) {
                        val uri = MediaStoreHelper.saveToGallery(this, file)
                        result.success(uri) // 返回 content:// URI 或 null
                    } else {
                        result.success(null)
                    }
                }
                "deleteFile" -> {
                    val path = call.argument<String>("path") ?: ""
                    result.success(File(path).let { it.exists() && it.delete() })
                }
                "fileExists" -> {
                    val path = call.argument<String>("path") ?: ""
                    result.success(File(path).exists() && File(path).length() > 0)
                }
                "getNetworkType" -> {
                    // wifi / cellular / none —— 用于自适应轮询省流
                    val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
                    val caps = cm.getNetworkCapabilities(cm.activeNetwork)
                    val type = when {
                        caps == null -> "none"
                        caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) ||
                            caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "wifi"
                        caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
                        else -> "other"
                    }
                    result.success(type)
                }
                else -> result.notImplemented()
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShareIntent(intent)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handleShareIntent(intent)
    }

    private fun handleShareIntent(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND || intent.type != "text/plain") return
        val text = intent.getStringExtra(Intent.EXTRA_TEXT) ?: return
        val url = Regex("https?://[\\w\\-._~:/?#\\[\\]@!\\\$&'()*+,;=%]+").find(text)?.value ?: return
        Log.i(TAG, "收到分享: $url")
        pendingShareUrl = url
        methodChannel?.invokeMethod("onShareReceived", mapOf("url" to url))
    }
}
