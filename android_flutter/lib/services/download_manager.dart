import 'dart:io';
import 'package:dio/dio.dart';
import 'package:path_provider/path_provider.dart';
import 'api_service.dart';

class DownloadManager {
  static final DownloadManager _instance = DownloadManager._();
  factory DownloadManager() => _instance;
  DownloadManager._();

  final Dio _dio = Dio(BaseOptions(
    connectTimeout: const Duration(seconds: 10),
    receiveTimeout: const Duration(minutes: 30),
  ));
  final Map<String, CancelToken> _active = {};

  /// 从服务器下载视频到本地临时文件，返回本地路径
  Future<String?> downloadToLocal(String taskId, String fileName,
      {Function(double progress, int speed)? onProgress}) async {
    if (_active.containsKey(taskId)) return null;

    final tempDir = await getTemporaryDirectory();
    final localPath = '${tempDir.path}/$fileName';
    final cancelToken = CancelToken();
    _active[taskId] = cancelToken;

    try {
      int lastBytes = 0;
      DateTime lastTime = DateTime.now();

      // 带登录 token（鉴权开启时无 token 会 401）
      final url = ApiService.downloadUrl(taskId);

      final headers = <String, String>{'Accept': 'video/mp4,*/*'};
      final tok = ApiService.authToken;
      if (tok != null && tok.isNotEmpty) {
        headers['Authorization'] = 'Bearer $tok';
      }

      await _dio.download(
        url,
        localPath,
        cancelToken: cancelToken,
        options: Options(headers: headers),
        onReceiveProgress: (received, total) {
          if (total <= 0) return;
          final now = DateTime.now();
          final elapsed = now.difference(lastTime).inMilliseconds / 1000.0;
          if (elapsed >= 0.5) {
            final speed = ((received - lastBytes) / elapsed).round();
            lastBytes = received;
            lastTime = now;
            onProgress?.call(received / total, speed);
          }
        },
      );

      _active.remove(taskId);

      // 验证文件
      final file = File(localPath);
      if (await file.exists() && await file.length() > 0) {
        return localPath;
      }
      return null;
    } catch (e) {
      _active.remove(taskId);
      return null;
    }
  }

  void cancel(String taskId) {
    _active[taskId]?.cancel('用户取消');
    _active.remove(taskId);
  }

  void cancelAll() {
    for (final t in _active.values) { t.cancel('应用退出'); }
    _active.clear();
  }
}
