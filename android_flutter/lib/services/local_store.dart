import 'dart:convert';
import 'dart:io';
import 'package:path_provider/path_provider.dart';

/// 本地缓存：任务/合集列表落盘，启动秒开，后台再与服务器对账。
class LocalStore {
  static String? _dir;
  static final Map<String, String> _lastWritten = {};

  static Future<File> _file(String name) async {
    _dir ??= (await getApplicationSupportDirectory()).path;
    return File('$_dir/$name.json');
  }

  static Future<List<dynamic>> load(String name) async {
    try {
      final f = await _file(name);
      if (!await f.exists()) return [];
      final data = jsonDecode(await f.readAsString());
      return data as List<dynamic>? ?? [];
    } catch (_) {
      return [];
    }
  }

  /// 内容变化才写盘，避免每2秒轮询都产生IO
  static Future<void> save(String name, List<dynamic> data) async {
    try {
      final encoded = jsonEncode(data);
      if (_lastWritten[name] == encoded) return;
      _lastWritten[name] = encoded;
      final f = await _file(name);
      await f.parent.create(recursive: true);
      await f.writeAsString(encoded, flush: true);
    } catch (_) {}
  }

  static Future<List<dynamic>> loadTasks() => load('tasks');
  static Future<List<dynamic>> loadCollections() => load('collections');
  static Future<void> saveTasks(List<dynamic> t) => save('tasks', t);
  static Future<void> saveCollections(List<dynamic> c) => save('collections', c);

  static Future<Map<String, dynamic>> loadStats() async {
    try {
      final f = await _file('stats');
      if (!await f.exists()) return {};
      return jsonDecode(await f.readAsString()) as Map<String, dynamic>? ?? {};
    } catch (_) {
      return {};
    }
  }

  static Future<void> saveStats(Map<String, dynamic> s) async {
    try {
      final encoded = jsonEncode(s);
      if (_lastWritten['stats'] == encoded) return;
      _lastWritten['stats'] = encoded;
      final f = await _file('stats');
      await f.parent.create(recursive: true);
      await f.writeAsString(encoded, flush: true);
    } catch (_) {}
  }
}
