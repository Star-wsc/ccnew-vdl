import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

class ApiService {
  static String baseUrl = '';

  static Future<void> init() async {
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString('server_url');
    if (saved != null && saved.isNotEmpty) baseUrl = saved;
    deleteServerAfterSave = prefs.getBool('delete_server_after_save') ?? false;
  }

  static Future<void> setServerUrl(String url) async {
    baseUrl = url.replaceAll(RegExp(r'/+$'), '');
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('server_url', baseUrl);
  }

  /// 保存到相册后是否删除服务器缓存
  static bool deleteServerAfterSave = false;

  static Future<void> setDeleteServerAfterSave(bool value) async {
    deleteServerAfterSave = value;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('delete_server_after_save', value);
  }

  // ===== 数据模型 =====

  static Future<Map<String, dynamic>> getStats() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/stats?source=app'))
          .timeout(const Duration(seconds: 5));
      return jsonDecode(resp.body);
    } catch (_) {
      return {'total': 0, 'completed': 0, 'downloading': 0, 'failed': 0, 'global_speed': 0};
    }
  }

  static Future<List<dynamic>> getTasks() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/tasks?source=app'))
          .timeout(const Duration(seconds: 5));
      return jsonDecode(resp.body) as List<dynamic>;
    } catch (_) {
      return [];
    }
  }

  static Future<List<dynamic>> getCollections() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/collections?source=app'))
          .timeout(const Duration(seconds: 5));
      return jsonDecode(resp.body) as List<dynamic>;
    } catch (_) {
      return [];
    }
  }

  static Future<String> getLogs() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/logs'))
          .timeout(const Duration(seconds: 5));
      return resp.body;
    } catch (_) {
      return '';
    }
  }

  static Future<Map<String, dynamic>?> createTask(String url, {String quality = '4k'}) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'url': url, 'quality': quality, 'source': 'app'}),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }

  static Future<void> deleteTask(String id, {bool deleteFile = true}) async {
    try {
      await http.delete(Uri.parse('$baseUrl/api/tasks/$id?deleteFile=$deleteFile'));
    } catch (_) {}
  }

  static Future<void> retryTask(String id) async {
    try {
      await http.post(Uri.parse('$baseUrl/api/tasks/$id/retry'));
    } catch (_) {}
  }

  static Future<Map<String, dynamic>> getConfig() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/config'));
      return jsonDecode(resp.body);
    } catch (_) {
      return {'version': '', 'platform': ''};
    }
  }

  static Future<bool> setBilibiliCookie(String cookie) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/bilibili/cookie'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'cookie': cookie}),
      );
      return resp.statusCode == 200;
    } catch (_) { return false; }
  }

  static Future<bool> setDouyinCookie(String cookie) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/settings'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'douyin_cookie': cookie}),
      );
      return resp.statusCode == 200;
    } catch (_) { return false; }
  }

  static Future<void> clearLogs() async {
    try { await http.delete(Uri.parse('$baseUrl/api/logs')); } catch (_) {}
  }

  static Future<void> downloadCollection(String id) async {
    try { await http.post(Uri.parse('$baseUrl/api/collections/$id/download')); } catch (_) {}
  }

  static Future<void> deleteCollection(String id) async {
    try { await http.delete(Uri.parse('$baseUrl/api/collections/$id?deleteFile=true')); } catch (_) {}
  }

  /// 合集内单个视频的播放地址
  static String collectionVideoUrl(String colId, int idx) =>
      '$baseUrl/api/collections/$colId/videos/$idx/file';

  /// 删除合集内单个视频（抖音用video_id，B站用bvid）
  static Future<void> deleteCollectionVideo(String videoId) async {
    try { await http.delete(Uri.parse('$baseUrl/api/collections/videos/$videoId')); } catch (_) {}
  }

  /// 切换合集订阅
  static Future<void> toggleCollectionSubscribe(String colId, bool subscribe) async {
    try {
      await http.post(Uri.parse('$baseUrl/api/collections/$colId/subscribe'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'subscribe': subscribe, 'refresh_interval': 60}));
    } catch (_) {}
  }

  /// 检查服务器连通性
  static Future<bool> checkConnection() async {
    try {
      final resp = await http.get(Uri.parse('$baseUrl/api/stats'))
          .timeout(const Duration(seconds: 3));
      return resp.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  /// 获取下载 URL（兼容旧版用 /download，新版用 /stream 自动删服务端文件）
  static String streamUrl(String taskId) => '$baseUrl/api/tasks/$taskId/stream';
  static String downloadUrl(String taskId) => '$baseUrl/api/tasks/$taskId/download';

  // ===== 预览接口 =====

  /// 预览链接（不创建任务，返回解析结果，服务器会自动重试）
  static Future<Map<String, dynamic>?> previewUrl(String url) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'url': url, 'quality': 'preview'}),
      ).timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) { return null; }
  }

  /// 从预览数据创建下载任务
  static Future<Map<String, dynamic>?> createFromPreview(Map<String, dynamic> preview, {String quality = '4k'}) async {
    try {
      final url = preview['url'] ?? preview['video_url'] ?? '';
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks/create-from-preview'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({
          'url': url,
          'quality': quality,
          'preview_data': preview,
          'source': 'app',
        }),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) { return null; }
  }

  /// 预览合集
  static Future<Map<String, dynamic>?> previewCollection(String url) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/collections/preview'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'url': url}),
      ).timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) { return null; }
  }

  /// 创建合集（支持 selected_indices 选择部分视频）
  static Future<Map<String, dynamic>?> createCollection(Map<String, dynamic> preview, {List<int>? selectedIndices}) async {
    try {
      final body = Map<String, dynamic>.from(preview);
      if (selectedIndices != null) body['selected_indices'] = selectedIndices;
      body['source'] = 'app';
      body['auto_download'] = true; // 用户点了"添加到下载"，直接开下
      if ((body['quality'] ?? '').toString().isEmpty) body['quality'] = '4k';
      final resp = await http.post(
        Uri.parse('$baseUrl/api/collections'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode(body),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) { return null; }
  }
}
