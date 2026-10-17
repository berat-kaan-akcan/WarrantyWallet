import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/features/settings/family_screen.dart';
import 'package:warranty_wallet/models/models.dart';
import 'package:warranty_wallet/state/providers.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

Widget _app(FakeRepository repo) => ProviderScope(
      overrides: overrides(repo, token: 'tok'),
      child: const MaterialApp(home: _Signed()),
    );

/// Restores the session first so the screen knows who "me" is.
class _Signed extends ConsumerWidget {
  const _Signed();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authControllerProvider);
    if (!auth.initialized) {
      ref.read(authControllerProvider.notifier).restore();
      return const SizedBox();
    }
    return const FamilyScreen();
  }
}

void main() {
  test('family json parsing', () {
    final f = FamilyInfo.fromJson({
      'family': {'id': 'f1', 'name': 'Ev'},
      'role': 'owner',
      'members': [
        {'id': 'u1', 'name': '', 'email': 'kaan@x.io', 'role': 'member'}
      ],
    });
    expect((f.inFamily, f.name, f.isOwner), (true, 'Ev', true));
    expect(f.members.single.displayName, 'kaan');
    expect(
        FamilyInfo.fromJson({'family': null, 'members': []}).inFamily, isFalse);
    final p = FamilyProduct.fromJson({
      'id': 'p1',
      'name': 'iPhone',
      'category': 'phone',
      'status': 'expiring_soon',
      'owner': {'id': 'u2', 'name': 'Kaan'}
    });
    expect((p.product.name, p.ownerName), ('iPhone', 'Kaan'));
  });

  testWidgets('create a family, invite, see family products', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository()
      ..familyItems = [FamilyProduct(headphones, 'u2', 'Kaan')];
    await tester.pumpWidget(_app(repo));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('family_create')));
    await tester.pumpAndSettle();
    expect(find.text('Aile adı girin'), findsOneWidget);

    await tester.enterText(
        find.byKey(const Key('family_name')), 'Akcan ailesi');
    await tester.tap(find.byKey(const Key('family_create')));
    await tester.pumpAndSettle();
    expect(repo.calls, contains('family+:Akcan ailesi'));
    expect(find.text('Akcan ailesi'), findsOneWidget);
    expect(find.text('Berat Kaan (siz)'), findsOneWidget);
    expect(find.text('Sony WH-1000XM5'), findsOneWidget);
    expect(find.textContaining('Kaan • Bitiş'), findsOneWidget);
    expect(find.text('Aileyi dağıt'), findsOneWidget);

    await tester.tap(find.byKey(const Key('family_invite')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('invite_code')), findsOneWidget);
    expect(find.text('K7QM-2XRP'), findsOneWidget);
    await tester.tap(find.text('Kapat'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('family_leave')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Aile dağıtılacak'), findsOneWidget);
    await tester.tap(find.byKey(const Key('confirm')));
    await tester.pumpAndSettle();
    expect(repo.calls, contains('family-:u1'));
    expect(find.byKey(const Key('family_create')), findsOneWidget);
  });

  testWidgets('join with an invite code', (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(_app(repo));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('family_code')), 'yanlis');
    await tester.tap(find.byKey(const Key('family_join')));
    await tester.pumpAndSettle();
    expect(find.text('Davet kodu hatalı, süresi dolmuş veya kullanılmış.'),
        findsOneWidget);

    await tester.enterText(find.byKey(const Key('family_code')), 'k7qm-2xrp');
    await tester.tap(find.byKey(const Key('family_join')));
    await tester.pumpAndSettle();
    expect(find.text('Akcan ailesi'), findsOneWidget);
    expect(find.text('Aileden ayrıl'), findsOneWidget);
    expect(find.byKey(const Key('family_invite')), findsNothing);
    expect(find.byKey(const Key('remove_u0')), findsNothing);
  });
}
