import 'dart:ui';
import 'package:flutter/material.dart';
import '../services/api_service.dart';
import '../services/theme_provider.dart';

/// 预览弹窗：先试合集解析，再试单视频，展示完整预览信息
class PreviewDialog extends StatefulWidget {
  final String url;
  final ThemeProvider theme;
  const PreviewDialog({super.key, required this.url, required this.theme});
  @override
  State<PreviewDialog> createState() => _PreviewDialogState();
}

class _PreviewDialogState extends State<PreviewDialog> {
  bool _loading = true;
  String? _error;
  Map<String, dynamic>? _videoPreview;
  Map<String, dynamic>? _collectionPreview;
  bool _isCollection = false;
  final Set<int> _selectedIndices = {};
  String _selectedQuality = ''; // 用户选中的清晰度

  @override
  void initState() {
    super.initState();
    _parse();
  }

  Future<void> _parse() async {
    debugPrint('[PreviewDialog] 开始解析: ${widget.url}');
    // 先尝试合集解析
    final col = await ApiService.previewCollection(widget.url);
    debugPrint('[PreviewDialog] 合集结果: ${col != null ? "有数据" : "null"}');
    if (col != null) {
      final videos = col['videos'] as List<dynamic>? ?? [];
      debugPrint('[PreviewDialog] 合集视频数: ${videos.length}');
      if (videos.isNotEmpty) {
        setState(() {
          _collectionPreview = col;
          _isCollection = true;
          _loading = false;
          _selectedIndices.addAll(List.generate(videos.length, (i) => i));
        });
        return;
      }
    }
    // 再试单视频
    final preview = await ApiService.previewUrl(widget.url);
    debugPrint('[PreviewDialog] 单视频结果: ${preview != null ? "有数据" : "null"}');
    if (preview != null) debugPrint('[PreviewDialog] title: ${preview['title']}');
    if (preview != null && (preview['title'] as String? ?? '').isNotEmpty) {
      setState(() { _videoPreview = preview; _loading = false; });
      return;
    }
    setState(() { _error = '解析失败，请检查链接'; _loading = false; });
  }

