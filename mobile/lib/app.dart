import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'features/auth/login_screen.dart';
import 'features/home/shell.dart';
import 'state/providers.dart';
import 'widgets/common.dart';

/// Root widget: picks the login screen or the main shell based on auth state.
class WarrantyWalletApp extends ConsumerStatefulWidget {
  const WarrantyWalletApp({super.key});

  @override
  ConsumerState<WarrantyWalletApp> createState() => _WarrantyWalletAppState();
}

class _WarrantyWalletAppState extends ConsumerState<WarrantyWalletApp> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(authControllerProvider.notifier).restore());
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authControllerProvider);
    return MaterialApp(
      title: 'Warranty Wallet',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(colorSchemeSeed: brandColor, useMaterial3: true),
      darkTheme: ThemeData(colorSchemeSeed: brandColor, brightness: Brightness.dark, useMaterial3: true),
      home: !auth.initialized
          ? const Scaffold(body: Center(child: CircularProgressIndicator()))
          : auth.isLoggedIn
              ? const MainShell()
              : const LoginScreen(),
    );
  }
}
