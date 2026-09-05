import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';
import '../widgets/glass.dart';
import '../services/api_service.dart';

const _bg = Color(0xFF050810);

class PlayerPage extends StatefulWidget {
  final String taskId;
  final String title;
  final String? localPath; // 本地缓存/content URI（可选）
  final String? directUrl; // 直连播放地址（合集视频等，优先级最高）
  const PlayerPage({super.key, required this.taskId, required this.title, this.localPath, this.directUrl});
  @override
  State<PlayerPage> createState() => _PlayerPageState();
}

class _PlayerPageState extends State<PlayerPage> {
  late VideoPlayerController _controller;
  bool _initialized = false;
  bool _failed = false;
  bool _showControls = true;

  @override
  void initState() {
    super.initState();
    // 优先直连URL，其次本地缓存/content URI，最后服务器任务接口
    String url;
    if (widget.directUrl != null) {
      url = widget.directUrl!;
    } else if (widget.localPath != null) {
      final p = widget.localPath!;
      // content:// 或 file:// 开头的直接用，否则加 file:// 前缀
      url = (p.startsWith('content://') || p.startsWith('file://'))
          ? p
          : 'file://$p';
    } else {
      url = '${ApiService.baseUrl}/api/tasks/${widget.taskId}/download';
    }
    final uri = Uri.parse(url);
    // content:// URI 用 contentUri 方式初始化，其他用 networkUrl
    _controller = uri.scheme == 'content'
        ? VideoPlayerController.contentUri(uri)
        : VideoPlayerController.networkUrl(uri)
      ..initialize().then((_) {
        if (mounted) setState(() => _initialized = true);
        _controller.play();
      }).catchError((e) {
        debugPrint('播放器初始化失败: $e');
        if (mounted) setState(() { _initialized = false; _failed = true; });
      });
    _controller.addListener(() { if (mounted) setState(() {}); });
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  String _fmt(Duration d) {
    final m = d.inMinutes.remainder(60).toString().padLeft(2, '0');
    final s = d.inSeconds.remainder(60).toString().padLeft(2, '0');
    return d.inHours > 0 ? '${d.inHours}:$m:$s' : '$m:$s';
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(gradient: RadialGradient(
        center: Alignment(0, -0.4), radius: 1.8,
        colors: [Color(0xFF0F2027), _bg])),
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: GlassAppBar(
          leading: IconButton(
            icon: const Icon(Icons.arrow_back_ios_rounded, color: Color(0xFF94A3B8), size: 20),
            onPressed: () => Navigator.pop(context),
          ),
          title: Text(widget.title, maxLines: 1, overflow: TextOverflow.ellipsis),
        ),
        body: GestureDetector(
          onTap: () => setState(() => _showControls = !_showControls),
          child: Stack(children: [
            Center(
              child: _initialized
                ? AspectRatio(aspectRatio: _controller.value.aspectRatio, child: VideoPlayer(_controller))
                : const Column(mainAxisSize: MainAxisSize.min, children: [
                    CircularProgressIndicator(color: Color(0xFF6EE7B7)),
                    SizedBox(height: 16),
                    Text('正在加载...', style: TextStyle(color: Color(0xFF94A3B8))),
                  ]),
            ),
            if (_failed)
              const Center(child: Text('无法播放此视频', style: TextStyle(color: Color(0xFF94A3B8), fontSize: 14))),
            if (_showControls && _initialized)
              Positioned(bottom: 0, left: 0, right: 0,
                child: Container(
                  padding: EdgeInsets.fromLTRB(16, 8, 16, MediaQuery.of(context).padding.bottom + 12),
                  decoration: BoxDecoration(
                    gradient: LinearGradient(begin: Alignment.bottomCenter, end: Alignment.topCenter,
                      colors: [Colors.black.withOpacity(0.8), Colors.transparent]),
                  ),
                  child: Column(children: [
                    _Seekbar(controller: _controller),
                    const SizedBox(height: 4),
                    Row(children: [
                      Text(_fmt(_controller.value.position), style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 12)),
                      const Spacer(),
                      Text(_fmt(_controller.value.duration), style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 12)),
                    ]),
                    Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                      IconButton(
                        icon: Icon(_controller.value.isPlaying ? Icons.pause_circle_filled : Icons.play_circle_fill,
                          color: const Color(0xFF6EE7B7), size: 48),
                        onPressed: () => setState(() {
                          _controller.value.isPlaying ? _controller.pause() : _controller.play();
                        }),
                      ),
                    ]),
                  ]),
                ),
              ),
          ]),
        ),
      ),
    );
  }
}

/// 自定义可拖进度条：支持点按跳转和拖动粗调，拖动中暂停跟随、松手定位
class _Seekbar extends StatefulWidget {
  final VideoPlayerController controller;
  const _Seekbar({required this.controller});
  @override
  State<_Seekbar> createState() => _SeekbarState();
}

class _SeekbarState extends State<_Seekbar> {
  double? _dragValue; // 拖动中的临时进度(0-1)，null=跟随播放

  double _positionFraction() {
    final dur = widget.controller.value.duration.inMilliseconds;
    if (dur <= 0) return 0;
    return (_dragValue ?? widget.controller.value.position.inMilliseconds / dur).clamp(0.0, 1.0);
  }

  void _seekTo(double fraction) {
    final dur = widget.controller.value.duration.inMilliseconds;
    if (dur <= 0) return;
    widget.controller.seekTo(Duration(milliseconds: (fraction * dur).round()));
  }

  @override
  Widget build(BuildContext context) {
    final played = Color(0xFF6EE7B7);
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      // 点按直接跳转
      onTapUp: (d) {
        final box = context.findRenderObject() as RenderBox;
        final f = (d.localPosition.dx / box.size.width).clamp(0.0, 1.0);
        _seekTo(f);
      },
      // 拖动粗调（拖动中不频繁seek，松手才定位）
      onHorizontalDragStart: (_) => setState(() => _dragValue = _positionFraction()),
      onHorizontalDragUpdate: (d) {
        final box = context.findRenderObject() as RenderBox;
        setState(() {
          _dragValue = ((_dragValue ?? 0) + d.delta.dx / box.size.width).clamp(0.0, 1.0);
        });
      },
      onHorizontalDragEnd: (_) {
        if (_dragValue != null) _seekTo(_dragValue!);
        setState(() => _dragValue = null);
      },
      child: Container(
        // 扩大触摸热区
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(children: [
          Stack(alignment: Alignment.centerLeft, children: [
            // 底轨
            Container(height: 4, decoration: BoxDecoration(
              color: const Color(0xFF1E293B), borderRadius: BorderRadius.circular(2))),
            // 已播放
            LayoutBuilder(builder: (ctx, box) {
              final w = box.maxWidth * _positionFraction();
              return Container(height: 4, width: w,
                decoration: BoxDecoration(color: played, borderRadius: BorderRadius.circular(2)));
            }),
            // 拖柄（拖动中放大）
            LayoutBuilder(builder: (ctx, box) {
              final w = box.maxWidth * _positionFraction();
              final dragging = _dragValue != null;
              return Transform.translate(
                offset: Offset(w - (dragging ? 9 : 7), 0),
                child: Container(width: dragging ? 18 : 14, height: dragging ? 18 : 14,
                  decoration: BoxDecoration(color: played, shape: BoxShape.circle,
                    boxShadow: [BoxShadow(color: played.withOpacity(0.4), blurRadius: dragging ? 10 : 6)])),
              );
            }),
          ]),
        ]),
      ),
    );
  }
}
