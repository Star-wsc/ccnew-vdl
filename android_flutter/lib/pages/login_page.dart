import 'dart:ui';
import 'package:flutter/material.dart';
import '../services/api_service.dart';
import 'home_page.dart';

class LoginPage extends StatefulWidget {
  final bool needSetup;
  const LoginPage({super.key, this.needSetup = false});
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  bool _busy = false;
  String? _error;

  Future<void> _login() async {
    final u = _userCtrl.text.trim();
    final p = _passCtrl.text;
    if (u.isEmpty || p.isEmpty) {
      setState(() => _error = '请输入用户名和密码');
      return;
    }
    setState(() { _busy = true; _error = null; });
    final r = await ApiService.login(u, p);
    if (!mounted) return;
    if (r['ok'] == true) {
      Navigator.of(context).pushAndRemoveUntil(
        MaterialPageRoute(builder: (_) => const HomePage()),
        (route) => false,
      );
      return;
    }
    setState(() {
      _busy = false;
      _error = (r['error'] ?? '登录失败').toString();
    });
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
                Container(
                  width: 72, height: 72,
                  decoration: BoxDecoration(
                    gradient: const LinearGradient(colors: [Color(0xFF6EE7B7), Color(0xFF34D399)]),
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: const Icon(Icons.lock_rounded, size: 34, color: Color(0xFF050810)),
                ),
                const SizedBox(height: 20),
                const Text('登录 DouBi', style: TextStyle(color: Colors.white, fontSize: 22, fontWeight: FontWeight.w800)),
                const SizedBox(height: 6),
                Text(
                  ApiService.baseUrl,
                  style: const TextStyle(color: Color(0xFF64748B), fontSize: 12, fontFamily: 'monospace'),
                ),
                const SizedBox(height: 28),
                if (widget.needSetup)
                  Container(
                    margin: const EdgeInsets.only(bottom: 16),
                    padding: const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: const Color(0x33FFB454),
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(color: const Color(0x66FFB454)),
                    ),
                    child: const Text(
                      '服务器尚未初始化，请先用浏览器打开该地址设置管理员密码，再回到这里登录。',
                      style: TextStyle(color: Color(0xFFFFD9A0), fontSize: 13, height: 1.5),
                    ),
                  ),
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
                      child: Column(children: [
                        _field(_userCtrl, '用户名', Icons.person_rounded, false),
                        const SizedBox(height: 12),
                        _field(_passCtrl, '密码', Icons.lock_rounded, true, onSubmitted: _login),
                        if (_error != null) ...[
                          const SizedBox(height: 10),
                          Text(_error!, style: const TextStyle(color: Color(0xFFFCA5A5), fontSize: 12)),
                        ],
                        const SizedBox(height: 16),
                        SizedBox(
                          width: double.infinity, height: 48,
                          child: ElevatedButton(
                            onPressed: _busy || widget.needSetup ? null : _login,
                            style: ElevatedButton.styleFrom(
                              backgroundColor: const Color(0xFF34D399),
                              foregroundColor: const Color(0xFF050810),
                              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                            ),
                            child: _busy
                                ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2, color: Color(0xFF050810)))
                                : const Text('登录', style: TextStyle(fontWeight: FontWeight.bold, fontSize: 16)),
                          ),
                        ),
                        const SizedBox(height: 10),
                        TextButton(
                          onPressed: () => Navigator.pop(context),
                          child: const Text('修改服务器地址', style: TextStyle(color: Color(0xFF64748B), fontSize: 12)),
                        ),
                      ]),
                    ),
                  ),
                ),
              ]),
            ),
          ),
        ),
      ),
    );
  }

  Widget _field(TextEditingController c, String hint, IconData icon, bool obscure, {VoidCallback? onSubmitted}) {
    return Container(
      decoration: BoxDecoration(
        color: const Color(0x0DFFFFFF),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: const Color(0x1AFFFFFF), width: 0.5),
      ),
      child: TextField(
        controller: c,
        obscureText: obscure,
        enableSuggestions: !obscure,
        autocorrect: !obscure,
        style: const TextStyle(color: Colors.white, fontSize: 15),
        onSubmitted: (_) => onSubmitted?.call(),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(color: Color(0xFF475569)),
          prefixIcon: Icon(icon, color: const Color(0xFF475569), size: 20),
          border: InputBorder.none,
          contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        ),
      ),
    );
  }
}
