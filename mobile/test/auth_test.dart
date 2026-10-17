import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/features/auth/password_reset_screen.dart';
import 'package:warranty_wallet/features/settings/change_password_screen.dart';
import 'package:warranty_wallet/services/services.dart';
import 'package:warranty_wallet/state/providers.dart';

import 'fakes.dart';
import 'widget_test.dart' show overrides, useTallScreen;

http.Response _json(int status, Object body) =>
    http.Response(jsonEncode(body), status,
        headers: {'content-type': 'application/json'});

void main() {
  group('session refresh', () {
    test('401 triggers one refresh and the request is retried', () async {
      final tokens = MemoryTokenStore()
        ..token = 'old'
        ..refreshToken = 'r1';
      var refreshCalls = 0;
      final mock = MockClient((req) async {
        if (req.url.path.endsWith('/auth/refresh')) {
          refreshCalls++;
          expect(jsonDecode(req.body), {'refresh_token': 'r1'});
          return _json(200, {'token': 'new', 'refresh_token': 'r2'});
        }
        if (req.headers['Authorization'] == 'Bearer new') {
          return _json(200, {'ok': true});
        }
        return _json(401, {
          'error': {'code': 'unauthorized', 'message': 'expired'}
        });
      });
      var expired = 0;
      final refresher = SessionRefresher(
        baseUrl: 'http://t',
        tokens: tokens,
        client: ApiClient(
            baseUrl: 'http://t', tokenProvider: () async => null, client: mock),
      );
      final api = ApiClient(
        baseUrl: 'http://t',
        tokenProvider: tokens.read,
        client: mock,
        refreshSession: refresher.refresh,
        onUnauthorized: () => expired++,
      );
      final results = await Future.wait([api.get('/me'), api.get('/me')]);
      expect(results.every((r) => r['ok'] == true), isTrue);
      expect(refreshCalls, 1, reason: 'concurrent 401s share one refresh');
      expect(tokens.token, 'new');
      expect(tokens.refreshToken, 'r2');
      expect(expired, 0);
    });

    test('failed refresh reports session expiry', () async {
      final tokens = MemoryTokenStore()
        ..token = 'old'
        ..refreshToken = 'used';
      final mock = MockClient((req) async => req.url.path.endsWith('/refresh')
          ? _json(401, {
              'error': {'code': 'invalid_refresh_token', 'message': 'x'}
            })
          : _json(401, {
              'error': {'code': 'unauthorized', 'message': 'x'}
            }));
      var expired = 0;
      final api = ApiClient(
        baseUrl: 'http://t',
        tokenProvider: tokens.read,
        client: mock,
        refreshSession: SessionRefresher(
                baseUrl: 'http://t',
                tokens: tokens,
                client: ApiClient(
                    baseUrl: 'http://t',
                    tokenProvider: () async => null,
                    client: mock))
            .refresh,
        onUnauthorized: () => expired++,
      );
      await expectLater(api.get('/me'),
          throwsA(isA<ApiException>().having((e) => e.status, 'status', 401)));
      expect(expired, 1);
    });

    test('no refresh token means no refresh call', () async {
      final refresher = SessionRefresher(
          baseUrl: 'http://t',
          tokens: MemoryTokenStore(),
          client: ApiClient(
              baseUrl: 'http://t',
              tokenProvider: () async => null,
              client: MockClient((_) async => fail('must not be called'))));
      expect(await refresher.refresh(), isFalse);
    });
  });

  group('auth controller', () {
    test('login stores both tokens and logout revokes and clears', () async {
      final repo = FakeRepository();
      final tokens = MemoryTokenStore();
      final container = ProviderContainer(overrides: [
        repositoryProvider.overrideWithValue(repo),
        tokenStoreProvider.overrideWithValue(tokens),
      ]);
      addTearDown(container.dispose);
      final auth = container.read(authControllerProvider.notifier);
      expect(await auth.login('berat@test.io', 'supersecret'), isTrue);
      expect(tokens.token, 'tok');
      expect(tokens.refreshToken, 'rtok');
      await auth.logout();
      expect(repo.calls, contains('logout:rtok'));
      expect(tokens.token, isNull);
      expect(tokens.refreshToken, isNull);
    });

    test('registers the push token after login and drops it on logout',
        () async {
      final repo = FakeRepository();
      final container = ProviderContainer(overrides: [
        repositoryProvider.overrideWithValue(repo),
        tokenStoreProvider.overrideWithValue(MemoryTokenStore()),
        pushServiceProvider.overrideWithValue(FakePushService('fcm-token-1')),
      ]);
      addTearDown(container.dispose);
      final auth = container.read(authControllerProvider.notifier);
      await auth.login('berat@test.io', 'supersecret');
      await Future<void>.delayed(Duration.zero);
      expect(repo.calls, contains('device+:fcm-token-1:android'));
      await auth.logout();
      expect(repo.calls,
          containsAllInOrder(['device-:fcm-token-1', 'logout:rtok']));
    });

    test('no push token → nothing registered', () async {
      final repo = FakeRepository();
      final container = ProviderContainer(overrides: [
        repositoryProvider.overrideWithValue(repo),
        tokenStoreProvider.overrideWithValue(MemoryTokenStore()),
        pushServiceProvider.overrideWithValue(FakePushService()),
      ]);
      addTearDown(container.dispose);
      await container
          .read(authControllerProvider.notifier)
          .login('berat@test.io', 'supersecret');
      await Future<void>.delayed(Duration.zero);
      expect(repo.calls.where((c) => c.startsWith('device')), isEmpty);
    });
  });

  testWidgets('password reset: request code, then set new password',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(ProviderScope(
      overrides: overrides(repo),
      child: const MaterialApp(home: PasswordResetScreen()),
    ));
    await tester.tap(find.byKey(const Key('send_code')));
    await tester.pump();
    expect(find.text('E-posta gerekli'), findsOneWidget);

    await tester.enterText(
        find.byKey(const Key('reset_email')), 'berat@test.io');
    await tester.tap(find.byKey(const Key('send_code')));
    await tester.pumpAndSettle();
    expect(repo.calls, contains('forgot:berat@test.io'));
    expect(find.byKey(const Key('code_sent')), findsOneWidget);

    await tester.enterText(find.byKey(const Key('reset_code')), 'bad');
    await tester.enterText(
        find.descendant(
            of: find.byKey(const Key('reset_password')),
            matching: find.byType(EditableText)),
        'newpassword1');
    await tester.tap(find.byKey(const Key('reset_submit')));
    await tester.pumpAndSettle();
    expect(find.text('Sıfırlama kodu geçersiz veya süresi dolmuş.'),
        findsOneWidget);

    await tester.enterText(find.byKey(const Key('reset_code')), 'good');
    await tester.tap(find.byKey(const Key('reset_submit')));
    await tester.pumpAndSettle();
    expect(repo.calls, contains('reset:good'));
  });

  testWidgets('change password shows field errors from the API',
      (tester) async {
    useTallScreen(tester);
    final repo = FakeRepository();
    await tester.pumpWidget(ProviderScope(
      overrides: overrides(repo),
      child: const MaterialApp(home: ChangePasswordScreen()),
    ));
    Finder input(String key) => find.descendant(
        of: find.byKey(Key(key)), matching: find.byType(EditableText));
    await tester.enterText(input('current_password'), 'wrongpass1');
    await tester.enterText(input('new_password'), 'newpassword1');
    await tester.tap(find.byKey(const Key('change_submit')));
    await tester.pumpAndSettle();
    expect(find.text('Mevcut şifre hatalı'), findsOneWidget);

    await tester.enterText(input('current_password'), 'supersecret');
    await tester.tap(find.byKey(const Key('change_submit')));
    await tester.pumpAndSettle();
    expect(repo.calls, contains('change'));
  });
}
