import 'dart:async';
import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../widgets/glass.dart';
import '../services/api_service.dart';
import '../services/download_manager.dart';
import '../services/local_store.dart';
import '../services/native_bridge.dart';
import '../services/theme_provider.dart';
import '../widgets/preview_dialog.dart';
import 'settings_page.dart';
import 'player_page.dart';

// 主题感知的颜色扩展
extension ThemeColors on BuildContext {
  ThemeProvider get tp => read<ThemeProvider>();
  Color get green => tp.primary;
  Color get blue => tp.secondary;
  Color get red => tp.error;
  Color get purple => const Color(0xFFC4B5FD);
  Color get bg => tp.bg;
}

class HomePage extends StatefulWidget {
  const HomePage({super.key});
  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> with WidgetsBindingObserver {
  int _tab = 0;
  List<dynamic> _tasks = [];
  List<dynamic> _collections = [];
  String _logs = '';
  Map<String, dynamic> _stats = {};
  final _urlCtrl = TextEditingController();
  bool _connected = false;
  String? _error;
  final Map<String, double> _dlProgress = {};
  final Map<String, int> _dlSpeed = {};
  Timer? _timer;

  // 主题色（build 里更新）
  late Color green, blue, red, bg, text1, text2, text3, border;
  static const purple = Color(0xFFC4B5FD);

  // 多选删除
  bool _multiSelect = false;
  final Set<String> _selected = {};
  // 已保存到相册的任务
  final Set<String> _savedToGallery = {};
  // 本地缓存路径（用于播放）
  final Map<String, String> _localCache = {};
  // 相册 content URI（保存后用于播放）
  final Map<String, String> _galleryUri = {};

  /// 获取最佳播放路径：优先 content URI，其次本地临时文件，最后服务器 URL
  String? _playbackUrl(String id) => _galleryUri[id] ?? _localCache[id];

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _loadCache(); // 先用手机缓存秒开列表
    _refresh(force: true); // 再与服务器对账
    _armStandingTimer();
    NativeBridge.setOnShareCallback((url) { _urlCtrl.text = url; _submit(); });
    NativeBridge.getPendingShare().then((url) { if (url != null && url.isNotEmpty) { _urlCtrl.text = url; _submit(); } });
  }

  /// 启动时从本地缓存加载列表，无网也能看到上次的内容
  Future<void> _loadCache() async {
    final tasks = await LocalStore.loadTasks();
    final cols = await LocalStore.loadCollections();
    final stats = await LocalStore.loadStats();
    if (!mounted) return;
    setState(() {
      if (_tasks.isEmpty && tasks.isNotEmpty) _tasks = tasks;
      if (_collections.isEmpty && cols.isNotEmpty) _collections = cols;
      if (stats.isNotEmpty) _stats = stats;
    });
  }

