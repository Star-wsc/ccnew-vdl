import 'dart:io';

import 'package:flutter/material.dart';

import '../services/api_service.dart';
import '../services/cover_cache.dart';

/// 灏侀潰缁勪欢锛氫紭鍏?App 绉佹湁鏈湴缂撳瓨锛屽啀閫€鍥炵綉缁滐紱涓嶅啓绯荤粺鐩稿唽銆?class CoverImage extends StatelessWidget {
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
        Container(
          width: width,
          height: height,
          color: const Color(0x0DFFFFFF),
        );
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
        // 鏃犳湰鍦扮紦瀛樻椂鍐嶈蛋缃戠粶锛堜粛缁忔湇鍔″櫒浠ｇ悊锛屼笉鍐欑浉鍐岋級
        if (coverUrl.isEmpty) return ph;
        return Image.network(
          ApiService.coverSrc(coverUrl),
          width: width,
          height: height,
          fit: BoxFit.cover,
          filterQuality: FilterQuality.low,
          cacheWidth: width != null ? (width! * 2).round() : null,
          errorBuilder: (_, __, ___) => ph,
        );
      },
    );
  }
}
