import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/api_client.dart';
import '../data/repository.dart';
import '../models/models.dart';
import '../services/services.dart';

/// Overridden in main() and in tests.
final tokenStoreProvider = Provider<TokenStore>((ref) => SecureTokenStore());
final ocrServiceProvider = Provider<OcrService>((ref) => MlKitOcrService());
final reminderSchedulerProvider =
    Provider<ReminderScheduler>((ref) => LocalReminderScheduler());
final pushServiceProvider = Provider<PushService>((ref) => NoopPushService());
final linkOpenerProvider =
    Provider<LinkOpener>((ref) => UrlLauncherLinkOpener());
final backupFilesProvider = Provider<BackupFiles>((ref) => DeviceBackupFiles());

final repositoryProvider = Provider<WarrantyRepository>((ref) {
  final tokens = ref.watch(tokenStoreProvider);
  final refresher = SessionRefresher(baseUrl: apiBaseUrl, tokens: tokens);
  final api = ApiClient(
    baseUrl: apiBaseUrl,
    tokenProvider: tokens.read,
    refreshSession: refresher.refresh,
    onUnauthorized: () =>
        ref.read(authControllerProvider.notifier).sessionExpired(),
  );
  return ApiWarrantyRepository(api);
});

/// Exchanges the stored refresh token for a new token pair. Concurrent 401s share a
/// single refresh call because refresh tokens are single use.
class SessionRefresher {
  SessionRefresher({required this.baseUrl, required this.tokens, this.client});

  final String baseUrl;
  final TokenStore tokens;
  final ApiClient? client;
  Future<bool>? _inFlight;

  Future<bool> refresh() => _inFlight ??= _refresh().whenComplete(() {
        _inFlight = null;
      });

  Future<bool> _refresh() async {
    final rt = await tokens.readRefresh();
    if (rt == null || rt.isEmpty) return false;
    final api =
        client ?? ApiClient(baseUrl: baseUrl, tokenProvider: () async => null);
    try {
      final j = await api.post('/auth/refresh', {'refresh_token': rt});
      final token = j['token'] as String?;
      if (token == null) return false;
      await tokens.write(token, refreshToken: j['refresh_token'] as String?);
      return true;
    } on ApiException {
      return false;
    }
  }
}

/// Authentication state.
class AuthState {
  const AuthState(
      {this.user, this.loading = false, this.error, this.initialized = false});

  final AppUser? user;
  final bool loading;
  final String? error;
  final bool initialized;

  bool get isLoggedIn => user != null;

  AuthState copyWith(
          {AppUser? user,
          bool? loading,
          String? error,
          bool? initialized,
          bool clearUser = false}) =>
      AuthState(
        user: clearUser ? null : (user ?? this.user),
        loading: loading ?? this.loading,
        error: error,
        initialized: initialized ?? this.initialized,
      );
}

/// Login/register/logout and session restore.
class AuthController extends StateNotifier<AuthState> {
  AuthController(this._ref) : super(const AuthState());

  final Ref _ref;

  WarrantyRepository get _repo => _ref.read(repositoryProvider);
  TokenStore get _tokens => _ref.read(tokenStoreProvider);

  /// Restores a saved session on app start.
  Future<void> restore() async {
    final token = await _tokens.read();
    if (token == null || token.isEmpty) {
      state = state.copyWith(initialized: true);
      return;
    }
    try {
      final user = await _repo.me();
      if (!mounted) return;
      state = AuthState(user: user, initialized: true);
      unawaited(_registerPush());
    } on ApiException catch (e) {
      if (e.isUnauthorized) await _tokens.clear();
      if (!mounted) return;
      state = AuthState(
          initialized: true, error: e.isNetwork ? e.userMessage : null);
    }
  }

  Future<bool> login(String email, String password) =>
      _authenticate(() => _repo.login(email, password));

  Future<bool> register(String email, String password, String name) =>
      _authenticate(() => _repo.register(email, password, name));

  Future<bool> _authenticate(Future<AuthSession> Function() call) async {
    state = state.copyWith(loading: true);
    try {
      final session = await call();
      await _tokens.write(session.token, refreshToken: session.refreshToken);
      if (!mounted) return false;
      state = AuthState(user: session.user, initialized: true);
      unawaited(_registerPush());
      return true;
    } on ApiException catch (e) {
      if (!mounted) return false;
      state = state.copyWith(loading: false, error: e.userMessage);
      return false;
    }
  }

  String? _pushToken;

