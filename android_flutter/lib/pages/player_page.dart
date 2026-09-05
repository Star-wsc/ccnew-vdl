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
                    VideoProgressIndicator(_controller, allowScrubbing: true,
                      colors: const VideoProgressColors(
                        playedColor: Color(0xFF6EE7B7),
                        bufferedColor: Color(0xFF1E3A5F),
                        backgroundColor: Color(0xFF1E293B))),
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
