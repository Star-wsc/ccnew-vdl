import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

class ApiService {
  static String baseUrl = '';
  static String? authToken;
  static String authUsername = '';
  static bool deleteServerAfterSave = false;

  static Future<void> init() async {
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString('server_url');
    if (saved != null && saved.isNotEmpty) baseUrl = saved;
    authToken = prefs.getString('auth_token');
    authUsername = prefs.getString('auth_username') ?? '';
    deleteServerAfterSave = prefs.getBool('delete_server_after_save') ?? false;
  }

  static Future<void> setServerUrl(String url) async {
    baseUrl = url.replaceAll(RegExp(r'/+$'), '');
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('server_url', baseUrl);
  }

  static Future<void> setDeleteServerAfterSave(bool value) async {
    deleteServerAfterSave = value;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('delete_server_after_save', value);
  }

  static Map<String, String> get _jsonHeaders => {
        'Content-Type': 'application/json',
        if (authToken != null && authToken!.isNotEmpty)
          'Authorization': 'Bearer $authToken',
      };

  static Map<String, String> get _authHeaders => {
        if (authToken != null && authToken!.isNotEmpty)
          'Authorization': 'Bearer $authToken',
      };

  /// 媒体播放器无法带 Header 时用 query token
  static String withToken(String url) {
    if (authToken == null || authToken!.isEmpty) return url;
    final sep = url.contains('?') ? '&' : '?';
    return '$url${sep}token=${Uri.encodeComponent(authToken!)}';
  }

  static Future<void> _saveToken(String? token, String username) async {
    authToken = token;
    authUsername = username;
    final prefs = await SharedPreferences.getInstance();
    if (token == null || token.isEmpty) {
      await prefs.remove('auth_token');
      await prefs.remove('auth_username');
    } else {
      await prefs.setString('auth_token', token);
      await prefs.setString('auth_username', username);
    }
  }

