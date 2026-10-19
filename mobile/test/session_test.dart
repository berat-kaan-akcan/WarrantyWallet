import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/app.dart';
import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/features/products/add_product_screen.dart';
import 'package:warranty_wallet/features/service/report_issue_screen.dart';
import 'package:warranty_wallet/services/services.dart';
import 'package:warranty_wallet/state/providers.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

/// A 1×1 transparent PNG.
const _png =
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';

class _OnePhotoPicker implements ReceiptImagePicker {
  @override
  Future<PickedImage?> pick({required bool camera}) async =>
      PickedImage('/tmp/a.png', 'a.png', base64Decode(_png));
}

void main() {
  test('logout cancels device reminders and drops cached lists', () async {
    final repo = FakeRepository();
    final scheduler = FakeReminderScheduler();
    final container = ProviderContainer(overrides: [
      repositoryProvider.overrideWithValue(repo),
      tokenStoreProvider.overrideWithValue(MemoryTokenStore()),
      reminderSchedulerProvider.overrideWithValue(scheduler),
    ]);
    addTearDown(container.dispose);
    final sub =
        container.listen(productsProvider(const ProductQuery()), (_, __) {});
    addTearDown(sub.close);
    final auth = container.read(authControllerProvider.notifier);
    await auth.login('berat@test.io', 'supersecret');
    expect(await container.read(productsProvider(const ProductQuery()).future),
        hasLength(2));

    repo.items = [headphones]; // what the next account owns
    await auth.logout();
    expect(scheduler.syncAllCalls.last, isEmpty);
    expect(await container.read(productsProvider(const ProductQuery()).future),
        hasLength(1));
  });

  testWidgets('offline start keeps the session and offers a retry',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()
      ..meError = const ApiException(0, 'network', 'offline');
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo, token: 'tok'),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    expect(find.text('Bağlantı yok'), findsOneWidget);
    expect(find.byKey(const Key('submit')), findsNothing); // not the login form

    repo.meError = null;
    await tester.tap(find.byKey(const Key('retry')));
    await tester.pumpAndSettle();
    expect(find.text('Bağlantı yok'), findsNothing);
    expect(find.byKey(const Key('active_count')), findsOneWidget);
  });

  testWidgets('date picker and system texts are Turkish', (tester) async {
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository()),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    final context = tester.element(find.byKey(const Key('submit')));
    expect(Localizations.localeOf(context).languageCode, 'tr');
    expect(MaterialLocalizations.of(context).cancelButtonLabel, 'İptal');
  });

  testWidgets('failed photo upload does not create a second service record',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()
      ..uploadError = const ApiException(0, 'network', 'offline');
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo, picker: _OnePhotoPicker()),
        child:
            MaterialApp(home: ReportIssueScreen(product: monitorProduct()))));
    await tester.tap(find.byKey(const Key('problem_screen_lines')));
    await tester.tap(find.text('Fotoğraf'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('create_service')));
    await tester.tap(find.byKey(const Key('create_service')));
    await tester.pumpAndSettle();
    expect(repo.serviceRecordsCreated, 1);
    expect(find.textContaining('1 fotoğraf yüklenemedi'), findsOneWidget);

    repo.uploadError = null;
    await tester.tap(find.byKey(const Key('create_service')));
    await tester.pumpAndSettle();
    expect(repo.serviceRecordsCreated, 1);
    expect(repo.uploads, hasLength(1));
    expect(find.text('Servis kaydı oluşturuldu'), findsOneWidget);
  });

  test('server messages never reach the user in English', () {
    const e = ApiException(403, 'some_new_code', 'only the owner can do this',
        {'purchase_date': 'x', 'price': 'y', 'unknown': 'z'});
    expect(e.userMessage, isNot(contains('owner')));
    expect(e.fieldList, 'satın alma tarihi, fiyat');
    expect(const ApiException(404, 'not_in_family', 'x').userMessage,
        'Bir aileye bağlı değilsiniz.');
  });
}
