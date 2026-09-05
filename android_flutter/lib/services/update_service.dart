import 'dart:convert';
import 'dart:io';
import 'package:dio/dio.dart';
import 'package:path_provider/path_provider.dart';
import 'native_bridge.dart';

/// 移动版检查更新：GitHub Releases，只认 tag 前缀 app-v 的 release
class UpdateService {
  static const _repo = 'Star-wsc/ccnew-vdl';
  static const _tagPrefix = 'app-v';

  static String? _latestVersion;
  static String? _latestNotes;
  static String? _apkUrl;
  static String? _downloadedPath;

  static String? get latestVersion => _latestVersion;
  static String? get latestNotes => _latestNotes;
  static String? get downloadedPath => _downloadedPath;

  /// 比较版本号 a > b 返回 true（按数字段比较）
  static bool _isNewer(String a, String b) {
    final pa = a.split(RegExp(r'[.\-+]')).map((s) => int.tryParse(s) ?? 0).toList();
    final pb = b.split(RegExp(r'[.\-+]')).map((s) => int.tryParse(s) ?? 0).toList();
    for (var i = 0; i < 3; i++) {
      final x = i < pa.length ? pa[i] : 0;
      final y = i < pb.length ? pb[i] : 0;
      if (x != y) return x > y;
    }
    return false;
  }

  /// 检查更新
  static Future<Map<String, dynamic>> checkUpdate() async {
    final appInfo = await NativeBridge.getAppVersion();
    final current = appInfo['version'] ?? '0.0.0';

    final resp = await Dio(BaseOptions(connectTimeout: const Duration(seconds: 10)))
        .get('https://api.github.com/repos/$_repo/releases',
            options: Options(headers: {'Accept': 'application/vnd.github+json'}));
    final releases = resp.data as List<dynamic>;
    Map<String, dynamic>? target;
    for (final r in releases) {
      final tag = (r['tag_name'] ?? '').toString();
      if (!tag.startsWith(_tagPrefix)) continue;
      if (target == null) { target = r; continue; }
      final t1 = tag.substring(_tagPrefix.length);
      final t2 = (target['tag_name'] as String).substring(_tagPrefix.length);
      if (_isNewer(t1, t2)) target = r;
    }
    if (target == null) return {'hasUpdate': false, 'reason': '仓库还没有移动版发布'};

    final latest = (target['tag_name'] as String).substring(_tagPrefix.length);
    String? apkUrl;
    for (final a in (target['assets'] as List<dynamic>? ?? [])) {
      final name = (a['name'] ?? '').toString().toLowerCase();
      if (name.endsWith('.apk')) { apkUrl = a['browser_download_url']; break; }
    }

    _latestVersion = latest;
    _latestNotes = (target['body'] ?? '').toString();
    _apkUrl = apkUrl;
    _downloadedPath = null;

    return {
      'hasUpdate': _isNewer(latest, current),
      'version': latest,
      'current': current,
      'notes': _latestNotes,
      'apkUrl': apkUrl,
    };
  }

  /// 下载新版APK，返回本地路径
  static Future<String?> downloadApk({void Function(double progress)? onProgress}) async {
    if (_apkUrl == null || _latestVersion == null) return null;
    final dir = await getTemporaryDirectory();
    final apkDir = Directory('${dir.path}/apks');
    if (!await apkDir.exists()) await apkDir.create(recursive: true);
    final path = '${apkDir.path}/DouBi-$_latestVersion.apk';

    await Dio().download(_apkUrl!, path, onReceiveProgress: (r, t) {
      if (t > 0) onProgress?.call(r / t);
    });
    _downloadedPath = path;
    return path;
  }

  /// 触发系统安装器
  static Future<bool> install(String path) => NativeBridge.installApk(path);
}