  /// Registers this install for server push. Best effort: reminders still arrive as local
  /// notifications when push is unavailable.
  Future<void> _registerPush() async {
    final push = _ref.read(pushServiceProvider);
    try {
      final token = await push.token();
      if (token == null || token.isEmpty) return;
      await _repo.registerDevice(token, push.platform);
      _pushToken = token;
    } catch (_) {}
  }

  Future<void> logout() async {
    final rt = await _tokens.readRefresh();
    final push = _pushToken;
    _pushToken = null;
    try {
      if (push != null) await _repo.unregisterDevice(push);
      await _repo.logout(rt);
    } on ApiException {
      // Signing out locally must work offline too.
    }
    await _tokens.clear();
    state = const AuthState(initialized: true);
  }

  /// Updates the signed-in user after a profile change.
  void userUpdated(AppUser user) {
    if (state.isLoggedIn) state = state.copyWith(user: user);
  }

  void sessionExpired() {
    if (state.isLoggedIn) {
      _tokens.clear().ignore();
      state = const AuthState(
          initialized: true,
          error: 'Oturumunuzun süresi doldu. Lütfen tekrar giriş yapın.');
    }
  }
}

final authControllerProvider = StateNotifierProvider<AuthController, AuthState>(
    (ref) => AuthController(ref));

/// Product list filter on the products tab.
class ProductQuery {
  const ProductQuery({this.category = '', this.status = '', this.text = ''});

  final String category;
  final String status;
  final String text;

  @override
  bool operator ==(Object other) =>
      other is ProductQuery &&
      other.category == category &&
      other.status == status &&
      other.text == text;

  @override
  int get hashCode => Object.hash(category, status, text);
}

final categoriesProvider = FutureProvider<List<ProductCategory>>((ref) async {
  try {
    final cats = await ref.watch(repositoryProvider).categories();
    return cats.isEmpty ? defaultCategories : cats;
  } on ApiException {
    return defaultCategories;
  }
});

final productsProvider =
    FutureProvider.family<List<Product>, ProductQuery>((ref, q) async {
  final products = await ref
      .watch(repositoryProvider)
      .products(category: q.category, status: q.status, query: q.text);
  if (q == const ProductQuery()) {
    // full list → keep device reminders in sync
    await ref.read(reminderSchedulerProvider).syncAll(products);
  }
  return products;
});

final expiringProvider = FutureProvider<List<Product>>(
    (ref) => ref.watch(repositoryProvider).expiring(days: 90));

final dashboardProvider = FutureProvider<Dashboard>(
    (ref) => ref.watch(repositoryProvider).dashboard());

final productProvider = FutureProvider.family<Product, String>(
    (ref, id) => ref.watch(repositoryProvider).product(id));

final documentsProvider = FutureProvider.family<List<ProductDocument>, String>(
    (ref, id) => ref.watch(repositoryProvider).documents(id));

/// Status filter of the service history screen (null = all).
final serviceStatusFilterProvider = StateProvider<ServiceStatus?>((_) => null);

final serviceHistoryProvider = FutureProvider<ServiceHistory>((ref) => ref
    .watch(repositoryProvider)
    .serviceHistory(status: ref.watch(serviceStatusFilterProvider)));

final serviceRecordProvider = FutureProvider.family<ServiceRecord, String>(
    (ref, id) => ref.watch(repositoryProvider).serviceRecord(id));

final productServiceRecordsProvider =
    FutureProvider.family<List<ServiceRecord>, String>(
  (ref, id) => ref.watch(repositoryProvider).serviceRecords(productId: id),
);

/// Invalidates everything that depends on the product list after a write.
void refreshProducts(WidgetRef ref, [String? productId]) {
  ref.invalidate(productsProvider);
  ref.invalidate(expiringProvider);
  ref.invalidate(dashboardProvider);
  ref.invalidate(serviceHistoryProvider);
  if (productId != null) {
    ref.invalidate(productProvider(productId));
    ref.invalidate(documentsProvider(productId));
    ref.invalidate(productServiceRecordsProvider(productId));
  }
}

final familyProvider =
    FutureProvider<FamilyInfo>((ref) => ref.watch(repositoryProvider).family());

final familyProductsProvider = FutureProvider<List<FamilyProduct>>(
    (ref) => ref.watch(repositoryProvider).familyProducts());

/// City typed in the service-center finder, kept for the session.
final serviceCityProvider = StateProvider<String>((_) => '');

final serviceCentersProvider =
    FutureProvider.family<ServiceCenterSearch, String>((ref, productId) => ref
        .watch(repositoryProvider)
        .serviceCenters(productId, city: ref.watch(serviceCityProvider)));
