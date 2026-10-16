import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/app.dart';
import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/features/auth/password_reset_screen.dart';
import 'package:warranty_wallet/features/products/add_product_screen.dart';
import 'package:warranty_wallet/features/products/product_detail_screen.dart';
import 'package:warranty_wallet/features/settings/settings_screen.dart';
import 'package:warranty_wallet/widgets/common.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen, wrap;

Future<void> expectAccessible(WidgetTester tester) async {
  await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
  await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
  await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
  await expectLater(tester, meetsGuideline(textContrastGuideline));
}

void main() {
  testWidgets('login screen meets a11y guidelines', (tester) async {
    useTallScreen(tester);
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository()),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    await expectAccessible(tester);
    handle.dispose();
  });

  testWidgets('home (signed in) meets a11y guidelines', (tester) async {
    useTallScreen(tester);
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository(), token: 'tok'),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('greeting')), findsOneWidget);
    await expectAccessible(tester);
    handle.dispose();
  });

  for (final entry in <String, Widget Function()>{
    'add product': () => const AddProductScreen(),
    'product detail': () => const ProductDetailScreen(productId: 'p1'),
    'settings': () => const SettingsScreen(),
    'password reset': () => const PasswordResetScreen(),
  }.entries) {
    testWidgets('${entry.key} meets a11y guidelines', (tester) async {
      useTallScreen(tester);
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(wrap(entry.value(), FakeRepository()));
      await tester.pumpAndSettle();
      await expectAccessible(tester);
      handle.dispose();
    });
  }

  testWidgets('error state distinguishes offline and offers retry',
      (tester) async {
    var retried = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ErrorState(
            error: const ApiException(0, 'network', 'down'),
            onRetry: () => retried++),
      ),
    ));
    expect(find.text('Bağlantı yok'), findsOneWidget);
    expect(
        find.text('Sunucuya ulaşılamıyor. İnternet bağlantınızı kontrol edin.'),
        findsOneWidget);
    await tester.tap(find.byKey(const Key('retry')));
    expect(retried, 1);
  });

  testWidgets('empty state works inside a list (unbounded height)',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: const [
          EmptyState(icon: Icons.inbox, title: 'Boş', message: 'Henüz yok')
        ]),
      ),
    ));
    expect(find.text('Boş'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
