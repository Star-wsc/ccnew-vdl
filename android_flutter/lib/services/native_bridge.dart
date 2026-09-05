import 'package:flutter/services.dart';

class NativeBridge {
  static const _channel = MethodChannel('ccnew/native');

  /// 从原生层获取 Go 服务地址
  static Future<String> getServerUrl() async {
    try {
      return await _channel.invokeMethod('getServerUrl') ?? '';
    } catch (_) {
      return '';
    }
  }

  static Future<String?> getPendingShare() async {
    try {
      return await _channel.invokeMethod('getPendingShare');
    } catch (_) {
      return null;
    }
  }

  /// 保存到相册，返回 content:// URI 字符串（成功）或 null（失败）
  static Future<String?> saveToGallery(String filePath) async {
    try {
      return await _channel.invokeMethod('saveToGallery', {'path': filePath});
    } catch (_) {
      return null;
    }
  }

  static Future<bool> fileExists(String filePath) async {
    try {
      return await _channel.invokeMethod('fileExists', {'path': filePath}) ?? false;
    } catch (_) {
      return false;
    }
  }

  /// 当前网络类型：wifi / cellular / none（用于省流轮询策略）
  static Future<String> getNetworkType() async {
    try {
      return await _channel.invokeMethod('getNetworkType') ?? 'wifi';
    } catch (_) {
      return 'wifi'; // 检测失败按WiFi处理
    }
  }

  static Future<bool> deleteFile(String filePath) async {
    try {
      return await _channel.invokeMethod('deleteFile', {'path': filePath}) ?? false;
    } catch (_) {
      return false;
    }
  }

  static void setOnShareCallback(Function(String url) callback) {
    _channel.setMethodCallHandler((call) async {
      if (call.method == 'onShareReceived') {
        callback(call.arguments['url'] ?? '');
      }
    });
  }
}
