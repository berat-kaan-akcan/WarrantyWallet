import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/features/settings/backup_screen.dart';
import 'package:warranty_wallet/models/models.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

void main() {
  test('import summary parsing', () {
    final s = ImportSummary.fromJson({
      'products': 3,
      'service_records': 1,
      'documents': 4,
      'skipped_duplicates': 2,
      'failed': 1,
      'errors': ['x'],
    });
    expect((s.products, s.documents, s.skippedDuplicates), (3, 4, 2));
    expect(s.errors, ['x']);
    expect(ImportSummary.fromJson({}).products, 0);
  });

  testWidgets('export shares the archive, import shows the summary',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    final files = FakeBackupFiles();
    await tester.pumpWidget(ProviderScope(
      overrides: overrides(repo, backup: files),
      child: const MaterialApp(home: BackupScreen()),
    ));

    await tester.tap(find.byKey(const Key('backup_export')));
    await tester.pumpAndSettle();
    expect(files.shared.keys.single, startsWith('warranty-wallet-yedek-'));
    expect(files.shared.values.single, repo.backup);

    // cancelled picker → nothing imported
    await tester.tap(find.byKey(const Key('backup_import')));
    await tester.pumpAndSettle();
    expect(repo.imported, isEmpty);

    files.toPick = Uint8List.fromList([1, 2, 3]);
    await tester.tap(find.byKey(const Key('backup_import')));
    await tester.pumpAndSettle();
    expect(repo.imported.single, [1, 2, 3]);
    expect(find.byKey(const Key('backup_result')), findsOneWidget);
    expect(
        find.text('2 ürün, 1 servis kaydı, 3 belge eklendi.'), findsOneWidget);
    expect(find.text('1 ürün zaten vardı, atlandı.'), findsOneWidget);
    expect(find.textContaining('dosya türü desteklenmiyor'), findsOneWidget);
    expect(find.text('2 ürün geri yüklendi'), findsOneWidget);
  });
}