  @override
  Widget build(BuildContext context) {
    final t = widget.theme;
    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.all(16),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
          child: Container(
            constraints: BoxConstraints(maxHeight: MediaQuery.of(context).size.height * 0.85),
            decoration: BoxDecoration(
              color: t.bg.withOpacity(0.95),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(color: t.border, width: 0.5),
            ),
            child: _loading
              ? Padding(padding: const EdgeInsets.all(40),
                  child: Column(mainAxisSize: MainAxisSize.min, children: [
                    const CircularProgressIndicator(),
                    const SizedBox(height: 16),
                    Text('正在解析...', style: TextStyle(color: t.textSecondary, fontSize: 13)),
                  ]))
              : _error != null
                ? _buildError(t)
                : _isCollection
                  ? _buildCollection(t)
                  : _buildVideo(t),
          ),
        ),
      ),
    );
  }

  Widget _buildError(ThemeProvider t) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        const Icon(Icons.error_outline_rounded, color: Color(0xFFFF6B6B), size: 48),
        const SizedBox(height: 12),
        Text(_error!, style: TextStyle(color: t.textPrimary, fontSize: 14)),
        const SizedBox(height: 16),
        TextButton(onPressed: () => Navigator.pop(context),
          child: Text('关闭', style: TextStyle(color: t.textSecondary))),
      ]),
    );
  }

  // ===== 单视频预览 =====
  Widget _buildVideo(ThemeProvider t) {
    final title = _videoPreview?['title'] ?? '';
    final author = _videoPreview?['author'] ?? '';
    final cover = _videoPreview?['cover_url'] ?? '';
    final platform = _videoPreview?['platform'] ?? '';
    final quality = _videoPreview?['quality'] ?? '';

    return SingleChildScrollView(
      padding: const EdgeInsets.all(20),
      child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
        // 关闭按钮
        Align(alignment: Alignment.topRight,
          child: GestureDetector(
            onTap: () => Navigator.pop(context),
            child: Icon(Icons.close_rounded, color: t.textSecondary, size: 22),
          )),
        const SizedBox(height: 8),
        // 封面大图
        if (cover.isNotEmpty) ...[
          ClipRRect(borderRadius: BorderRadius.circular(14),
            child: Image.network(ApiService.coverSrc(cover), width: double.infinity, height: 200, fit: BoxFit.cover,
              errorBuilder: (_, __, ___) => Container(width: double.infinity, height: 200,
                color: t.glass, child: Icon(Icons.image_rounded, color: t.textDim, size: 48)))),
          const SizedBox(height: 16),
        ],
        // 平台 + 画质
        Row(children: [
          if (platform.isNotEmpty) _platformBadge(platform),
          if (quality.isNotEmpty) ...[const SizedBox(width: 8), _tag(quality, const Color(0xFFC4B5FD))],
        ]),
        const SizedBox(height: 10),
        // 清晰度选择
        if (platform == 'bilibili' || platform == 'douyin' || platform == 'youtube') ...[
          Wrap(spacing: 8, runSpacing: 6, children: [
            for (final q in ['4k', '2k', '1080p', '720p', '480p'])
              _qualityChip(q, t),
          ]),
          const SizedBox(height: 12),
        ],
        // 标题
        Text(title, style: TextStyle(color: t.textPrimary, fontSize: 16, fontWeight: FontWeight.w600, height: 1.4)),
        if (author.isNotEmpty) ...[
          const SizedBox(height: 6),
          Text(author, style: TextStyle(color: t.textSecondary, fontSize: 13)),
        ],
        const SizedBox(height: 24),
        // 下载按钮
        SizedBox(width: double.infinity, height: 48,
          child: _gradientBtn('添加到下载', Icons.download_rounded, t.primary, () {
            final data = Map<String, dynamic>.from(_videoPreview!);
            if (_selectedQuality.isNotEmpty) data['quality'] = _selectedQuality;
            Navigator.pop(context, {'action': 'download', 'preview': data});
          })),
      ]),
    );
  }

  // ===== 合集预览 =====
  Widget _buildCollection(ThemeProvider t) {
    final title = _collectionPreview?['title'] ?? '';
    final videos = _collectionPreview?['videos'] as List<dynamic>? ?? [];
    final platform = _collectionPreview?['platform'] ?? '';
    final cover = _collectionPreview?['cover_url'] ?? '';

    return Column(mainAxisSize: MainAxisSize.min, children: [
      // 顶部信息
      Padding(padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            _platformBadge(platform),
            const Spacer(),
            GestureDetector(onTap: () => Navigator.pop(context),
              child: Icon(Icons.close_rounded, color: t.textSecondary, size: 22)),
          ]),
          const SizedBox(height: 10),
          if (cover.isNotEmpty) ...[
            ClipRRect(borderRadius: BorderRadius.circular(12),
              child: Image.network(ApiService.coverSrc(cover), width: double.infinity, height: 120, fit: BoxFit.cover,
                errorBuilder: (_, __, ___) => const SizedBox.shrink())),
            const SizedBox(height: 10),
          ],
          Text(title, style: TextStyle(color: t.textPrimary, fontSize: 16, fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          // 全选/已选
          Row(children: [
            Text('${_selectedIndices.length}/${videos.length} 已选',
              style: TextStyle(color: t.textSecondary, fontSize: 12)),
            const Spacer(),
            GestureDetector(
              onTap: () => setState(() {
                if (_selectedIndices.length == videos.length) _selectedIndices.clear();
                else _selectedIndices.addAll(List.generate(videos.length, (i) => i));
              }),
              child: Text(_selectedIndices.length == videos.length ? '取消全选' : '全选',
                style: TextStyle(color: t.primary, fontSize: 13, fontWeight: FontWeight.w600)),
            ),
          ]),
        ])),
      const SizedBox(height: 8),
      // 视频列表
      Flexible(child: ListView.builder(
        shrinkWrap: true,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        itemCount: videos.length,
        itemBuilder: (_, i) => _collectionItem(t, videos[i], i),
      )),
      // 下载按钮
      Padding(padding: const EdgeInsets.all(16),
        child: SizedBox(width: double.infinity, height: 48,
          child: _gradientBtn(
            '下载选中 (${_selectedIndices.length})',
            Icons.download_rounded,
            t.primary,
            _selectedIndices.isEmpty ? null : () {
              Navigator.pop(context, {
                'action': 'collection',
                'collection': _collectionPreview,
                'selectedIndices': _selectedIndices.toList(),
              });
            },
          ))),
    ]);
  }

  Widget _collectionItem(ThemeProvider t, dynamic video, int index) {
    final selected = _selectedIndices.contains(index);
    final cover = video['cover_url'] ?? '';
    final title = video['title'] ?? '';
    final author = video['author'] ?? '';

    return GestureDetector(
      onTap: () => setState(() {
        if (selected) _selectedIndices.remove(index); else _selectedIndices.add(index);
      }),
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: selected ? t.primary.withOpacity(0.12) : t.surface.withOpacity(0.5),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected ? t.primary.withOpacity(0.5) : t.border,
            width: selected ? 1.0 : 0.5,
          ),
        ),
        child: Row(children: [
          Checkbox(
            value: selected,
            onChanged: (v) => setState(() {
              if (v == true) _selectedIndices.add(index); else _selectedIndices.remove(index);
            }),
            activeColor: t.primary,
            side: BorderSide(color: t.textDim),
          ),
          if (cover.isNotEmpty) ...[
            ClipRRect(borderRadius: BorderRadius.circular(8),
              child: Image.network(ApiService.coverSrc(cover), width: 52, height: 52, fit: BoxFit.cover,
                errorBuilder: (_, __, ___) => Container(width: 52, height: 52, color: t.glass))),
            const SizedBox(width: 10),
          ],
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(title, style: TextStyle(color: t.textPrimary, fontSize: 13, fontWeight: FontWeight.w500),
              maxLines: 2, overflow: TextOverflow.ellipsis),
            if (author.isNotEmpty) ...[
              const SizedBox(height: 2),
              Text(author, style: TextStyle(color: t.textSecondary, fontSize: 11)),
            ],
          ])),
        ]),
      ),
    );
  }

  Widget _platformBadge(String platform) {
    final isBili = platform == 'bilibili';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: isBili ? const Color(0xFFFF6B9D) : Colors.black87,
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(isBili ? 'B站' : '抖音',
        style: const TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.bold)),
    );
  }

  Widget _tag(String text, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.15),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: color.withOpacity(0.25), width: 0.5),
      ),
      child: Text(text, style: TextStyle(color: color, fontSize: 10, fontWeight: FontWeight.w600)),
    );
  }

  static const _qualities = ['4k', '2k', '1080p', '720p', '480p'];

  Widget _qualityChip(String q, ThemeProvider t) {
    final selected = _selectedQuality == q ||
        (_selectedQuality.isEmpty && q == (_videoPreview?['quality'] ?? '').toString().toLowerCase());
    return GestureDetector(
      onTap: () => setState(() => _selectedQuality = q),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
        decoration: BoxDecoration(
          color: selected ? t.primary.withOpacity(0.2) : t.glass,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: selected ? t.primary : t.border, width: selected ? 1.5 : 0.5),
        ),
        child: Text(q.toUpperCase(),
          style: TextStyle(color: selected ? t.primary : t.textSecondary, fontSize: 11, fontWeight: FontWeight.w700)),
      ),
    );
  }

  Widget _gradientBtn(String label, IconData icon, Color color, VoidCallback? onPressed) {
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(colors: [color, color.withOpacity(0.8)]),
        borderRadius: BorderRadius.circular(14),
        boxShadow: [BoxShadow(color: color.withOpacity(0.35), blurRadius: 16, offset: const Offset(0, 6))],
      ),
      child: ElevatedButton.icon(
        onPressed: onPressed,
        icon: Icon(icon, size: 18, color: const Color(0xFF0D0D0D)),
        label: Text(label, style: const TextStyle(fontWeight: FontWeight.bold, color: Color(0xFF0D0D0D), fontSize: 14)),
        style: ElevatedButton.styleFrom(
          backgroundColor: Colors.transparent, shadowColor: Colors.transparent,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
        ),
      ),
    );
  }
}
