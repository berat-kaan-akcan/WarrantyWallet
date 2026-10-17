import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/features/dashboard/dashboard_screen.dart';
import 'package:warranty_wallet/models/models.dart';

import 'widget_test.dart' show useTallScreen;

Map<String, dynamic> _json() => {
      'total_products': 3,
      'analytics': {
        'primary_currency': 'TRY',
        'protected_value': {'TRY': 21498.9},
        'coverage_rate': 0.667,
        'expiry_timeline': [
          for (var m = 10; m <= 12; m++)
            {'month': '2026-$m', 'count': m == 11 ? 2 : 0},
          for (var m = 1; m <= 9; m++) {'month': '2027-0$m', 'count': 0},
        ],
        'monthly_purchases': [
          for (var m = 1; m <= 9; m++)
            {'month': '2026-0$m', 'count': m == 9 ? 1 : 0},
          {'month': '2026-10', 'count': 2, 'total': 21498.9},
          {'month': '2026-11', 'count': 0},
          {'month': '2026-12', 'count': 0},
        ],
        'top_brands': [
          {'brand': 'Sony', 'count': 2},
          {'brand': 'ASUS', 'count': 1}
        ],
        'service': {
          'total': 3,
          'active': 1,
          'resolved': 2,
          'total_cost': 450.5,
          'avg_resolution_days': 4.6
        },
      },
    };

final _outer = find.byType(Scrollable).first;

void main() {
  test('analytics parse and month labels', () {
    final d = Dashboard.fromJson(_json());
    final a = d.analytics!;
    expect(a.protectedValue['TRY'], 21498.9);
    expect(a.expiryTimeline, hasLength(12));
    expect(a.expiryTimeline[1].shortLabel, 'Kas');
    expect(a.expiryTimeline[1].longLabel, 'Kasım 2026');
    expect(a.topBrands.first, ('Sony', 2));
    expect(a.avgResolutionDays, 4.6);
    expect(Dashboard.fromJson({'total_products': 1}).analytics, isNull);
    expect(const MonthStat('bozuk', 0).shortLabel, 'bozuk');
  });

  testWidgets('dashboard shows analytics with accessible charts',
      (tester) async {
    useTallScreen(tester);
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: DashboardView(d: Dashboard.fromJson(_json())))));
    await tester.scrollUntilVisible(find.text('Garanti altındaki değer'), 300,
        scrollable: _outer);
    expect(find.text('21.498,90 TL'), findsWidgets);
    expect(find.text('Ürünlerinizin %67 kadarının garantisi sürüyor'),
        findsOneWidget);
    await tester.scrollUntilVisible(find.byKey(const Key('expiry_chart')), 300,
        scrollable: _outer);
    expect(
        find.bySemanticsLabel('Kasım 2026: 2 garanti bitiyor'), findsOneWidget);
    await tester.scrollUntilVisible(find.text('Sony · 2'), 300,
        scrollable: _outer);
    expect(find.text('Ortalama çözüm süresi: 5 gün'), findsOneWidget);
    expect(find.textContaining('450,50 TL'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('empty charts say there is nothing to show', (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body: DashboardView(
                d: Dashboard.fromJson({
      'analytics': {
        'expiry_timeline': [
          {'month': '2026-10', 'count': 0}
        ],
        'monthly_purchases': []
      }
    })))));
    await tester.scrollUntilVisible(
        find.text('Bu dönemde kayıt yok.').first, 300,
        scrollable: _outer);
    expect(find.text('Bu dönemde kayıt yok.'), findsNWidgets(2));
    expect(find.text('Servis'), findsNothing);
  });
}
