import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/features/service/diagnosis_screen.dart';
import 'package:warranty_wallet/features/service/report_issue_screen.dart';
import 'package:warranty_wallet/models/models.dart';

import 'fakes.dart';
import 'widget_test.dart' show useTallScreen, wrap;

void main() {
  test('Diagnosis.fromJson maps the API response', () {
    final d = Diagnosis.fromJson({
      'summary': 'Özet',
      'likely_causes': [
        {'cause': 'Pil', 'likelihood': 'low', 'explanation': 'x'}
      ],
      'steps': [
        {'title': 'Şarja takın'}
      ],
      'recommend_service': true,
      'provider': 'rules',
      'warnings': ['ai_unavailable'],
    });
    expect(d.summary, 'Özet');
    expect(d.causes.single.likelihoodLabel, 'Düşük olasılık');
    expect(d.steps.single.detail, '');
    expect(d.isAi, isFalse);
    expect(d.aiUnavailable, isTrue);
    expect(d.safetyWarning, '');
  });

  testWidgets('shows causes, steps, safety warning and source', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(wrap(
        DiagnosisScreen(
            product: monitorProduct(),
            problem: ProblemType.screenLines,
            description: 'dikey çizgi'),
        repo));
    await tester.pumpAndSettle();
    expect(repo.diagnoseCalls, ['screen_lines:dikey çizgi']);
    expect(find.byKey(const Key('safety_warning')), findsOneWidget);
    expect(find.text('Panel kablosu'), findsOneWidget);
    expect(find.text('Yeniden başlatın'), findsOneWidget);
    expect(find.text('Ürününüz garanti kapsamında.'), findsOneWidget);
    expect(find.text('Servis kaydı oluştur (önerilir)'), findsOneWidget);
    expect(find.textContaining('Kaynak: Gemini'), findsOneWidget);
  });

  testWidgets('error state retries', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()
      ..diagnoseError = ApiException(429, 'rate_limited', 'slow down');
    await tester.pumpWidget(wrap(
        DiagnosisScreen(
            product: monitorProduct(), problem: ProblemType.wontTurnOn),
        repo));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('retry')), findsOneWidget);
    repo.diagnoseError = null;
    await tester.tap(find.byKey(const Key('retry')));
    await tester.pumpAndSettle();
    expect(find.text('Panel kablosu'), findsOneWidget);
    expect(repo.diagnoseCalls.length, 2);
  });

  testWidgets('report issue → AI assistant → create service record',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester
        .pumpWidget(wrap(ReportIssueScreen(product: monitorProduct()), repo));
    await tester.ensureVisible(find.byKey(const Key('ai_diagnose')));
    await tester.tap(find.byKey(const Key('ai_diagnose')));
    await tester.pump();
    expect(find.text('Lütfen sorunu seçin'), findsOneWidget);

    await tester.tap(find.byKey(const Key('problem_other')));
    await tester.ensureVisible(find.byKey(const Key('ai_diagnose')));
    await tester.tap(find.byKey(const Key('ai_diagnose')));
    await tester.pump();
    expect(find.text('Lütfen sorunu kısaca açıklayın'), findsOneWidget);

    await tester.tap(find.byKey(const Key('problem_screen_lines')));
    await tester.ensureVisible(find.byKey(const Key('ai_diagnose')));
    await tester.tap(find.byKey(const Key('ai_diagnose')));
    await tester.pumpAndSettle();
    expect(find.text('Arıza asistanı'), findsOneWidget);

    await tester
        .ensureVisible(find.byKey(const Key('diagnosis_create_service')));
    await tester.tap(find.byKey(const Key('diagnosis_create_service')));
    await tester.pumpAndSettle();
    expect(find.text('Servis kaydı oluşturuldu'), findsOneWidget);
  });

  testWidgets('"Sorun çözüldü" returns without a service record',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester
        .pumpWidget(wrap(ReportIssueScreen(product: monitorProduct()), repo));
    await tester.tap(find.byKey(const Key('problem_wont_turn_on')));
    await tester.ensureVisible(find.byKey(const Key('ai_diagnose')));
    await tester.tap(find.byKey(const Key('ai_diagnose')));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('diagnosis_done')));
    await tester.tap(find.byKey(const Key('diagnosis_done')));
    await tester.pumpAndSettle();
    expect(find.text('Servis kaydı oluşturuldu'), findsNothing);
    expect(find.byKey(const Key('create_service')), findsOneWidget);
  });
}
