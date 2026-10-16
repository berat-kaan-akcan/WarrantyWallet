import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/api_client.dart';
import '../data/repository.dart';
import '../models/models.dart';
import '../services/services.dart';

/// Overridden in main() and in tests.
final tokenStoreProvider = Provider<TokenStore>((ref) => SecureTokenStore());
final ocrServiceProvider = Provider<OcrService>((ref) => MlKitOcrService());
final reminderSchedulerProvider = Provider<ReminderScheduler>((ref) => LocalReminderScheduler());

final repositoryProvider = Provider<WarrantyRepository>((ref) {
  final tokens = ref.watch(tokenStoreProvider);
  final api = ApiClient(
    baseUrl: apiBaseUrl,
    tokenProvider: tokens.read,
    onUnauthorized: () => ref.read(authControllerProvider.notifier).sessionExpired(),
  );
  return ApiWarrantyRepository(api);
});

/// Authentication state.
class AuthState {
  const AuthState({this.user, this.loading = false, this.error, this.initialized = false});

  final AppUser? user;
  final bool loading;
  final String? error;
  final bool initialized;

  bool get isLoggedIn => user != null;

  AuthState copyWith({AppUser? user, bool? loading, String? error, bool? initialized, bool clearUser = false}) =>
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
    } on ApiException catch (e) {
      if (e.isUnauthorized) await _tokens.clear();
      if (!mounted) return;
      state = AuthState(initialized: true, error: e.isNetwork ? e.userMessage : null);
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
      await _tokens.write(session.token);
      if (!mounted) return false;
      state = AuthState(user: session.user, initialized: true);
      return true;
    } on ApiException catch (e) {
      if (!mounted) return false;
      state = state.copyWith(loading: false, error: e.userMessage);
      return false;
    }
  }

  Future<void> logout() async {
    await _tokens.clear();
    state = const AuthState(initialized: true);
  }

  void sessionExpired() {
    if (state.isLoggedIn) {
      _tokens.clear().ignore();
      state = const AuthState(initialized: true, error: 'Oturumunuzun süresi doldu. Lütfen tekrar giriş yapın.');
    }
  }
}

final authControllerProvider =
    StateNotifierProvider<AuthController, AuthState>((ref) => AuthController(ref));

/// Product list filter on the products tab.
class ProductQuery {
  const ProductQuery({this.category = '', this.status = '', this.text = ''});

  final String category;
  final String status;
  final String text;

  @override
  bool operator ==(Object other) =>
      other is ProductQuery && other.category == category && other.status == status && other.text == text;

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

final productsProvider = FutureProvider.family<List<Product>, ProductQuery>((ref, q) async {
  final products =
      await ref.watch(repositoryProvider).products(category: q.category, status: q.status, query: q.text);
  if (q == const ProductQuery()) {
    // full list → keep device reminders in sync
    await ref.read(reminderSchedulerProvider).syncAll(products);
  }
  return products;
});

final expiringProvider = FutureProvider<List<Product>>((ref) => ref.watch(repositoryProvider).expiring(days: 90));

final dashboardProvider = FutureProvider<Dashboard>((ref) => ref.watch(repositoryProvider).dashboard());

final productProvider = FutureProvider.family<Product, String>((ref, id) => ref.watch(repositoryProvider).product(id));

final documentsProvider =
    FutureProvider.family<List<ProductDocument>, String>((ref, id) => ref.watch(repositoryProvider).documents(id));

final serviceHistoryProvider =
    FutureProvider<List<ServiceRecord>>((ref) => ref.watch(repositoryProvider).serviceRecords());

final productServiceRecordsProvider = FutureProvider.family<List<ServiceRecord>, String>(
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
