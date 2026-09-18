import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:liquid_glass_widgets/liquid_glass_widgets.dart';
import 'package:provider/provider.dart';
import 'services/api_service.dart';
import 'services/local_store.dart';
import 'services/theme_provider.dart';
import 'pages/home_page.dart';
import 'pages/login_page.dart';
import 'pages/setup_page.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Colors.transparent,
    statusBarIconBrightness: Brightness.light,
    systemNavigationBarColor: Colors.transparent,
  ));

  await LiquidGlassWidgets.initialize(
    // 预热 shader 到内存；列表卡片业务侧已用 minimal，滚动期几乎不跑自定义 shader
    enablePerformanceMonitor: false,
  );
  await ApiService.init();

  runApp(
    ChangeNotifierProvider(
      create: (_) => ThemeProvider(),
      child: LiquidGlassWidgets.wrap(
        child: const DouBiApp(),
      ),
    ),
  );
}

/// 启动路由：未配置→设置；在线且需登录→登录；否则主页（含离线缓存）
enum _StartRoute { setup, login, needSetup, home }

class DouBiApp extends StatelessWidget {
  const DouBiApp({super.key});

  static Future<({_StartRoute route, bool needSetup})> _decide() async {
    if (ApiService.baseUrl.isEmpty) {
      return (route: _StartRoute.setup, needSetup: false);
    }
    final online = await ApiService.checkConnection();
    if (!online) {
      final tasks = await LocalStore.loadTasks();
      final cols = await LocalStore.loadCollections();
      if (tasks.isNotEmpty || cols.isNotEmpty) {
        return (route: _StartRoute.home, needSetup: false); // 离线缓存
      }
      return (route: _StartRoute.setup, needSetup: false);
    }
    final auth = await ApiService.authStatus();
    if (auth['auth_mode'] == 'off') {
      return (route: _StartRoute.home, needSetup: false);
    }
    final hasToken =
        ApiService.authToken != null && ApiService.authToken!.isEmpty == false;
    // 有本地 token 但服务器会话已丢（重启等）→ 必须回登录，不能进主页
    if (auth['logged_in'] == true) {
      return (route: _StartRoute.home, needSetup: false);
    }
    if (hasToken) {
      // 用一次业务请求验证 token 是否仍有效
      try {
        await ApiService.getTasks();
        return (route: _StartRoute.home, needSetup: false);
      } catch (_) {
        await ApiService.setToken(null);
      }
    }
    if (auth['need_setup'] == true) {
      return (route: _StartRoute.needSetup, needSetup: true);
    }
    return (route: _StartRoute.login, needSetup: false);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Provider.of<ThemeProvider>(context);

    return MaterialApp(
      title: 'DouBi下载器',
      debugShowCheckedModeBanner: false,
      theme: theme.theme,
      home: FutureBuilder<({_StartRoute route, bool needSetup})>(
        future: _decide(),
        builder: (ctx, snap) {
          if (snap.connectionState != ConnectionState.done) return const _Splash();
          final d = snap.data;
          if (d == null) return const SetupPage();
          switch (d.route) {
            case _StartRoute.home:
              return const HomePage();
            case _StartRoute.login:
              return const LoginPage();
            case _StartRoute.needSetup:
              return const LoginPage(needSetup: true);
            case _StartRoute.setup:
              return const SetupPage();
          }
        },
      ),
    );
  }
}

class _Splash extends StatelessWidget {
  const _Splash();
  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        gradient: RadialGradient(center: Alignment(0, -0.4), radius: 1.8,
          colors: [Color(0xFF0F2027), Color(0xFF050810)]),
      ),
      child: const Center(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Icon(Icons.play_circle_fill_rounded, size: 64, color: Color(0xFF00D09C)),
          SizedBox(height: 16),
          Text('DouBi', style: TextStyle(color: Colors.white, fontSize: 28, fontWeight: FontWeight.w800, letterSpacing: 4)),
          SizedBox(height: 8),
          Text('视频下载器', style: TextStyle(color: Color(0xFF64748B), fontSize: 14)),
        ]),
      ),
    );
  }
}
