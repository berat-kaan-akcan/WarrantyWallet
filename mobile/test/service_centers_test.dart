import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/features/service/service_centers_screen.dart';
import 'package:warranty_wallet/models/models.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

void main() {
  test('service center parsing', () {
    final r = ServiceCenterSearch.fromJson({
      'brand': 'ASUS',
      'items': [
        {
          'id': 'c1',
          'brand': 'ASUS',
          'name': 'X',
          'city': 'Ankara',
          'district': '',
          'distance_km': 3
        }
      ],
      'maps_search_url': 'https://maps',
    });
    expect(r.items.single.place, 'Ankara');
    expect(r.items.single.distanceKm, 3.0);
    expect(ServiceCenterSearch.fromJson({}).items, isEmpty);
  });

  testWidgets('lists centers, filters by city, opens phone and maps',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    final links = FakeLinkOpener();
    final product = monitorProduct();
    await tester.pumpWidget(ProviderScope(
      overrides: overrides(repo, links: links),
      child: MaterialApp(home: ServiceCentersScreen(product: product)),
    ));
    await tester.pumpAndSettle();

    expect(find.text('ASUS yetkili servisleri'), findsOneWidget);
    expect(find.text('ASUS Kadıköy Servis'), findsOneWidget);
    expect(find.text('Kadıköy, İstanbul • 2.4 km'), findsOneWidget);

    await tester.tap(find.byKey(const Key('call_c1')));
    await tester.pumpAndSettle();
    expect(links.opened.last.toString(), 'tel:+902160000000');

    await tester.enterText(find.byKey(const Key('service_city')), 'Van');
    await tester.tap(find.byKey(const Key('service_search')));
    await tester.pumpAndSettle();
    expect(repo.centerQueries.last, 'p1:Van');
    expect(find.byKey(const Key('no_centers')), findsOneWidget);
    expect(find.text('ASUS Kadıköy Servis'), findsNothing);

    await tester.tap(find.byKey(const Key('maps_search')));
    await tester.pumpAndSettle();
    expect(
        links.opened.last.queryParameters['query'], 'ASUS yetkili servis Van');

    links.result = false;
    await tester.tap(find.byKey(const Key('maps_search')));
    await tester.pumpAndSettle();
    expect(find.text('Bağlantı açılamadı'), findsOneWidget);
  });
}
