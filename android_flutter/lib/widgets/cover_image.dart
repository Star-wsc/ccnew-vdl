import 'dart:io';

import 'package:flutter/material.dart';

import '../services/api_service.dart';
import '../services/cover_cache.dart';

/// 封面：优先 App 私有目录本地缓存，失败再网络；不写系统相册。
class CoverImage extends StatelessWidget {
  final String taskId;
  final String coverUrl;
  final double? width;
  final double? height;
  final Widget? placeholder;

  const CoverImage({
    super.key,
    required this.taskId,
    required this.coverUrl,
    this.width,
    this.height,
    this.placeholder,
  });

  @override
  Widget build(BuildContext context) {
    final ph = placeholder ??
        Container(width: width, height: height, color: const Color(0x0DFFFFFF));
    if (taskId.isEmpty && coverUrl.isEmpty) return ph;
    return FutureBuilder<String?>(
      future: CoverCache.localPath(taskId, coverUrl),
      builder: (ctx, snap) {
        final path = snap.data;
        if (path != null && path.isNotEmpty) {
          return Image.file(
            File(path),
            width: width,
            height: height,
            fit: BoxFit.cover,
            errorBuilder: (_, __, ___) => ph,
          );
        }
        if (snap.connectionState == ConnectionState.waiting) return ph;
        if (coverUrl.isEmpty) return ph;
        return Image.network(
          ApiService.coverSrc(coverUrl),
          width: width,
          height: height,
          fit: BoxFit.cover,
          filterQuality: FilterQuality.low,
          errorBuilder: (_, __, ___) => ph,
        );
      },
    );
  }
}