  // ===== 数据刷新策略：本地为准 + 事件驱动 + 低频兜底 =====
  // - 列表读手机缓存，秒开
  // - 切换菜单 → 刷新当前菜单（受1分钟CD约束）
  // - 页内操作(创建/删除/下载等) → 立即刷新（绕过CD）
  // - 常驻兜底轮询：5分钟一次
  // - 后台完全停止
  bool _foreground = true;
  String _netType = 'wifi';
  DateTime _lastRefresh = DateTime.fromMillisecondsSinceEpoch(0);
  static const _refreshCD = Duration(minutes: 1);
  static const _standingInterval = Duration(minutes: 5);

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      if (!_foreground) {
        _foreground = true;
        _refresh(force: true); // 回前台立即刷新一次
        _armStandingTimer();
      }
    } else if (state == AppLifecycleState.paused || state == AppLifecycleState.detached) {
      _foreground = false;
      _timer?.cancel();
    }
  }

  /// 常驻兜底轮询：5分钟一次，仅前台运行
  void _armStandingTimer() {
    _timer?.cancel();
    if (!_foreground || !mounted) return;
    _timer = Timer(_standingInterval, () async {
      await _refresh();
      _armStandingTimer();
    });
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _timer?.cancel();
    _urlCtrl.dispose();
    super.dispose();
  }

  /// 拉取服务器数据。
  /// force=true: 用户操作触发，绕过CD
  /// force=false: 切菜单/兜底触发，1分钟内不重复请求
  Future<void> _refresh({bool force = false}) async {
    if (!_foreground || !mounted) return;
    if (!force && DateTime.now().difference(_lastRefresh) < _refreshCD) return;
    _lastRefresh = DateTime.now();

    final net = await NativeBridge.getNetworkType();
    if (mounted) setState(() => _netType = net);
    // 先探测连通性：离线时保持本地缓存内容不动，绝不能用空数据覆盖
    final ok = await ApiService.checkConnection();
    if (!ok) {
      if (mounted) setState(() => _connected = false);
      return;
    }
    try {
      final results = await Future.wait([
        ApiService.getStats(),
        ApiService.getTasks(),
        ApiService.getCollections(),
      ]);
      if (!mounted) return;
      setState(() {
        _stats = results[0] as Map<String, dynamic>;
        _tasks = results[1] as List<dynamic>;
        _collections = results[2] as List<dynamic>;
        _connected = true;
      });
      // 新数据回写本地缓存（内容变化才写盘）
      LocalStore.saveTasks(_tasks);
      LocalStore.saveCollections(_collections);
      LocalStore.saveStats(_stats);
    } catch (_) { if (mounted) setState(() => _connected = false); }
    if (_tab == 2) { final l = await ApiService.getLogs(); if (mounted) setState(() => _logs = l); }
  }

  /// 从分享文字中提取 URL
  String _extractUrl(String text) {
    // 匹配 http/https 链接
    final match = RegExp(r'https?://[\w\-._~:/?#\[\]@!$&'"'"'()*+,;=%]+').firstMatch(text);
    return match?.group(0) ?? text.trim();
  }

  Future<void> _submit() async {
    final raw = _urlCtrl.text.trim();
    if (raw.isEmpty) return;
    final url = _extractUrl(raw);
    debugPrint('[DouBi] 原始输入: $raw');
    debugPrint('[DouBi] 提取URL: $url');
    FocusScope.of(context).unfocus();
    _urlCtrl.clear();

    // 所有链接都弹预览弹窗
    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (_) => PreviewDialog(url: url, theme: Provider.of<ThemeProvider>(context, listen: false)),
    );

    if (result == null) {
      debugPrint('[DouBi] 弹窗返回null（用户关闭或解析失败）');
      return;
    }

    debugPrint('[DouBi] 弹窗返回: action=${result['action']}');
    if (result['action'] == 'download') {
      final preview = result['preview'] as Map<String, dynamic>?;
      if (preview == null) {
        debugPrint('[DouBi] preview数据为null');
        return;
      }
      debugPrint('[DouBi] 创建任务: ${preview['title']}');
      final task = await ApiService.createFromPreview(preview);
      debugPrint('[DouBi] 创建结果: ${task != null ? "成功 ${task['id']}" : "失败"}');
    } else if (result['action'] == 'collection') {
      // 合集：用选中的索引创建
      final col = result['collection'] as Map<String, dynamic>;
      final indices = result['selectedIndices'] as List<dynamic>?;
      await ApiService.createCollection(col, selectedIndices: indices?.cast<int>());
    }
    // 立即刷新，不等下一轮轮询
    _refresh(force: true); // 操作后立即刷新
  }

  Future<void> _downloadToGallery(dynamic task) async {
    final id = task['id'];
    final title = (task['title'] ?? 'video').toString();
    final safe = title.replaceAll(RegExp(r'[/\\:*?"<>|]'), '_');
    final fileName = '${task['platform'] ?? 'v'}_$safe.mp4';
    setState(() { _dlProgress[id] = 0; _dlSpeed[id] = 0; });
    final localPath = await DownloadManager().downloadToLocal(id, fileName,
      onProgress: (p, s) { if (mounted) setState(() { _dlProgress[id] = p; _dlSpeed[id] = s; }); });
    if (localPath == null) { if (mounted) setState(() { _dlProgress.remove(id); _dlSpeed.remove(id); }); return; }
    final contentUri = await NativeBridge.saveToGallery(localPath);
    if (contentUri != null) {
      _savedToGallery.add(id);
      _localCache[id] = localPath;
      _galleryUri[id] = contentUri; // 保存 content:// URI 用于播放
      // 保存成功后删除临时文件（用 content URI 播放，不需要临时文件）
      await NativeBridge.deleteFile(localPath);
      _localCache.remove(id);
      // 根据设置决定是否删除服务端缓存
      if (ApiService.deleteServerAfterSave) {
        await ApiService.deleteTask(id, deleteFile: true);
      }
    } else {
      // 保存失败，保留临时文件用于播放
      _localCache[id] = localPath;
    }
    if (mounted) {
      setState(() { _dlProgress.remove(id); _dlSpeed.remove(id); });
      if (contentUri != null) ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(ApiService.deleteServerAfterSave
            ? '✓ 已保存到相册，服务器缓存已删除'
            : '✓ 已保存到相册'),
          backgroundColor: const Color(0xFF065F46)));
    }
  }

  /// 长按弹出操作菜单
  void _showTaskMenu(dynamic task) {
    final status = task['status'] ?? '';
    final id = task['id'] ?? '';
    final title = task['title'] ?? task['url'] ?? '';
    final filePath = task['file_path'] ?? '';

    showModalBottomSheet(
      context: context,
      backgroundColor: Colors.transparent,
      builder: (_) => ClipRRect(
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 25, sigmaY: 25),
          child: Container(
            padding: EdgeInsets.fromLTRB(20, 16, 20, MediaQuery.of(context).padding.bottom + 16),
            decoration: BoxDecoration(
              color: const Color(0xE60A0E14),
              border: Border(top: BorderSide(color: const Color(0xFFFFFFFF).withOpacity(0.15), width: 0.5)),
            ),
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              // 标题
              Text(title, style: TextStyle(color: text1, fontSize: 15, fontWeight: FontWeight.w600),
                maxLines: 1, overflow: TextOverflow.ellipsis),
              const SizedBox(height: 4),
              Text(_statusText(status), style: TextStyle(color: _statusColor(status), fontSize: 12)),
              const SizedBox(height: 16),
              // 操作按钮
              if (status == 'completed') ...[
                _menuButton(Icons.download_rounded, '下载到相册', green, () {
                  Navigator.pop(context);
                  _downloadToGallery(task);
                }),
                if (filePath.isNotEmpty)
                  _menuButton(Icons.play_arrow_rounded, '播放', blue, () {
                    Navigator.pop(context);
                    Navigator.push(context, MaterialPageRoute(
                      builder: (_) => PlayerPage(taskId: id, title: title, localPath: _playbackUrl(id))));
                  }),
              ],
              if (status == 'failed')
                _menuButton(Icons.refresh_rounded, '重新下载', blue, () {
                  Navigator.pop(context);
                  ApiService.retryTask(id);
                }),
              _menuButton(Icons.delete_rounded, '删除任务', red, () {
                Navigator.pop(context);
                ApiService.deleteTask(id, deleteFile: true);
              }),
              const SizedBox(height: 8),
              TextButton(
                onPressed: () => Navigator.pop(context),
                child: Text('取消', style: TextStyle(color: Color(0xFF64748B))),
              ),
            ]),
          ),
        ),
      ),
    );
  }

  /// 删除确认弹窗
  Future<void> _confirmDelete(String id, String title, {bool deleteFile = true}) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1F2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text('确认删除', style: TextStyle(color: text1, fontSize: 18)),
        content: Text('确定要删除「$title」吗？\n已保存在相册内的视频不会被删除。${ApiService.deleteServerAfterSave ? "\n服务器上的文件也会被删除。" : ""}',
          style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 14)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text('取消', style: TextStyle(color: Color(0xFF64748B))),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text('删除', style: TextStyle(color: Color(0xFFFCA5A5), fontWeight: FontWeight.bold)),
          ),
        ],
      ),
    );
    if (confirmed == true) {
      await ApiService.deleteTask(id, deleteFile: deleteFile);
    }
  }

  /// 批量删除选中任务
  Future<void> _deleteSelected() async {
    if (_selected.isEmpty) return;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1F2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text('删除 ${_selected.length} 个任务', style: TextStyle(color: text1, fontSize: 18)),
        content: Text('确定要删除所有选中的任务吗？\n已保存在相册内的视频不会被删除。${ApiService.deleteServerAfterSave ? "\n服务器上的文件也会被删除。" : ""}',
          style: TextStyle(color: Color(0xFF94A3B8), fontSize: 14)),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false),
            child: Text('取消', style: TextStyle(color: Color(0xFF64748B)))),
          TextButton(onPressed: () => Navigator.pop(ctx, true),
            child: Text('全部删除', style: TextStyle(color: Color(0xFFFCA5A5), fontWeight: FontWeight.bold))),
        ],
      ),
    );
    if (confirmed == true) {
      for (final id in _selected.toList()) {
        await ApiService.deleteTask(id, deleteFile: true);
      }
      setState(() { _selected.clear(); _multiSelect = false; });
    }
  }

  Widget _menuButton(IconData icon, String label, Color color, VoidCallback onTap) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(14),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
          child: Container(
            decoration: BoxDecoration(
              color: color.withOpacity(0.1),
              borderRadius: BorderRadius.circular(14),
              border: Border.all(color: color.withOpacity(0.25), width: 0.5),
            ),
            child: ListTile(
              leading: Icon(icon, color: color),
              title: Text(label, style: TextStyle(color: color, fontWeight: FontWeight.w600)),
              onTap: onTap,
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
            ),
          ),
        ),
      ),
    );
  }

  String _statusText(String s) => switch (s) {
    'pending' => '等待中', 'parsing' => '解析中', 'downloading' => '服务端下载中',
    'completed' => '已完成', 'failed' => '失败', _ => s,
  };

  Color _statusColor(String s) => switch (s) {
    'completed' => green, 'failed' => red, 'downloading' => blue, _ => text3,
  };

  @override
  Widget build(BuildContext context) {
    final tp = Provider.of<ThemeProvider>(context);
    green = tp.primary;
    blue = tp.secondary;
    red = tp.error;
    bg = tp.bg;
    text1 = tp.textPrimary;
    text2 = tp.textSecondary;
    text3 = tp.textDim;
    border = tp.border;

    return Container(
      decoration: BoxDecoration(
        gradient: RadialGradient(center: const Alignment(0, -0.3), radius: 1.6,
          colors: [bg.withOpacity(0.85), bg]),
      ),
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: GlassAppBar(
          title: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text('DouBi', style: TextStyle(letterSpacing: 3)),
            Row(children: [
              Text(_connected ? '已连接' : '未连接',
                style: TextStyle(color: _connected ? green : red, fontSize: 11)),
              if (_netType == 'cellular') ...[
                const SizedBox(width: 6),
                Container(padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                  decoration: BoxDecoration(color: purple.withOpacity(0.15), borderRadius: BorderRadius.circular(4)),
                  child: Text('省流模式', style: TextStyle(color: purple, fontSize: 9))),
              ],
            ]),
          ]),
          actions: [
            if (_multiSelect) ...[
              // 全选/取消全选
              GlassIconButton(
                icon: _selected.length == _tasks.length ? Icons.deselect_rounded : Icons.select_all_rounded,
                color: green,
                onPressed: () => setState(() {
                  if (_selected.length == _tasks.length) {
                    _selected.clear();
                  } else {
                    _selected.addAll(_tasks.map((t) => t['id'] as String));
                  }
                }),
              ),
              const SizedBox(width: 4),
              // 批量删除
              GlassIconButton(
                icon: Icons.delete_sweep_rounded,
                color: red,
                onPressed: _deleteSelected,
              ),
              const SizedBox(width: 4),
              // 退出多选
              GlassIconButton(
                icon: Icons.close_rounded,
                onPressed: () => setState(() { _multiSelect = false; _selected.clear(); }),
              ),
            ] else ...[
              // 多选模式按钮
              GlassIconButton(icon: Icons.checklist_rounded,
                onPressed: () => setState(() => _multiSelect = true)),
              const SizedBox(width: 4),
              GlassIconButton(icon: Icons.settings_rounded,
                onPressed: () => Navigator.push(context, MaterialPageRoute(builder: (_) => const SettingsPage())).then((_) => _refresh(force: true))),
            ],
            const SizedBox(width: 8),
          ],
        ),
        bottomNavigationBar: GlassBottomNav(
          currentIndex: _tab,
          onTap: (i) { setState(() => _tab = i); _refresh(); }, // 切菜单刷新当前菜单(受CD约束)
          items: const [
            GlassNavItem(icon: Icons.home_rounded, label: '任务'),
            GlassNavItem(icon: Icons.collections_bookmark_rounded, label: '合集'),
            GlassNavItem(icon: Icons.terminal_rounded, label: '日志'),
          ],
        ),
        body: IndexedStack(index: _tab, children: [
          _buildTasks(), _buildCollections(), _buildLogs(),
        ]),
      ),
    );
  }

  // ===== 任务页 =====
  Widget _buildTasks() {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 100),
      children: [
        // 统计栏
        GlassContainer(
          padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 8),
          child: Row(mainAxisAlignment: MainAxisAlignment.spaceEvenly, children: [
            _stat(Icons.all_inbox_rounded, '全部', _stats['total'] ?? 0, text1),
            _stat(Icons.downloading_rounded, '下载中', _stats['downloading'] ?? 0, blue),
            _stat(Icons.check_circle_rounded, '完成', _stats['completed'] ?? 0, green),
            _stat(Icons.error_rounded, '失败', _stats['failed'] ?? 0, red),
          ]),
        ),
        const SizedBox(height: 16),
        // 输入框
        GlassContainer(
          padding: const EdgeInsets.all(14),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text('粘贴链接', style: TextStyle(color: text2, fontSize: 11, letterSpacing: 2, fontWeight: FontWeight.w700)),
            const SizedBox(height: 10),
            Row(children: [
              Expanded(child: GlassTextField(
                controller: _urlCtrl,
                hintText: 'bilibili.com / douyin.com / b23.tv',
                prefixIcon: Icons.link_rounded,
                onSubmitted: (_) => _submit(),
              )),
              const SizedBox(width: 10),
              GlowButton(label: '解析', icon: Icons.arrow_forward_rounded, onPressed: _submit),
            ]),
            if (_error != null) Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(_error!, style: TextStyle(color: red, fontSize: 12)),
            ),
          ]),
        ),
        const SizedBox(height: 20),
        if (_tasks.isEmpty) _emptyState()
        else ...[
          Padding(
            padding: const EdgeInsets.only(bottom: 10, left: 4),
            child: Text('任务 · ${_tasks.length}',
              style: TextStyle(color: text3, fontSize: 12, letterSpacing: 2, fontWeight: FontWeight.w700)),
          ),
          GridView.builder(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
              crossAxisCount: 3,
              crossAxisSpacing: 10,
              mainAxisSpacing: 10,
              childAspectRatio: 0.6,
            ),
            itemCount: _tasks.length,
            itemBuilder: (ctx, i) {
              final t = _tasks[i];
              return GestureDetector(
                onTap: _multiSelect ? () {
                  setState(() {
                    final id = t['id'] as String;
                    if (_selected.contains(id)) _selected.remove(id);
                    else _selected.add(id);
                  });
                } : null,
                child: _taskCardGrid(t),
              );
            },
          ),
        ],
      ],
    );
  }

  Widget _stat(IconData icon, String label, int value, Color color) {
    return Column(children: [
      Icon(icon, color: color.withOpacity(0.5), size: 20),
      const SizedBox(height: 4),
      Text('$value', style: TextStyle(color: color, fontSize: 22, fontWeight: FontWeight.bold)),
      Text(label, style: TextStyle(color: text3, fontSize: 11)),
    ]);
  }

  Widget _taskCard(dynamic task) {
    final status = task['status'] ?? '';
    final id = task['id'] ?? '';
    final title = task['title'] ?? task['url'] ?? '';
    final author = task['author'] ?? '';
    final cover = task['cover_url'] ?? '';
    final platform = task['platform'] ?? '';
    final quality = task['quality'] ?? '';
    final error = task['error_message'] ?? '';
    final isLocalDl = _dlProgress.containsKey(id);

    return GlassContainer(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          // 多选复选框
          if (_multiSelect) ...[
            Checkbox(
              value: _selected.contains(id),
              onChanged: (v) => setState(() {
                if (v == true) _selected.add(id); else _selected.remove(id);
              }),
              activeColor: green,
              checkColor: const Color(0xFF050810),
              side: BorderSide(color: text3),
            ),
            const SizedBox(width: 4),
          ],
          // 封面
          if (cover.isNotEmpty) ...[
            ClipRRect(
              borderRadius: BorderRadius.circular(12),
              child: Stack(children: [
                Image.network(cover, width: 72, height: 72, fit: BoxFit.cover,
                  errorBuilder: (_, __, ___) => Container(width: 72, height: 72, color: const Color(0x0DFFFFFF))),
                if (platform.isNotEmpty)
                  Positioned(bottom: 4, left: 4, child: _tag(
                    platform == 'bilibili' ? 'B站' : '抖音',
                    platform == 'bilibili' ? blue : red)),
              ]),
            ),
            const SizedBox(width: 12),
          ],
          // 标题区
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(title, style: TextStyle(color: text1, fontSize: 14, fontWeight: FontWeight.w600, height: 1.4),
              maxLines: 2, overflow: TextOverflow.ellipsis),
            if (author.isNotEmpty) ...[const SizedBox(height: 2), Text(author, style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 12))],
            const SizedBox(height: 6),
            Row(children: [
              if (quality.isNotEmpty) _tag(quality, purple),
              const SizedBox(width: 6),
              if (_savedToGallery.contains(id)) ...[
                _tag('相册已保存', green),
                const SizedBox(width: 4),
                _tag(ApiService.deleteServerAfterSave ? '服务器已删除' : '服务器已保留', blue),
              ] else
                _statusTag(status),
            ]),
          ])),
          // 三个竖排按钮：下载 / 播放 / 删除
          const SizedBox(width: 8),
          Column(children: [
            if (status == 'completed') ...[
              if (!_savedToGallery.contains(id)) ...[
                _iconBtn(Icons.download_rounded, green, () => _downloadToGallery(task)),
                const SizedBox(height: 6),
              ],
              _iconBtn(Icons.play_arrow_rounded, blue, () =>
                Navigator.push(context, MaterialPageRoute(builder: (_) => PlayerPage(taskId: id, title: task['title'] ?? '', localPath: _playbackUrl(id))))),
            ] else if (status == 'failed') ...[
              _iconBtn(Icons.refresh_rounded, blue, () => ApiService.retryTask(id)),
            ] else if (status == 'downloading' || status == 'parsing') ...[
              SizedBox(width: 32, height: 32,
                child: Stack(alignment: Alignment.center, children: [
                  CircularProgressIndicator(
                    value: isLocalDl ? _dlProgress[id]! : (task['progress'] ?? 0) / 100.0,
                    strokeWidth: 2.5, color: isLocalDl ? green : blue, backgroundColor: const Color(0x1AFFFFFF)),
                  Text('${isLocalDl ? ((_dlProgress[id] ?? 0) * 100).round() : (task['progress'] ?? 0)}',
                    style: TextStyle(color: isLocalDl ? green : blue, fontSize: 8, fontWeight: FontWeight.bold)),
                ]),
              ),
            ],
            if (status != 'parsing') ...[
              const SizedBox(height: 6),
              _iconBtn(Icons.delete_outline_rounded, const Color(0xFF64748B), () {
                _confirmDelete(id, task['title'] ?? task['url'] ?? '');
              }),
            ],
          ]),
        ]),
        // 进度条
        if (isLocalDl) ...[
          const SizedBox(height: 10),
          _progressBar(_dlProgress[id]!, green),
          const SizedBox(height: 4),
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            Text('${((_dlProgress[id] ?? 0) * 100).round()}%', style: TextStyle(color: green, fontSize: 11, fontWeight: FontWeight.bold)),
            if ((_dlSpeed[id] ?? 0) > 0) Text(_fmtSpeed(_dlSpeed[id]!), style: TextStyle(color: blue, fontSize: 11)),
          ]),
        ] else if (status == 'downloading') ...[
          const SizedBox(height: 10),
          _progressBar((task['progress'] ?? 0) / 100.0, blue),
          const SizedBox(height: 4),
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            Text('服务端 ${task['progress'] ?? 0}%', style: TextStyle(color: blue, fontSize: 11)),
            if ((task['speed'] ?? 0) > 0) Text(_fmtSpeed(task['speed']), style: TextStyle(color: purple, fontSize: 11)),
          ]),
        ],
        // 错误信息
        if (status == 'failed' && error.isNotEmpty) ...[
          const SizedBox(height: 8),
          Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(color: const Color(0x1AFCA5A5), borderRadius: BorderRadius.circular(10)),
            child: Row(children: [
              Icon(Icons.warning_rounded, color: red, size: 15),
              const SizedBox(width: 8),
              Expanded(child: Text(error, style: TextStyle(color: red, fontSize: 12), maxLines: 2, overflow: TextOverflow.ellipsis)),
            ]),
          ),
        ],
      ]),
    );
  }

  /// 网格版任务卡片（一排三个，紧凑竖排）
  Widget _taskCardGrid(dynamic task) {
    final status = task['status'] ?? '';
    final id = task['id'] ?? '';
    final title = task['title'] ?? task['url'] ?? '';
    final cover = task['cover_url'] ?? '';
    final platform = task['platform'] ?? '';
    final isLocalDl = _dlProgress.containsKey(id);
    final isSaved = _savedToGallery.contains(id);

    return GlassContainer(
      borderRadius: 14,
      padding: const EdgeInsets.all(8),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        // 复选框（多选模式）
        if (_multiSelect)
          Align(
            alignment: Alignment.topRight,
            child: Checkbox(
              value: _selected.contains(id),
              onChanged: (v) => setState(() {
                if (v == true) _selected.add(id); else _selected.remove(id);
              }),
              activeColor: green,
              checkColor: const Color(0xFF050810),
              side: BorderSide(color: text3),
              materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
          ),
        // 封面（填满剩余空间）
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(10),
            child: Stack(children: [
              if (cover.isNotEmpty)
                Image.network(cover, width: double.infinity, height: double.infinity, fit: BoxFit.cover,
                  errorBuilder: (_, __, ___) => Container(color: const Color(0x0DFFFFFF)))
              else
                Container(color: const Color(0x0DFFFFFF), child: Center(child: Icon(Icons.video_file, color: text3, size: 32))),
              // 平台角标
              if (platform.isNotEmpty)
                Positioned(top: 4, left: 4, child: Container(
                  width: 22, height: 22,
                  decoration: BoxDecoration(
                    color: platform == 'bilibili' ? const Color(0xFFFF6B9D) : Colors.black.withOpacity(0.85),
                    borderRadius: BorderRadius.circular(6),
                  ),
                  alignment: Alignment.center,
                  child: Text(
                    platform == 'bilibili' ? 'B' : 'D',
                    style: TextStyle(color: text1, fontSize: 12, fontWeight: FontWeight.w900),
                  ),
                )),
              // 已保存角标
              if (isSaved)
                Positioned(top: 4, right: 4, child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(color: green.withOpacity(0.9), borderRadius: BorderRadius.circular(6)),
                  child: Text('已保存', style: TextStyle(color: Color(0xFF050810), fontSize: 9, fontWeight: FontWeight.bold)),
                )),
              // 进度条覆盖
              if (isLocalDl)
                Positioned(bottom: 0, left: 0, right: 0,
                  child: LinearProgressIndicator(
                    value: _dlProgress[id]!, minHeight: 4,
                    backgroundColor: Colors.transparent,
                    valueColor: AlwaysStoppedAnimation(green),
                  ),
                ),
            ]),
          ),
        ),
        // ===== 固定文字区（4行） =====
        const SizedBox(height: 6),
        // 第1-2行：标题
        SizedBox(
          height: 30,
          child: Text(title, style: TextStyle(color: text1, fontSize: 11, fontWeight: FontWeight.w600, height: 1.3),
            maxLines: 2, overflow: TextOverflow.ellipsis),
        ),
        // 第3行：状态
        SizedBox(
          height: 20,
          child: Row(children: [
            if (isSaved)
              Expanded(child: Text(
                ApiService.deleteServerAfterSave ? '已保存·已清理' : '已保存',
                style: TextStyle(
                  color: ApiService.deleteServerAfterSave ? const Color(0xFF8B949E) : blue,
                  fontSize: 10, fontWeight: FontWeight.w500),
                maxLines: 1, overflow: TextOverflow.ellipsis,
              ))
            else if (status == 'completed')
              _tag('待下载到相册', green)
            else if (status == 'failed')
              _tag('失败', red)
            else if (status == 'downloading')
              _tag('下载中', blue)
            else if (status == 'parsing')
              _tag('解析中', blue),
          ]),
        ),
        // 第4行：文字按钮（平分）
        Row(children: [
          if (status == 'completed' && !isSaved) ...[
            Expanded(child: _textBtn('下载', green, () => _downloadToGallery(task))),
            Expanded(child: _textBtn('播放', blue, () =>
              Navigator.push(context, MaterialPageRoute(builder: (_) => PlayerPage(taskId: id, title: task['title'] ?? '', localPath: _playbackUrl(id)))))),
            Expanded(child: _textBtn('删除', const Color(0xFF64748B), () => _confirmDelete(id, task['title'] ?? ''))),
          ] else if (status == 'completed' || isSaved) ...[
            Expanded(child: _textBtn('播放', blue, () =>
              Navigator.push(context, MaterialPageRoute(builder: (_) => PlayerPage(taskId: id, title: task['title'] ?? '', localPath: _playbackUrl(id)))))),
            Expanded(child: _textBtn('删除', const Color(0xFF64748B), () => _confirmDelete(id, task['title'] ?? ''))),
          ] else if (status == 'failed') ...[
            Expanded(child: _textBtn('重试', blue, () => ApiService.retryTask(id))),
            Expanded(child: _textBtn('删除', const Color(0xFF64748B), () => _confirmDelete(id, task['title'] ?? ''))),
          ] else ...[
            Expanded(child: _textBtn('删除', const Color(0xFF64748B), () => _confirmDelete(id, task['title'] ?? ''))),
          ],
        ]),
      ]),
    );
  }

  Widget _textBtn(String label, Color color, VoidCallback onTap) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 3),
        decoration: BoxDecoration(
          color: color.withOpacity(0.12),
          borderRadius: BorderRadius.circular(6),
        ),
        alignment: Alignment.center,
        child: Text(label, style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
      ),
    );
  }

  Widget _miniBtn(IconData icon, Color color, VoidCallback onTap) {
    return GestureDetector(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 2),
        child: Icon(icon, color: color, size: 16),
      ),
    );
  }

  Widget _iconBtn(IconData icon, Color color, VoidCallback onTap) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: 32, height: 32,
        decoration: BoxDecoration(
          color: color.withOpacity(0.12),
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: color.withOpacity(0.2), width: 0.5),
        ),
        child: Icon(icon, color: color, size: 16),
      ),
    );
  }

  Widget _tag(String text, Color color) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
    decoration: BoxDecoration(color: color.withOpacity(0.15), borderRadius: BorderRadius.circular(6),
      border: Border.all(color: color.withOpacity(0.25), width: 0.5)),
    child: Text(text, style: TextStyle(color: color, fontSize: 10, fontWeight: FontWeight.w700)),
  );

  Widget _statusTag(String status) {
    final (text, color) = switch (status) {
      'pending' => ('等待', text3),
      'parsing' => ('解析中', blue),
      'downloading' => ('服务端下载中', blue),
      'completed' => ('就绪', green),
      'failed' => ('失败', red),
      _ => ('', text3),
    };
    if (text.isEmpty) return const SizedBox.shrink();
    return _tag(text, color);
  }

  Widget _progressBar(double value, Color color) => ClipRRect(
    borderRadius: BorderRadius.circular(4),
    child: LinearProgressIndicator(value: value, minHeight: 5,
      backgroundColor: const Color(0x1AFFFFFF), valueColor: AlwaysStoppedAnimation(color)),
  );

  // ===== 合集页 =====
  Widget _buildCollections() {
    if (_collections.isEmpty) return _emptyPlaceholder(Icons.collections_bookmark_rounded, '暂无合集', '在服务器上粘贴合集链接自动识别');
    return ListView.builder(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 100),
      itemCount: _collections.length,
      itemBuilder: (_, i) => _expandableCollection(_collections[i]),
    );
  }

  Widget _expandableCollection(dynamic col) {
    final title = col['title'] ?? '';
    final cover = col['cover_url'] ?? '';
    final platform = col['platform'] ?? '';
    final totalCount = col['total_count'] ?? 0;
    final videos = col['videos'] as List<dynamic>? ?? [];
    final completedCount = videos.where((v) => v['status'] == 'completed').length;
    final colId = col['id'] ?? '';

    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(18),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
          child: Container(
            decoration: BoxDecoration(
              color: bg.withOpacity(0.5),
              borderRadius: BorderRadius.circular(18),
              border: Border.all(color: border, width: 0.5),
            ),
            child: Theme(
              data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
              child: ExpansionTile(
                tilePadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 4),
                childrenPadding: const EdgeInsets.fromLTRB(14, 0, 14, 14),
                leading: cover.isNotEmpty
                  ? ClipRRect(borderRadius: BorderRadius.circular(10),
                      child: Image.network(cover, width: 48, height: 48, fit: BoxFit.cover,
                        errorBuilder: (_, __, ___) => Container(width: 48, height: 48, color: bg.withOpacity(0.3))))
                  : null,
                title: Text(title, style: TextStyle(color: text1, fontWeight: FontWeight.w600, fontSize: 14),
                  maxLines: 2, overflow: TextOverflow.ellipsis),
                subtitle: Row(children: [
                  if (platform.isNotEmpty) ...[_tag(platform == 'bilibili' ? 'B站' : '抖音', platform == 'bilibili' ? blue : red), const SizedBox(width: 8)],
                  Text('$completedCount/$totalCount 已完成', style: TextStyle(color: green, fontSize: 12, fontWeight: FontWeight.w500)),
                ]),
                trailing: Row(mainAxisSize: MainAxisSize.min, children: [
                  // 订阅开关
                  GlassIconButton(
                    icon: col['subscribed'] == true ? Icons.bookmarks_rounded : Icons.bookmark_border_rounded,
                    color: col['subscribed'] == true ? purple : text3, size: 32,
                    onPressed: () => _toggleSubscribe(colId, col['subscribed'] == true)),
                  const SizedBox(width: 4),
                  GlassIconButton(icon: Icons.delete_outline_rounded, color: const Color(0xFF64748B), size: 32,
                    onPressed: () => _confirmDeleteCollection(colId, title)),
                ]),
                children: [for (var j = 0; j < videos.length; j++) _collectionVideoItem(colId, videos[j], j)],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _collectionVideoItem(String colId, dynamic video, int index) {
    final title = video['title'] ?? '';
    final author = video['author'] ?? '';
    final cover = video['cover_url'] ?? '';
    final status = video['status'] ?? '';
    final videoId = (video['video_id'] ?? '').toString();
    final bvid = (video['bvid'] ?? '').toString();
    final filePath = video['file_path'] ?? '';

    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: bg.withOpacity(0.3),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: border, width: 0.5),
      ),
      child: Row(children: [
        if (cover.isNotEmpty) ...[
          ClipRRect(borderRadius: BorderRadius.circular(8),
            child: Image.network(cover, width: 48, height: 48, fit: BoxFit.cover,
              errorBuilder: (_, __, ___) => Container(width: 48, height: 48, color: bg.withOpacity(0.3)))),
          const SizedBox(width: 10),
        ],
        Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(title, style: TextStyle(color: text1, fontSize: 13, fontWeight: FontWeight.w500),
            maxLines: 2, overflow: TextOverflow.ellipsis),
          if (author.isNotEmpty) ...[
            const SizedBox(height: 2),
            Text(author, style: TextStyle(color: text3, fontSize: 11)),
          ],
          const SizedBox(height: 4),
          _statusTag(status),
        ])),
        if (status == 'completed' && filePath.isNotEmpty) ...[
          GlassIconButton(icon: Icons.play_arrow_rounded, color: blue, size: 32,
            onPressed: () => Navigator.push(context, MaterialPageRoute(
              builder: (_) => PlayerPage(taskId: colId, title: title,
                directUrl: ApiService.collectionVideoUrl(colId, index))))),
          const SizedBox(width: 4),
        ],
        // 单视频删除（从合集中移除并删除服务器文件）
        GlassIconButton(icon: Icons.close_rounded, color: const Color(0xFF64748B), size: 32,
          onPressed: () => _confirmDeleteCollectionVideo(colId, videoId.isNotEmpty ? videoId : bvid, title)),
      ]),
    );
  }

  /// 切换合集订阅
  Future<void> _toggleSubscribe(String colId, bool current) async {
    await ApiService.toggleCollectionSubscribe(colId, !current);
    _refresh(force: true); // 操作后立即刷新
  }

  /// 删除合集确认
  Future<void> _confirmDeleteCollection(String colId, String title) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1F2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text('删除合集', style: TextStyle(color: text1, fontSize: 18)),
        content: Text('确定删除合集「$title」吗？\n服务器上的视频文件将一并删除。已保存到相册的不受影响。',
          style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 14)),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false),
            child: Text('取消', style: TextStyle(color: Color(0xFF64748B)))),
          TextButton(onPressed: () => Navigator.pop(ctx, true),
            child: Text('删除', style: TextStyle(color: Color(0xFFFCA5A5), fontWeight: FontWeight.bold))),
        ],
      ),
    );
    if (confirmed == true) await ApiService.deleteCollection(colId);
    _refresh(force: true); // 操作后立即刷新
  }

  /// 删除合集内单个视频确认
  Future<void> _confirmDeleteCollectionVideo(String colId, String videoId, String title) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF1A1F2E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: Text('删除视频', style: TextStyle(color: text1, fontSize: 18)),
        content: Text('从合集中移除「$title」吗？\n服务器上的文件将一并删除。已保存到相册的不受影响。',
          style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 14)),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false),
            child: Text('取消', style: TextStyle(color: Color(0xFF64748B)))),
          TextButton(onPressed: () => Navigator.pop(ctx, true),
            child: Text('删除', style: TextStyle(color: Color(0xFFFCA5A5), fontWeight: FontWeight.bold))),
        ],
      ),
    );
    if (confirmed == true) await ApiService.deleteCollectionVideo(videoId);
    _refresh(force: true); // 操作后立即刷新
  }

  // ===== 日志页 =====
  Widget _buildLogs() {
    return Column(children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
        child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
          Text('系统日志', style: TextStyle(color: text3, fontSize: 12, letterSpacing: 2, fontWeight: FontWeight.w700)),
          TextButton.icon(
            icon: const Icon(Icons.delete_sweep_rounded, size: 16),
            label: Text('清空', style: TextStyle(fontSize: 12)),
            onPressed: () { ApiService.clearLogs(); setState(() => _logs = ''); },
            style: TextButton.styleFrom(foregroundColor: text2),
          ),
        ]),
      ),
      Expanded(child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16),
        child: GlassContainer(
          child: _logs.isEmpty
            ? Center(child: Text('暂无日志', style: TextStyle(color: text3)))
            : SingleChildScrollView(
                padding: const EdgeInsets.all(14),
                child: SelectableText(_logs,
                  style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 11, fontFamily: 'monospace', height: 1.6)),
              ),
        ),
      )),
    ]);
  }

  Widget _emptyState() => Center(
    child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
      GlassContainer(borderRadius: 40, width: 80, height: 80,
        child: Icon(Icons.link_rounded, size: 36, color: text3)),
      const SizedBox(height: 20),
      Text('粘贴链接开始下载', style: TextStyle(color: Color(0xFF94A3B8), fontSize: 16, fontWeight: FontWeight.w600)),
      const SizedBox(height: 6),
      Text('支持 B站 · 抖音 · 短链接', style: TextStyle(color: text3, fontSize: 13)),
    ]),
  );

  Widget _emptyPlaceholder(IconData icon, String title, String subtitle) =>
    Center(child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
      Icon(icon, size: 60, color: text3),
      const SizedBox(height: 16),
      Text(title, style: const TextStyle(color: Color(0xFF94A3B8), fontSize: 16, fontWeight: FontWeight.w600)),
      const SizedBox(height: 4),
      Text(subtitle, style: TextStyle(color: text3, fontSize: 13)),
    ]));

  String _fmtSpeed(int bytes) {
    if (bytes <= 0) return '';
    if (bytes >= 1048576) return '${(bytes / 1048576).toStringAsFixed(1)} MB/s';
    if (bytes >= 1024) return '${(bytes / 1024).toStringAsFixed(0)} KB/s';
    return '$bytes B/s';
  }
}
