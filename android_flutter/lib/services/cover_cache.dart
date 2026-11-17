import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:path_provider/path_provider.dart';

import 'api_service.dart';

/// 封面本地缓存（App 私有目录，扩展名 .dat，不进系统相册）
class CoverCache {
  static Future<Directory> _dir() async {
    final root = await getApplicationSupportDirectory();
    final d = Directory('${root.path}/covers');
    if (!await d.exists()) {
      await d.create(recursive: true);
    }
    return d;
  }

  static String _safe(String id) =>
      id.replaceAll(RegExp(r'[^A-Za-z0-9_-]'), '_');

  static Future<String?> localPath(String taskId, String coverUrl) async {
    if (taskId.isEmpty && coverUrl.isEmpty) return null;
    final key = taskId.isNotEmpty ? taskId : coverUrl.hashCode.toRadixString(16);
    final dir = await _dir();
    final f = File('${dir.path}/${_safe(key)}.dat');
    try {
      if (await f.exists() && await f.length() > 64) return f.path;
    } catch (_) {}

    final url = taskId.isNotEmpty
        ? '${ApiService.baseUrl}/api/tasks/$taskId/cover'
        : ApiService.coverSrc(coverUrl);
    if (url.isEmpty) return null;

    try {
      final req = http.Request('GET', Uri.parse(url));
      final tok = ApiService.authToken;
      if (tok != null && tok.isNotEmpty) {
        req.headers['Authorization'] = 'Bearer $tok';
      }
      final resp =
          await http.Client().send(req).timeout(const Duration(seconds: 15));
      if (resp.statusCode != 200) return null;
      final bytes = await resp.stream.toBytes();
      if (bytes.length < 64) return null;
      await f.writeAsBytes(bytes, flush: true);
      return f.path;
    } catch (_) {
      return null;
    }
  }
}
