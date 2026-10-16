import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/core/formatters.dart';
import 'package:warranty_wallet/features/service/service_history_screen.dart';
import 'package:warranty_wallet/models/models.dart';

import 'fakes.dart';
import 'widget_test.dart' show useTallScreen, wrap;

ServiceRecord _record(String id, ServiceStatus status,
        {double cost = 0, List<ServiceEvent> events = const []}) =>
    ServiceRecord(
      id: id,
      productId: 'p1',
      problem: ProblemType.screenLines,
      status: status,
      productName: 'ASUS VG27AQ',
      serviceName: 'Yetkili Servis',
      cost: cost,
      createdAt: DateTime.utc(2026, 9, 1),
      resolvedAt:
          status == ServiceStatus.resolved ? DateTime.utc(2026, 9, 6) : null,
      events: events,
    );

FakeRepository _repo() => FakeRepository()
  ..services = [
    _record('s1', ServiceStatus.resolved, cost: 450, events: [
      ServiceEvent(
          ServiceStatus.open, 'Kayıt açıldı', DateTime.utc(2026, 9, 1)),
      ServiceEvent(
          ServiceStatus.resolved, 'Panel değişti', DateTime.utc(2026, 9, 6)),
    ]),
    _record('s2', ServiceStatus.open),
  ]
  ..serviceSummary = const ServiceSummary(
      total: 2,
      active: 1,
      byStatus: {ServiceStatus.open: 1, ServiceStatus.resolved: 1},
      totalCost: 450,
      avgResolutionDays: 5);

void main() {
  test('parseMoneyInput accepts Turkish and plain formats', () {
    expect(parseMoneyInput(''), 0);
    expect(parseMoneyInput('1.250,50'), 1250.5);
    expect(parseMoneyInput('1250.5'), 1250.5);
    expect(parseMoneyInput('abc'), isNull);
    expect(parseMoneyInput('-5'), isNull);
    expect(parseMoneyInput('2000000000'), isNull);
  });

  test('service history models parse the API shape', () {
    final s = ServiceSummary.fromJson({
      'total': 3,
      'active': 2,
      'by_status': {'open': 2, 'resolved': 1},
      'total_cost': 99.5,
      'avg_resolution_days': null,
    });
    expect(s.count(ServiceStatus.open), 2);
    expect(s.count(ServiceStatus.cancelled), 0);
    expect(s.avgResolutionDays, isNull);
    final r = ServiceRecord.fromJson({
      'id': 'x',
      'status': 'resolved',
      'problem': 'other',
      'cost': 10,
      'created_at': '2026-09-01T10:00:00Z',
      'resolved_at': '2026-09-04T11:00:00Z',
      'events': [
        {
          'status': 'in_service',
          'note': 'n',
          'created_at': '2026-09-02T10:00:00Z'
        }
      ],
    });
    expect(r.cost, 10);
    expect(r.resolutionDays, 3);
    expect(r.isClosed, isTrue);
    expect(r.events.single.status, ServiceStatus.inService);
  });

  testWidgets('history shows summary, filters by status and opens detail',
      (tester) async {
    useTallScreen(tester);
    final repo = _repo();
    await tester.pumpWidget(wrap(const ServiceHistoryScreen(), repo));
    await tester.pumpAndSettle();
    expect(find.text('Devam eden'), findsOneWidget);
    expect(find.text('5 gün'), findsOneWidget);
    expect(find.text('Tümü (2)'), findsOneWidget);
    expect(find.byType(ServiceRecordTile), findsNWidgets(2));

    await tester.tap(find.byKey(const Key('filter_resolved')));
    await tester.pumpAndSettle();
    expect(find.byType(ServiceRecordTile), findsOneWidget);
    await tester.tap(find.byKey(const Key('filter_cancelled')));
    await tester.pumpAndSettle();
    expect(find.text('Bu durumda kayıt yok'), findsOneWidget);
    await tester.tap(find.byKey(const Key('filter_resolved')));
    await tester.pumpAndSettle();

    await tester.tap(find.byType(ServiceRecordTile));
    await tester.pumpAndSettle();
    expect(find.text('Zaman çizelgesi'), findsOneWidget);
    expect(find.textContaining('Panel değişti'), findsOneWidget);
    expect(find.text('5 gün'), findsOneWidget); // resolution time
  });

  testWidgets('empty history explains how to start', (tester) async {
    await tester
        .pumpWidget(wrap(const ServiceHistoryScreen(), FakeRepository()));
    await tester.pumpAndSettle();
    expect(find.text('Servis kaydı yok'), findsOneWidget);
  });

  testWidgets('status update sends cost and timeline note', (tester) async {
    useTallScreen(tester);
    final repo = _repo();
    await tester.pumpWidget(
        wrap(const ServiceRecordDetailScreen(recordId: 's2'), repo));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('update_service')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('event_note')), findsNothing);
    await tester.tap(find.text('Serviste'));
    await tester.pumpAndSettle();
    await tester.enterText(
        find.byKey(const Key('event_note')), 'Servise teslim edildi');

    await tester.enterText(find.byKey(const Key('service_cost')), 'abc');
    await tester.ensureVisible(find.byKey(const Key('service_save')));
    await tester.tap(find.byKey(const Key('service_save')));
    await tester.pump();
    expect(find.text('Geçerli bir tutar girin'), findsOneWidget);
    expect(repo.serviceUpdates, isEmpty);

    await tester.enterText(find.byKey(const Key('service_cost')), '1.250,50');
    await tester.ensureVisible(find.byKey(const Key('service_save')));
    await tester.tap(find.byKey(const Key('service_save')));
    await tester.pumpAndSettle();
    expect(repo.serviceUpdates.single, {
      'id': 's2',
      'status': ServiceStatus.inService,
      'cost': 1250.5,
      'event_note': 'Servise teslim edildi',
    });
  });

  testWidgets('deleting a record asks for confirmation', (tester) async {
    useTallScreen(tester);
    final repo = _repo();
    await tester.pumpWidget(
        wrap(const ServiceRecordDetailScreen(recordId: 's1'), repo));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('delete_service')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Vazgeç'));
    await tester.pumpAndSettle();
    expect(repo.deletedServices, isEmpty);
    await tester.tap(find.byKey(const Key('delete_service')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirm_delete')));
    await tester.pumpAndSettle();
    expect(repo.deletedServices, ['s1']);
  });
}
