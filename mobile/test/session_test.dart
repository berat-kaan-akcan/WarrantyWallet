import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/app.dart';
import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/features/products/add_product_screen.dart';
import 'package:warranty_wallet/features/products/product_detail_screen.dart';
import 'package:warranty_wallet/features/products/product_list_screen.dart';
import 'package:warranty_wallet/features/settings/settings_screen.dart';
import 'package:warranty_wallet/models/models.dart';
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

  testWidgets(
      'a tapped reminder opens its product; permission asked after sign-in',
      (tester) async {
    useTallScreen(tester);
    final scheduler = FakeReminderScheduler()..launchProduct = 'p2';
    await tester.pumpWidget(ProviderScope(
        overrides:
            overrides(FakeRepository(), token: 'tok', scheduler: scheduler),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    expect(scheduler.permissionRequests, 1);
    expect(
        find.text('Sony WH-1000XM5'), findsWidgets); // launched from a reminder
    Navigator.of(tester.element(find.text('Sony WH-1000XM5').first)).pop();
    await tester.pumpAndSettle();

    scheduler.opened.add('p1');
    await tester.pumpAndSettle();
    expect(find.text('ASUS TUF VG27AQ'), findsWidgets);
  });

  testWidgets('no permission prompt before sign-in', (tester) async {
    final scheduler = FakeReminderScheduler();
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository(), scheduler: scheduler),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    expect(scheduler.permissionRequests, 0);
  });

  testWidgets(
      'a wrong document can be deleted; documents come from the gallery too',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()
      ..docs.add(const ProductDocument(
          id: 'd9',
          type: 'invoice',
          contentType: 'application/pdf',
          originalName: 'fatura.pdf'));
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo, picker: _OnePhotoPicker()),
        child: const MaterialApp(home: ProductDetailScreen(productId: 'p1'))));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Faturayı Gör'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('delete_document')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirm_delete_document')));
    await tester.pumpAndSettle();
    expect(repo.deletedDocs, ['d9']);
    expect(find.text('Faturayı Gör'), findsOneWidget); // back on the detail

    await tester.tap(find.text('Ürün Fotoğrafları'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Ekle'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('source_gallery')));
    await tester.pumpAndSettle();
    expect(repo.uploads, ['p1/photo/a.png']);
  });

  testWidgets('search runs while typing and can be cleared', (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository()),
        child: const MaterialApp(home: ProductListScreen())));
    await tester.pumpAndSettle();
    expect(find.text('ASUS TUF VG27AQ'), findsOneWidget);
    await tester.enterText(find.byType(TextField).first, 'sony');
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pumpAndSettle();
    expect(find.text('ASUS TUF VG27AQ'), findsNothing);
    expect(find.text('Sony WH-1000XM5'), findsOneWidget);
    await tester.tap(find.byTooltip('Aramayı temizle'));
    await tester.pumpAndSettle();
    expect(find.text('ASUS TUF VG27AQ'), findsOneWidget);
  });

  testWidgets('logout asks first', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo, token: 'tok'),
        child: const MaterialApp(home: SettingsScreen())));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('logout')));
    await tester.tap(find.byKey(const Key('logout')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Vazgeç'));
    await tester.pumpAndSettle();
    expect(repo.calls.where((c) => c.startsWith('logout')), isEmpty);
    await tester.tap(find.byKey(const Key('logout')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirm_logout')));
    await tester.pumpAndSettle();
    expect(repo.calls.where((c) => c.startsWith('logout')), hasLength(1));
  });

  testWidgets('registration asks for the password twice', (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository()),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Hesabınız yok mu? Kayıt olun'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('email')), 'yeni@test.io');
    await tester.enterText(find.byKey(const Key('password')), 'supersecret');
    await tester.enterText(find.byKey(const Key('password2')), 'supersecreT');
    await tester.tap(find.byKey(const Key('submit')));
    await tester.pumpAndSettle();
    expect(find.text('Şifreler aynı değil'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('password2')), 'supersecret');
    await tester.tap(find.byKey(const Key('submit')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('password2')), findsNothing); // signed in
  });

  testWidgets('category chips and summary tiles open a filtered product list',
      (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository(), token: 'tok'),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('Kulaklık (1)'));
    await tester.tap(find.text('Kulaklık (1)'));
    await tester.pumpAndSettle();
    expect(find.text('Sony WH-1000XM5'), findsOneWidget);
    expect(find.text('ASUS TUF VG27AQ'), findsNothing);

    await tester.tap(find.text('Özet'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('stat_total')));
    await tester.pumpAndSettle();
    expect(find.text('ASUS TUF VG27AQ'), findsOneWidget);
    expect(find.text('Sony WH-1000XM5'), findsOneWidget);
  });

  testWidgets('the privacy notice is reachable from registration',
      (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository()),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Hesabınız yok mu? Kayıt olun'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('privacy_notice')));
    await tester.tap(find.byKey(const Key('privacy_notice')));
    await tester.pumpAndSettle();
    expect(find.text('Yapay zekâ ve yurt dışına aktarım'), findsOneWidget);
  });
}
