import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import '../products/product_detail_screen.dart';
import '../settings/settings_screen.dart';

/// Ana ekran: selamlama, aktif garanti sayısı, yakında bitenler, kategoriler.
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key, this.onShowAll});

  final VoidCallback? onShowAll;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final dashboard = ref.watch(dashboardProvider);
    final expiring = ref.watch(expiringProvider);
    final user = ref.watch(authControllerProvider).user;
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Warranty Wallet'),
        actions: [
          IconButton(
            tooltip: 'Ayarlar',
            icon: const Icon(Icons.settings_outlined),
            onPressed: () => Navigator.of(context).push(
              MaterialPageRoute<void>(builder: (_) => const SettingsScreen()),
            ),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          refreshProducts(ref);
          await ref.read(dashboardProvider.future);
        },
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text(
              dashboard.valueOrNull?.greeting ??
                  (user?.firstName.isNotEmpty == true
                      ? 'Merhaba, ${user!.firstName}'
                      : 'Merhaba'),
              key: const Key('greeting'),
              style: theme.textTheme.headlineSmall,
            ),
            const SizedBox(height: 16),
            dashboard.when(
              data: (d) => _SummaryCard(d: d),
              loading: () => const SizedBox(
                  height: 96,
                  child: Center(child: CircularProgressIndicator())),
              error: (e, _) => ErrorState(
                  error: e, onRetry: () => ref.invalidate(dashboardProvider)),
            ),
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                    child: Text('Garanti yakında bitiyor',
                        style: theme.textTheme.titleMedium)),
                if (onShowAll != null)
                  TextButton(onPressed: onShowAll, child: const Text('Tümü')),
              ],
            ),
            expiring.when(
              data: (items) => items.isEmpty
                  ? const Padding(
                      padding: EdgeInsets.symmetric(vertical: 12),
                      child: Text(
                          'Önümüzdeki 90 gün içinde garantisi biten ürün yok. 🎉'),
                    )
                  : Column(children: [for (final p in items) _ExpiringTile(p)]),
              loading: () => const LinearProgressIndicator(),
              error: (e, _) => Text(errorMessage(e)),
            ),
            const SizedBox(height: 24),
            Text('Kategoriler', style: theme.textTheme.titleMedium),
            const SizedBox(height: 8),
            dashboard.maybeWhen(
              data: (d) => d.byCategory.isEmpty
                  ? EmptyState(
                      icon: Icons.receipt_long,
                      title: 'Henüz ürün eklemedin',
                      message:
                          'Fişin fotoğrafını çek, gerisini uygulama halletsin.',
                    )
                  : Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        for (final c in d.byCategory)
                          Chip(
                            avatar: Icon(categoryIcon(c.category), size: 18),
                            label: Text('${c.name} (${c.count})'),
                          ),
                      ],
                    ),
              orElse: () => const SizedBox.shrink(),
            ),
            const SizedBox(height: 80),
          ],
        ),
      ),
    );
  }
}

class _SummaryCard extends StatelessWidget {
  const _SummaryCard({required this.d});

  final Dashboard d;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      color: theme.colorScheme.primaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            Icon(Icons.verified_user,
                size: 40, color: theme.colorScheme.onPrimaryContainer),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '${d.activeWarranties}',
                    key: const Key('active_count'),
                    style: theme.textTheme.displaySmall
                        ?.copyWith(color: theme.colorScheme.onPrimaryContainer),
                  ),
                  Text(
                    'aktif garanti • ${d.totalProducts} ürün',
                    style: theme.textTheme.bodyLarge
                        ?.copyWith(color: theme.colorScheme.onPrimaryContainer),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ExpiringTile extends StatelessWidget {
  const _ExpiringTile(this.p);

  final Product p;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        leading: Icon(categoryIcon(p.category)),
        title: Text(p.name),
        subtitle: Text(categoryName(p.category)),
        trailing: DaysLeftText(p.daysLeft, p.status),
        onTap: () => Navigator.of(context).push(
          MaterialPageRoute<void>(
              builder: (_) => ProductDetailScreen(productId: p.id)),
        ),
      ),
    );
  }
}
