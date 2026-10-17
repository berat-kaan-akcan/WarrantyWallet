import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/app.dart';
import 'package:warranty_wallet/models/models.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

const _unverified = AppUser(
    id: 'u1',
    email: 'berat@test.io',
    name: 'Berat Kaan',
    role: 'user',
    emailVerified: false);

void main() {
  test('email_verified_at parsing', () {
    expect(
        AppUser.fromJson({'id': 'u1', 'email_verified_at': null}).emailVerified,
        isFalse);
    expect(
        AppUser.fromJson(
                {'id': 'u1', 'email_verified_at': '2026-10-17T10:00:00Z'})
            .emailVerified,
        isTrue);
    expect(AppUser.fromJson({'id': 'u1'}).emailVerified, isTrue);
  });

  testWidgets('unverified user sees banner, can resend and confirm',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()..user = _unverified;
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(repo, token: 'tok'),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('verify_banner')), findsOneWidget);
    await tester.tap(find.byKey(const Key('verify_resend')));
    await tester.pumpAndSettle();
    expect(repo.resendCalls, 1);
    expect(find.text('Doğrulama bağlantısı berat@test.io adresine gönderildi'),
        findsOneWidget);

    await tester.tap(find.byKey(const Key('verify_check')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Henüz doğrulanmamış'), findsOneWidget);
    expect(find.byKey(const Key('verify_banner')), findsOneWidget);

    repo.user = testUser;
    await tester.tap(find.byKey(const Key('verify_check')));
    await tester.pumpAndSettle();
    expect(
        find.text('Teşekkürler, e-posta adresiniz doğrulandı'), findsOneWidget);
    expect(find.byKey(const Key('verify_banner')), findsNothing);
  });

  testWidgets('verified user sees no banner', (tester) async {
    useTallScreen(tester);
    await tester.pumpWidget(ProviderScope(
        overrides: overrides(FakeRepository(), token: 'tok'),
        child: const WarrantyWalletApp()));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('verify_banner')), findsNothing);
  });
}
