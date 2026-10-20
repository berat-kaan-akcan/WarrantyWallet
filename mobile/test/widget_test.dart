import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/app.dart';
import 'package:warranty_wallet/core/formatters.dart';
import 'package:warranty_wallet/core/scan_parser.dart';
import 'package:warranty_wallet/core/warranty.dart';
import 'package:warranty_wallet/features/dashboard/dashboard_screen.dart';
import 'package:warranty_wallet/features/products/add_product_screen.dart';
import 'package:warranty_wallet/features/scan/code_scanner.dart';
import 'package:warranty_wallet/features/service/report_issue_screen.dart';
import 'package:warranty_wallet/services/services.dart';
import 'package:warranty_wallet/state/providers.dart';

import 'fakes.dart';

class FakeCodeScanner implements CodeScanner {
  FakeCodeScanner([this.raw]);

  String? raw;

  @override
  Future<ScannedCode?> scan(BuildContext context, {String title = ''}) async =>
      raw == null ? null : parseScannedCode(raw!);
}

class FakePicker implements ReceiptImagePicker {
  @override
  Future<PickedImage?> pick({required bool camera}) async => null;
}

List<Override> overrides(FakeRepository repo,
    {String? token,
    FakeReminderScheduler? scheduler,
    FakeCodeScanner? scanner,
    PushService? push,
    BackupFiles? backup,
    LinkOpener? links,
    ReceiptImagePicker? picker,
    List<String>? shared}) {
  final tokens = MemoryTokenStore()..token = token;
  return [
    repositoryProvider.overrideWithValue(repo),
    tokenStoreProvider.overrideWithValue(tokens),
    ocrServiceProvider.overrideWithValue(NoopOcrService()),
    reminderSchedulerProvider
        .overrideWithValue(scheduler ?? FakeReminderScheduler()),
    imageSourceProvider.overrideWithValue(picker ?? FakePicker()),
    codeScannerProvider.overrideWithValue(scanner ?? FakeCodeScanner()),
    pushServiceProvider.overrideWithValue(push ?? FakePushService()),
    backupFilesProvider.overrideWithValue(backup ?? FakeBackupFiles()),
    linkOpenerProvider.overrideWithValue(links ?? FakeLinkOpener()),
    textSharerProvider.overrideWithValue((t) async => shared?.add(t)),
  ];
}

Widget wrap(Widget child, FakeRepository repo) => ProviderScope(
      overrides: overrides(repo),
      child: MaterialApp(home: child),
    );

/// Phone-sized but tall surface so long forms are fully built.
void useTallScreen(WidgetTester tester) {
  tester.view.physicalSize = const Size(1080, 2600);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.reset);
}

