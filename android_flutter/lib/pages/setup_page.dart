import 'dart:ui';
import 'package:flutter/material.dart';
import '../services/api_service.dart';
import 'home_page.dart';
import 'login_page.dart';

class SetupPage extends StatefulWidget {
  const SetupPage({super.key});
  @override
  State<SetupPage> createState() => _SetupPageState();
}

class _SetupPageState extends State<SetupPage> {
  final _ctrl = TextEditingController();
  bool _testing = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _ctrl.text = ApiService.baseUrl;
  }

  Future<void> _connect() async {
    final url = _ctrl.text.trim();
    if (url.isEmpty) { setState(() => _error = '请输入服务器地址'); return; }

    final normalized = url.startsWith('http') ? url : 'http://$url';
    setState(() { _testing = true; _error = null; });

    await ApiService.setServerUrl(normalized);
    final ok = await ApiService.checkConnection();

    if (!mounted) return;
    if (!ok) {
      setState(() { _testing = false; _error = '无法连接到服务器，请检查地址和端口'; });
      return;
    }

    final auth = await ApiService.authStatus();
    if (!mounted) return;
    if (auth['auth_mode'] == 'off') {
      Navigator.pushReplacement(context,
        MaterialPageRoute(builder: (_) => const HomePage()));
      return;
    }
    if (auth['logged_in'] == true ||
        (ApiService.authToken != null && ApiService.authToken!.isNotEmpty)) {
      Navigator.pushReplacement(context,
        MaterialPageRoute(builder: (_) => const HomePage()));
      return;
    }
    // 开了鉴权且未登录 → 必须走登录页，不能直接进主页
    Navigator.pushReplacement(context,
      MaterialPageRoute(builder: (_) => LoginPage(needSetup: auth['need_setup'] == true)));
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        gradient: RadialGradient(
          center: Alignment(0, -0.4),
          radius: 1.8,
          colors: [Color(0xFF0F2027), Color(0xFF050810)],
        ),
      ),
      child: Scaffold(
        backgroundColor: Colors.transparent,
        body: SafeArea(
          child: Center(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(32),
              child: Column(mainAxisSize: MainAxisSize.min, children: [
                // Logo
                Container(
                  width: 80, height: 80,
                  decoration: BoxDecoration(
                    gradient: const LinearGradient(colors: [Color(0xFF6EE7B7), Color(0xFF34D399)]),
                    borderRadius: BorderRadius.circular(20),
                    boxShadow: [BoxShadow(color: const Color(0xFF6EE7B7).withOpacity(0.3), blurRadius: 24, offset: const Offset(0, 8))],
                  ),
                  child: const Center(child: Text('C', style: TextStyle(color: Color(0xFF050810), fontWeight: FontWeight.w900, fontSize: 36))),
                ),
                const SizedBox(height: 24),
                const Text('DouBi 视频下载器', style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.w800, letterSpacing: 2)),
                const SizedBox(height: 6),
                const Text('连接到你的 NAS 服务器', style: TextStyle(color: Color(0xFF64748B), fontSize: 14)),
                const SizedBox(height: 48),

                // 输入框
                ClipRRect(
                  borderRadius: BorderRadius.circular(16),
                  child: BackdropFilter(
                    filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
                    child: Container(
                      padding: const EdgeInsets.all(20),
                      decoration: BoxDecoration(
                        color: const Color(0x1AFFFFFF),
                        borderRadius: BorderRadius.circular(16),
                        border: Border.all(color: const Color(0x33FFFFFF), width: 0.5),
                      ),
                      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        const Text('服务器地址', style: TextStyle(color: Color(0xFF64748B), fontSize: 12, fontWeight: FontWeight.w600, letterSpacing: 1)),
                        const SizedBox(height: 12),
                        Container(
                          decoration: BoxDecoration(
                            color: const Color(0x0DFFFFFF),
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(color: const Color(0x1AFFFFFF), width: 0.5),
                          ),
                          child: TextField(
                            controller: _ctrl,
                            style: TextStyle(color: Colors.white, fontSize: 15),
                            decoration: const InputDecoration(
                              hintText: 'http://192.168.x.x:18000',
                              hintStyle: TextStyle(color: Color(0xFF475569)),
                              prefixIcon: Icon(Icons.dns_rounded, color: Color(0xFF475569), size: 20),
                              border: InputBorder.none,
                              contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                            ),
                            onSubmitted: (_) => _connect(),
                          ),
                        ),
                        if (_error != null) ...[
                          const SizedBox(height: 8),
                          Text(_error!, style: const TextStyle(color: Color(0xFFFCA5A5), fontSize: 12)),
                        ],
                        const SizedBox(height: 16),
                        SizedBox(
                          width: double.infinity, height: 48,
                          child: Container(
                            decoration: BoxDecoration(
                              gradient: const LinearGradient(colors: [Color(0xFF6EE7B7), Color(0xFF34D399)]),
                              borderRadius: BorderRadius.circular(12),
                              boxShadow: [BoxShadow(color: const Color(0xFF6EE7B7).withOpacity(0.3), blurRadius: 16, offset: const Offset(0, 6))],
                            ),
                            child: ElevatedButton(
                              onPressed: _testing ? null : _connect,
                              style: ElevatedButton.styleFrom(
                                backgroundColor: Colors.transparent,
                                shadowColor: Colors.transparent,
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                              ),
                              child: _testing
                                ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2, color: Color(0xFF050810)))
                                : const Text('连接服务器', style: TextStyle(color: Color(0xFF050810), fontWeight: FontWeight.bold, fontSize: 16)),
                            ),
                          ),
                        ),
                      ]),
                    ),
                  ),
                ),
                const SizedBox(height: 24),
                const Text('需要先在 NAS 上部署 Docker 版服务端\n详见 github.com/Star-wsc/ccnew-vdl',
                  textAlign: TextAlign.center,
                  style: TextStyle(color: Color(0xFF334155), fontSize: 12, height: 1.6)),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
