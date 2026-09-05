import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:liquid_glass_widgets/liquid_glass_widgets.dart';
import 'package:provider/provider.dart';
import 'services/api_service.dart';
import 'services/local_store.dart';
import 'services/theme_provider.dart';
import 'pages/home_page.dart';
import 'pages/setup_page.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Colors.transparent,
    statusBarIconBrightness: Brightness.light,
    systemNavigationBarColor: Colors.transparent,
  ));

  await LiquidGlassWidgets.initialize();
  await ApiService.init();

  runApp(
    ChangeNotifierProvider(
      create: (_) => ThemeProvider(),
      child: LiquidGlassWidgets.wrap(child: const DouBiApp()),
    ),
  );
}

class DouBiApp extends StatelessWidget {
  const DouBiApp({super.key});

  /// 路由决策：配置过服务器就尽量进主页。
  /// 服务器不可达但手机有缓存 → 离线模式进主页（轮询自动重连恢复）
  static Future<bool> _shouldGoHome() async {
    if (ApiService.baseUrl.isEmpty) return false;
    if (await ApiService.checkConnection()) return true;
    final tasks = await LocalStore.loadTasks();
    final cols = await LocalStore.loadCollections();
    return tasks.isNotEmpty || cols.isNotEmpty;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Provider.of<ThemeProvider>(context);

    return MaterialApp(
      title: 'DouBi下载器',
      debugShowCheckedModeBanner: false,
      theme: theme.theme,
      home: FutureBuilder<bool>(
        future: _shouldGoHome(),
        builder: (ctx, snap) {
          if (snap.connectionState != ConnectionState.done) return const _Splash();
          if (snap.data == true) return const HomePage();
          return const SetupPage();
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
