import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../services/api_service.dart';
import '../services/native_bridge.dart';
import '../services/theme_provider.dart';
import '../services/update_service.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key});
  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  final _serverCtrl = TextEditingController();
  bool _testing = false;
  String? _toast;
  Map<String, dynamic> _config = {};
  bool _serverOnline = false;
  // 版本与更新
  String _appVersion = '';
  String _updateStatus = ''; // checking / latest / available / downloading / ready / error
  Map<String, dynamic>? _updateInfo;
  double _dlProgress = 0;

  @override
  void initState() {
    super.initState();
    _serverCtrl.text = ApiService.baseUrl;
    _checkServer();
    _loadVersionAndCheckUpdate();
  }

  Future<void> _loadVersionAndCheckUpdate() async {
    final info = await NativeBridge.getAppVersion();
    if (mounted) setState(() => _appVersion = info['version'] ?? '');
    try {
      final r = await UpdateService.checkUpdate();
      if (!mounted) return;
      setState(() {
        _updateInfo = r;
        _updateStatus = r['hasUpdate'] == true ? 'available' : 'latest';
      });
    } catch (_) {
      if (mounted) setState(() => _updateStatus = 'error');
    }
  }

  Future<void> _downloadAndInstall() async {
    setState(() => _updateStatus = 'downloading');
    final path = await UpdateService.downloadApk(onProgress: (p) {
      if (mounted) setState(() => _dlProgress = p);
    });
    if (path == null) {
      if (mounted) setState(() => _updateStatus = 'error');
      return;
    }
    if (mounted) setState(() => _updateStatus = 'ready');
    await UpdateService.install(path);
  }

  Future<void> _checkServer() async {
    _serverOnline = await ApiService.checkConnection();
    if (_serverOnline) _config = await ApiService.getConfig();
    if (mounted) setState(() {});
  }

  void _showToast(String msg) {
    setState(() => _toast = msg);
    Future.delayed(const Duration(seconds: 2), () { if (mounted) setState(() => _toast = null); });
  }

  Future<void> _testConnection() async {
    setState(() => _testing = true);
    final old = ApiService.baseUrl;
    final url = _serverCtrl.text.trim();
    await ApiService.setServerUrl(url.startsWith('http') ? url : 'http://$url');
    final ok = await ApiService.checkConnection();
    setState(() => _testing = false);
    if (ok) { _showToast('连接成功'); _checkServer(); }
    else { _showToast('连接失败'); await ApiService.setServerUrl(old); }
  }

  @override
  Widget build(BuildContext context) {
    final tp = Provider.of<ThemeProvider>(context);
    return Container(
      decoration: BoxDecoration(
        gradient: RadialGradient(center: const Alignment(0, -0.3), radius: 1.6,
          colors: [tp.bg.withOpacity(0.85), tp.bg]),
      ),
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          backgroundColor: Colors.transparent, elevation: 0,
          leading: IconButton(icon: Icon(Icons.arrow_back_ios_rounded, color: tp.textSecondary, size: 20),
            onPressed: () => Navigator.pop(context)),
          title: Text('设置', style: TextStyle(color: tp.textPrimary, fontWeight: FontWeight.bold, fontSize: 18)),
        ),
        body: Stack(children: [
          ListView(padding: const EdgeInsets.all(16), children: [
            // 版本 + 服务器状态
            _card(tp, Row(children: [
              Container(width: 44, height: 44,
                decoration: BoxDecoration(gradient: LinearGradient(colors: [tp.primary, tp.primaryDim]),
                  borderRadius: BorderRadius.circular(12)),
                child: const Center(child: Text('C', style: TextStyle(color: Color(0xFF0D0D0D), fontWeight: FontWeight.w900, fontSize: 20)))),
              const SizedBox(width: 14),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('DouBi 视频下载器', style: TextStyle(color: tp.textPrimary, fontWeight: FontWeight.w600, fontSize: 15)),
                const SizedBox(height: 4),
                Row(children: [
                  Container(width: 8, height: 8, decoration: BoxDecoration(color: _serverOnline ? const Color(0xFF00D09C) : tp.error, shape: BoxShape.circle)),
                  const SizedBox(width: 6),
                  Text(_serverOnline ? '服务器在线 v${_config['version'] ?? ''}' : '服务器离线',
                    style: TextStyle(color: _serverOnline ? const Color(0xFF00D09C) : tp.error, fontSize: 12)),
                ]),
              ])),
            ])),
            const SizedBox(height: 16),
            // 服务器连接
            _section(tp, Icons.dns_rounded, '服务器连接'),
            _card(tp, Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              _input(tp, _serverCtrl, '服务器地址', 'http://192.168.x.x:18000', Icons.language_rounded),
              const SizedBox(height: 12),
              SizedBox(width: double.infinity, child: _btn(tp, '测试连接', Icons.wifi_find_rounded, onPressed: _testing ? null : _testConnection, loading: _testing)),
            ])),
            const SizedBox(height: 16),
            // 外观
            _section(tp, Icons.palette_rounded, '外观'),
            _card(tp, Row(children: [
              Icon(tp.isDark ? Icons.dark_mode_rounded : Icons.light_mode_rounded, color: tp.primary, size: 22),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(tp.isDark ? '深色模式' : '浅色模式', style: TextStyle(color: tp.textPrimary, fontWeight: FontWeight.w600, fontSize: 14)),
                Text('点击切换', style: TextStyle(color: tp.textDim, fontSize: 12)),
              ])),
              Switch(value: tp.isDark, onChanged: (_) => tp.toggleTheme(), activeColor: tp.primary),
            ])),
            const SizedBox(height: 16),
            // 下载设置
            _section(tp, Icons.download_rounded, '下载设置'),
            _card(tp, Row(children: [
              Icon(Icons.delete_sweep_rounded, color: tp.primary, size: 22),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('保存后删除服务器缓存', style: TextStyle(color: tp.textPrimary, fontWeight: FontWeight.w600, fontSize: 14)),
                Text('视频保存到相册后自动删除服务器上的文件', style: TextStyle(color: tp.textDim, fontSize: 12)),
              ])),
              Switch(
                value: ApiService.deleteServerAfterSave,
                onChanged: (v) { ApiService.setDeleteServerAfterSave(v); setState(() {}); },
                activeColor: tp.primary,
              ),
            ])),
            const SizedBox(height: 16),
            // 关于 + 更新
            _section(tp, Icons.info_rounded, '关于'),
            _card(tp, Column(children: [
              _info(tp, '版本', 'v$_appVersion'),
              Divider(color: tp.border, height: 1),
              _info(tp, '框架', 'Flutter + Go'),
              Divider(color: tp.border, height: 1),
              _info(tp, '架构', '瘦客户端 + NAS 服务器'),
              Divider(color: tp.border, height: 1),
              _buildUpdateRow(tp),
            ])),
          ]),
          if (_toast != null) Positioned(bottom: 40, left: 0, right: 0,
            child: Center(child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 10),
              decoration: BoxDecoration(color: tp.primary, borderRadius: BorderRadius.circular(24)),
              child: Text(_toast!, style: TextStyle(color: tp.textPrimary, fontWeight: FontWeight.bold, fontSize: 13)),
            ))),
        ]),
      ),
    );
  }

  Widget _section(ThemeProvider tp, IconData icon, String title) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: Row(children: [
      Icon(icon, color: tp.primary, size: 16),
      const SizedBox(width: 8),
      Text(title, style: TextStyle(color: tp.primary, fontSize: 13, fontWeight: FontWeight.w700, letterSpacing: 1)),
    ]),
  );

  Widget _card(ThemeProvider tp, Widget child) => ClipRRect(
    borderRadius: BorderRadius.circular(16),
    child: BackdropFilter(
      filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
      child: Container(width: double.infinity, padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(color: tp.glass, borderRadius: BorderRadius.circular(16),
          border: Border.all(color: tp.border, width: 0.5)),
        child: child),
    ),
  );

  Widget _input(ThemeProvider tp, TextEditingController ctrl, String label, String hint, IconData icon) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(12),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
        child: Container(
          decoration: BoxDecoration(color: tp.glass, borderRadius: BorderRadius.circular(12),
            border: Border.all(color: tp.border, width: 0.5)),
          child: TextField(controller: ctrl,
            style: TextStyle(color: tp.textPrimary, fontSize: 14),
            decoration: InputDecoration(
              labelText: label, labelStyle: TextStyle(color: tp.textDim, fontSize: 12),
              hintText: hint, hintStyle: TextStyle(color: tp.textDim, fontSize: 13),
              prefixIcon: Icon(icon, color: tp.textDim, size: 18),
              border: InputBorder.none,
              contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12))),
        ),
      ),
    );
  }

  Widget _btn(ThemeProvider tp, String label, IconData icon, {VoidCallback? onPressed, bool loading = false}) {
    return Container(height: 44,
      decoration: BoxDecoration(
        gradient: LinearGradient(colors: [tp.primary, tp.primary.withOpacity(0.8)]),
        borderRadius: BorderRadius.circular(12),
        boxShadow: [BoxShadow(color: tp.primary.withOpacity(0.3), blurRadius: 12, offset: const Offset(0, 4))]),
      child: ElevatedButton.icon(
        onPressed: loading ? null : onPressed,
        icon: loading ? SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
          : Icon(icon, size: 16, color: Colors.white),
        label: Text(label, style: const TextStyle(fontWeight: FontWeight.bold, color: Colors.white)),
        style: ElevatedButton.styleFrom(backgroundColor: Colors.transparent, shadowColor: Colors.transparent,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12))),
      ),
    );
  }

  Widget _info(ThemeProvider tp, String label, String value) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 10),
    child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
      Text(label, style: TextStyle(color: tp.textSecondary, fontSize: 14)),
      Text(value, style: TextStyle(color: tp.textPrimary, fontSize: 14, fontWeight: FontWeight.w500)),
    ]),
  );

  /// 检查更新行：状态 + 动作
  Widget _buildUpdateRow(ThemeProvider tp) {
    Widget trailing;
    switch (_updateStatus) {
      case 'checking':
        trailing = SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: tp.primary));
      case 'latest':
        trailing = Text('已是最新', style: TextStyle(color: tp.textDim, fontSize: 13));
      case 'available':
        final v = _updateInfo?['version'] ?? '';
        trailing = TextButton(
          onPressed: _downloadAndInstall,
          child: Text('更新到 v$v', style: TextStyle(color: tp.primary, fontWeight: FontWeight.bold)));
      case 'downloading':
        trailing = Text('下载中 ${(_dlProgress * 100).round()}%', style: TextStyle(color: tp.primary, fontSize: 13));
      case 'ready':
        trailing = TextButton(
          onPressed: () => UpdateService.install(UpdateService.downloadedPath ?? ''),
          child: Text('点击安装', style: TextStyle(color: tp.primary, fontWeight: FontWeight.bold)));
      case 'error':
        trailing = TextButton(
          onPressed: () { setState(() => _updateStatus = 'checking'); _loadVersionAndCheckUpdate(); },
          child: Text('检查失败，点重试', style: TextStyle(color: tp.error, fontSize: 13)));
      default:
        trailing = Text('未知', style: TextStyle(color: tp.textDim, fontSize: 13));
    }
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
        Text('检查更新', style: TextStyle(color: tp.textSecondary, fontSize: 14)),
        trailing,
      ]),
    );
  }
}
