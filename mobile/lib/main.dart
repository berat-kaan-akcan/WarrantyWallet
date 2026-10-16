import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';
import 'state/providers.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final container = ProviderContainer();
  try {
    await container.read(reminderSchedulerProvider).init();
  } on Exception catch (e) {
    debugPrint('Notifications unavailable: $e');
  }
  runApp(UncontrolledProviderScope(
      container: container, child: const WarrantyWalletApp()));
}