void main() {
  testWidgets('login validates input and signs in', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo), child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();

    expect(find.text('Warranty Wallet'), findsOneWidget);
    await tester.tap(find.byKey(const Key('submit')));
    await tester.pump();
    expect(find.text('E-posta gerekli'), findsOneWidget);
    expect(find.text('Şifre en az 8 karakter olmalı'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('email')), 'berat@test.io');
    await tester.enterText(find.byKey(const Key('password')), 'supersecret');
    await tester.tap(find.byKey(const Key('submit')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('greeting')), findsOneWidget);
    expect(find.text('Günaydın, Berat'), findsOneWidget);
  });

  testWidgets('login shows API error message', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()..loginError = 'invalid_credentials';
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo), child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('email')), 'berat@test.io');
    await tester.enterText(find.byKey(const Key('password')), 'wrongpass1');
    await tester.tap(find.byKey(const Key('submit')));
    await tester.pumpAndSettle();
    expect(find.text('E-posta veya şifre hatalı.'), findsOneWidget);
  });

  testWidgets('home shows active count and expiring products', (tester) async {
    useTallScreen(tester);
    final scheduler = FakeReminderScheduler();
    final repo = FakeRepository();
    await tester.pumpWidget(
      ProviderScope(
          overrides: overrides(repo, token: 'tok', scheduler: scheduler),
          child: const WarrantyWalletApp()),
    );
    await tester.pumpAndSettle();

    expect(find.text('Günaydın, Berat'), findsOneWidget);
    expect(
        tester.widget<Text>(find.byKey(const Key('active_count'))).data, '2');
    expect(find.text('Garantisi 90 gün içinde bitenler'), findsOneWidget);
    expect(find.text('16 gün kaldı'), findsOneWidget);
    // product list loaded in the shell → device reminders synced
    expect(scheduler.synced, containsAll(['p1', 'p2']));
  });

  testWidgets('manual add product computes the warranty end date and saves',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(wrap(const AddProductScreen(), repo));
    await tester.tap(find.byKey(const Key('manual')));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('f_name')), 'Philips Airfryer');
    await tester.tap(find.byKey(const Key('f_date')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('OK'));
    await tester.pumpAndSettle();

    final now = DateTime.now();
    final expected = formatDate(warrantyEnd(
        DateTime.utc(now.year, now.month, now.day), defaultWarrantyMonths));
    expect(find.text(expected), findsOneWidget);

    await tester.ensureVisible(find.byKey(const Key('f_submit')));
    await tester.tap(find.byKey(const Key('f_submit')));
    await tester.pumpAndSettle();

    expect(repo.created.single.name, 'Philips Airfryer');
    expect(repo.created.single.warrantyMonths, 24);
    // navigated to the detail screen of the new product
    expect(find.text('Philips Airfryer'), findsWidgets);
    expect(find.text('Satın alma tarihi'), findsOneWidget);
  });

  testWidgets('add product form requires name and date', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(wrap(const AddProductScreen(), repo));
    await tester.tap(find.byKey(const Key('manual')));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('f_submit')));
    await tester.tap(find.byKey(const Key('f_submit')));
    await tester.pumpAndSettle();
    expect(find.text('Ürün adı gerekli'), findsOneWidget);
    expect(find.text('Satın alma tarihi gerekli'), findsOneWidget);
    expect(repo.created, isEmpty);
  });

  testWidgets('dashboard shows totals and category distribution',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    final d = await repo.dashboard();
    await tester
        .pumpWidget(MaterialApp(home: Scaffold(body: DashboardView(d: d))));
    expect(find.text('Toplam ürün'), findsOneWidget);
    expect(find.text('Aktif garanti'), findsOneWidget);
    expect(find.text('Garanti dışı'), findsOneWidget);
    expect(find.text('21.498,90 TL'), findsOneWidget);
    expect(find.text('Monitör'), findsOneWidget);
    expect(find.text('Kulaklık'), findsOneWidget);
  });

  testWidgets('broken product flow creates a service record with checklist',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester
        .pumpWidget(wrap(ReportIssueScreen(product: monitorProduct()), repo));
    await tester.ensureVisible(find.byKey(const Key('create_service')));
    await tester.tap(find.byKey(const Key('create_service')));
    await tester.pump();
    expect(find.text('Lütfen sorunu seçin'), findsOneWidget);

    await tester.tap(find.byKey(const Key('problem_screen_lines')));
    await tester.pump();
    await tester.ensureVisible(find.byKey(const Key('create_service')));
    await tester.tap(find.byKey(const Key('create_service')));
    await tester.pumpAndSettle();

    expect(find.text('Servis kaydı oluşturuldu'), findsOneWidget);
    expect(find.text('Sorun: Ekranda çizgiler'), findsOneWidget);
    expect(find.text('Garanti belgesi'), findsOneWidget);
    expect(find.text('Belgeleri PDF olarak hazırla'), findsOneWidget);
  });

  test('fake picker bytes type', () {
    expect(PickedImage('p', 'n', Uint8List(1)).bytes.length, 1);
  });
}
