import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/features/products/add_product_screen.dart';
import 'package:warranty_wallet/features/products/product_list_screen.dart';

import 'fakes.dart';
import 'widget_test.dart' show FakeCodeScanner, overrides, useTallScreen;

Widget _app(FakeRepository repo, FakeCodeScanner scanner, Widget home) =>
    ProviderScope(
      overrides: overrides(repo, scanner: scanner),
      child: MaterialApp(home: home),
    );

void main() {
  testWidgets('scanning a known serial opens the product', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(_app(
        repo, FakeCodeScanner('S/N: M1LMQS123'), const ProductListScreen()));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('scan_lookup')));
    await tester.pumpAndSettle();
    expect(find.text('Seri numarası'), findsOneWidget);
    expect(find.text('M1LMQS123'), findsOneWidget);
  });

  testWidgets('unknown code offers to add the product with the code',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(_app(
        repo,
        FakeCodeScanner('(01)04548736132610(21)NEW999'),
        const ProductListScreen()));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('scan_lookup')));
    await tester.pumpAndSettle();
    expect(find.textContaining('ile kayıtlı ürün bulunamadı'), findsOneWidget);
    await tester.tap(find.text('Ürün ekle'));
    await tester.pumpAndSettle();
    expect(find.byType(AddProductScreen), findsOneWidget);
    await tester.tap(find.byKey(const Key('manual')));
    await tester.pumpAndSettle();
    final serial =
        tester.widget<TextFormField>(find.byKey(const Key('f_serial')));
    final barcode =
        tester.widget<TextFormField>(find.byKey(const Key('f_barcode')));
    expect(serial.controller!.text, 'NEW999');
    expect(barcode.controller!.text, '04548736132610');
  });

  testWidgets('scan button in the form fills barcode', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(
        _app(repo, FakeCodeScanner('4548736132610'), const AddProductScreen()));
    await tester.tap(find.byKey(const Key('manual')));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Barkodu tara'));
    await tester.pumpAndSettle();
    final barcode =
        tester.widget<TextFormField>(find.byKey(const Key('f_barcode')));
    expect(barcode.controller!.text, '4548736132610');
  });
}