  /// 服务器鉴权状态（公开接口，不带 token 也可探测）
  static Future<Map<String, dynamic>> authStatus() async {
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/auth/status'))
          .timeout(const Duration(seconds: 4));
      if (resp.statusCode == 200) {
        return jsonDecode(resp.body) as Map<String, dynamic>;
      }
    } catch (_) {}
    return {'auth_mode': 'unknown', 'need_setup': false, 'logged_in': false};
  }

  static Future<Map<String, dynamic>> login(String username, String password) async {
    final resp = await http.post(
      Uri.parse('$baseUrl/api/auth/login'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    ).timeout(const Duration(seconds: 10));
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    if (resp.statusCode == 200 && body['ok'] == true) {
      final tok = (body['token'] ?? '').toString();
      if (tok.isNotEmpty) await _saveToken(tok, username);
      return {'ok': true};
    }
    return {'ok': false, 'error': body['error'] ?? '登录失败'};
  }

  static Future<void> logout() async {
    try {
      await http.post(Uri.parse('$baseUrl/api/auth/logout'), headers: _jsonHeaders);
    } catch (_) {}
    await _saveToken(null, '');
  }

  // ===== 数据模型 =====

  static bool lastUnauthorized = false;

  static Future<Map<String, dynamic>> getStats() async {
    lastUnauthorized = false;
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/stats'), headers: _authHeaders)
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 401) {
        lastUnauthorized = true;
        throw Exception('unauthorized');
      }
      return jsonDecode(resp.body);
    } catch (e) {
      if (lastUnauthorized) rethrow;
      return {
        'total': 0,
        'completed': 0,
        'downloading': 0,
        'failed': 0,
        'global_speed': 0
      };
    }
  }

  static Future<List<dynamic>> getTasks() async {
    lastUnauthorized = false;
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/tasks'), headers: _authHeaders)
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 401) {
        lastUnauthorized = true;
        throw Exception('unauthorized');
      }
      return jsonDecode(resp.body) as List<dynamic>;
    } catch (e) {
      if (lastUnauthorized) rethrow;
      return [];
    }
  }

  static Future<List<dynamic>> getCollections() async {
    lastUnauthorized = false;
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/collections'), headers: _authHeaders)
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 401) {
        lastUnauthorized = true;
        throw Exception('unauthorized');
      }
      return jsonDecode(resp.body) as List<dynamic>;
    } catch (e) {
      if (lastUnauthorized) rethrow;
      return [];
    }
  }

  static Future<String> getLogs() async {
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/logs'), headers: _authHeaders)
          .timeout(const Duration(seconds: 5));
      if (resp.statusCode == 401) return '';
      try {
        final list = jsonDecode(resp.body) as List<dynamic>;
        return list.map((e) {
          final ts = (e['timestamp'] ?? '').toString();
          final level = (e['level'] ?? '').toString();
          final msg = (e['message'] ?? '').toString();
          final time = ts.length >= 19 ? ts.substring(11, 19) : ts;
          return '$time [$level] $msg';
        }).join('\n');
      } catch (_) {
        return resp.body;
      }
    } catch (_) {
      return '';
    }
  }

  static Future<Map<String, dynamic>?> createTask(String url,
      {String quality = '4k'}) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks'),
        headers: _jsonHeaders,
        body: jsonEncode({'url': url, 'quality': quality, 'source': 'app'}),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }

  static Future<bool> deleteTask(String id, {bool deleteFile = true}) async {
    try {
      await http.delete(Uri.parse('$baseUrl/api/tasks/$id?deleteFile=$deleteFile'),
          headers: _authHeaders);
      return true;
    } catch (_) {
      return false;
    }
  }

  static Future<void> retryTask(String id) async {
    try {
      await http.post(Uri.parse('$baseUrl/api/tasks/$id/retry'), headers: _jsonHeaders);
    } catch (_) {}
  }

  static Future<Map<String, dynamic>> getConfig() async {
    try {
      final resp =
          await http.get(Uri.parse('$baseUrl/api/config'), headers: _authHeaders);
      if (resp.statusCode == 401) return {'version': '', 'platform': ''};
      return jsonDecode(resp.body);
    } catch (_) {
      return {'version': '', 'platform': ''};
    }
  }

  static Future<bool> setBilibiliCookie(String cookie) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/bilibili/cookie'),
        headers: _jsonHeaders,
        body: jsonEncode({'cookie': cookie}),
      );
      return resp.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  static Future<bool> setDouyinCookie(String cookie) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/settings'),
        headers: _jsonHeaders,
        body: jsonEncode({'douyin_cookie': cookie}),
      );
      return resp.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  static Future<void> clearLogs() async {
    try {
      await http.delete(Uri.parse('$baseUrl/api/logs'), headers: _authHeaders);
    } catch (_) {}
  }

  static Future<void> downloadCollection(String id) async {
    try {
      await http.post(Uri.parse('$baseUrl/api/collections/$id/download'),
          headers: _jsonHeaders);
    } catch (_) {}
  }

  static Future<void> deleteCollection(String id) async {
    try {
      await http.delete(Uri.parse('$baseUrl/api/collections/$id?deleteFile=true'),
          headers: _authHeaders);
    } catch (_) {}
  }

  static String collectionVideoUrl(String colId, int idx) =>
      withToken('$baseUrl/api/collections/$colId/videos/$idx/file');

  static Future<void> deleteCollectionVideo(String videoId) async {
    try {
      await http.delete(Uri.parse('$baseUrl/api/collections/videos/$videoId'),
          headers: _authHeaders);
    } catch (_) {}
  }

  static Future<void> toggleCollectionSubscribe(String colId, bool subscribe) async {
    try {
      await http.post(Uri.parse('$baseUrl/api/collections/$colId/subscribe'),
          headers: _jsonHeaders,
          body: jsonEncode(
              {'subscribe': subscribe, 'refresh_interval': 60}));
    } catch (_) {}
  }

  /// 探测服务器是否可达：走公开的 auth/status，401 业务接口也算“在线但要登录”
  static Future<bool> checkConnection() async {
    try {
      final resp = await http
          .get(Uri.parse('$baseUrl/api/auth/status'))
          .timeout(const Duration(seconds: 3));
      return resp.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  static String coverSrc(String u) {
    if (u.isEmpty) return u;
    final lower = u.toLowerCase();
    if (lower.contains('ytimg.com') ||
        lower.contains('ggpht.com') ||
        lower.contains('youtube.com')) {
      return withToken('$baseUrl/api/proxy/image?url=${Uri.encodeComponent(u)}');
    }
    return u;
  }

  static String streamUrl(String taskId) =>
      withToken('$baseUrl/api/tasks/$taskId/stream');
  static String downloadUrl(String taskId) =>
      withToken('$baseUrl/api/tasks/$taskId/download');

  // ===== 预览接口 =====

  static Future<Map<String, dynamic>?> previewUrl(String url) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks'),
        headers: _jsonHeaders,
        body: jsonEncode({'url': url, 'quality': 'preview'}),
      ).timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }

  static Future<Map<String, dynamic>?> createFromPreview(
      Map<String, dynamic> preview,
      {String quality = '4k'}) async {
    try {
      final url = preview['url'] ?? preview['video_url'] ?? '';
      final selectedQuality = (preview['quality'] ?? quality).toString();
      final resp = await http.post(
        Uri.parse('$baseUrl/api/tasks/create-from-preview'),
        headers: _jsonHeaders,
        body: jsonEncode({
          'url': url,
          'quality': selectedQuality,
          'preview_data': preview,
          'source': 'app',
        }),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }

  static Future<Map<String, dynamic>?> previewCollection(String url) async {
    try {
      final resp = await http.post(
        Uri.parse('$baseUrl/api/collections/preview'),
        headers: _jsonHeaders,
        body: jsonEncode({'url': url}),
      ).timeout(const Duration(seconds: 30));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }

  static Future<Map<String, dynamic>?> createCollection(
      Map<String, dynamic> preview,
      {List<int>? selectedIndices}) async {
    try {
      final body = Map<String, dynamic>.from(preview);
      if (selectedIndices != null) body['selected_indices'] = selectedIndices;
      body['source'] = 'app';
      body['auto_download'] = true;
      if ((body['quality'] ?? '').toString().isEmpty) body['quality'] = '4k';
      final resp = await http.post(
        Uri.parse('$baseUrl/api/collections'),
        headers: _jsonHeaders,
        body: jsonEncode(body),
      ).timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) return jsonDecode(resp.body);
      return null;
    } catch (_) {
      return null;
    }
  }
}
