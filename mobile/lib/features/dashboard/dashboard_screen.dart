import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Dashboard: toplam ürün, aktif garanti, garanti dışı, toplam harcama, kategori dağılımı.
class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final dashboard = ref.watch(dashboardProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Özet')),
      body: dashboard.when(
        data: (d) => RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(dashboardProvider);
            await ref.read(dashboardProvider.future);
          },
          child: DashboardView(d: d),
        ),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => ErrorState(
            error: e, onRetry: () => ref.invalidate(dashboardProvider)),
      ),
    );
  }
}

/// Stateless dashboard body (widget-tested directly).
class DashboardView extends StatelessWidget {
  const DashboardView({super.key, required this.d});

  final Dashboard d;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final spent = d.totalSpent.entries.where((e) => e.value > 0).toList();
    final maxCount =
        d.byCategory.fold<int>(0, (m, c) => c.count > m ? c.count : m);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        GridView.count(
          crossAxisCount: 2,
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          mainAxisSpacing: 12,
          crossAxisSpacing: 12,
          childAspectRatio: 1.5,
          children: [
            _StatTile(
                key: const Key('stat_total'),
                label: 'Toplam ürün',
                value: '${d.totalProducts}',
                icon: Icons.inventory_2),
            _StatTile(
              key: const Key('stat_active'),
              label: 'Aktif garanti',
              value: '${d.activeWarranties}',
              icon: Icons.verified_user,
              color: const Color(0xFF2E7D32),
            ),
            _StatTile(
              key: const Key('stat_expired'),
              label: 'Garanti dışı',
              value: '${d.expired}',
              icon: Icons.gpp_bad,
              color: const Color(0xFFC62828),
            ),
            _StatTile(
              key: const Key('stat_spent'),
              label: 'Toplam harcama',
              value: spent.isEmpty
                  ? formatMoney(0)
                  : spent.map((e) => formatMoney(e.value, e.key)).join('\n'),
              icon: Icons.payments,
            ),
          ],
        ),
        const SizedBox(height: 12),
        Card(
          child: ListTile(
            leading: const Icon(Icons.schedule),
            title: Text(
                '${d.expiringSoon} ürünün garantisi 30 gün içinde bitiyor'),
            subtitle: Text('${d.openServiceRecords} açık servis kaydı'),
          ),
        ),
        const SizedBox(height: 16),
        Text('Kategori dağılımı', style: theme.textTheme.titleMedium),
        const SizedBox(height: 8),
        if (d.byCategory.isEmpty) const Text('Henüz ürün yok.'),
        for (final c in d.byCategory)
          Semantics(
            label: '${c.name}: ${c.count} ürün, ${formatMoney(c.total)}',
            excludeSemantics: true,
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(
                children: [
                  Icon(categoryIcon(c.category), size: 20),
                  const SizedBox(width: 8),
                  SizedBox(
                      width: 110,
                      child: Text(c.name, overflow: TextOverflow.ellipsis)),
                  Expanded(
                    child: ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: LinearProgressIndicator(
                        value: maxCount == 0 ? 0 : c.count / maxCount,
                        minHeight: 12,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  SizedBox(
                      width: 28,
                      child: Text('${c.count}', textAlign: TextAlign.end)),
                ],
              ),
            ),
          ),
      ],
    );
  }
}

class _StatTile extends StatelessWidget {
  const _StatTile(
      {super.key,
      required this.label,
      required this.value,
      required this.icon,
      this.color});

  final String label;
  final String value;
  final IconData icon;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final c = color ?? theme.colorScheme.primary;
    return MergeSemantics(
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Icon(icon, color: c),
              FittedBox(
                fit: BoxFit.scaleDown,
                alignment: Alignment.centerLeft,
                child: Text(value,
                    style: theme.textTheme.titleLarge
                        ?.copyWith(fontWeight: FontWeight.bold)),
              ),
              Text(label, style: theme.textTheme.bodySmall),
            ],
          ),
        ),
      ),
    );
  }
}
