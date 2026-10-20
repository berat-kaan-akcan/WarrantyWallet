import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Dashboard: toplam ürün, aktif garanti, garanti dışı, toplam harcama, kategori dağılımı.
class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key, this.onOpenProducts});

  /// Switches to the products tab; tiles set its filter first.
  final VoidCallback? onOpenProducts;

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
          child: DashboardView(
            d: d,
            onOpenProducts: onOpenProducts == null
                ? null
                : (q) {
                    ref.read(productListQueryProvider.notifier).state = q;
                    onOpenProducts!();
                  },
          ),
        ),
        loading: () => const LoadingView(),
        error: (e, _) => ErrorState(
            error: e, onRetry: () => ref.invalidate(dashboardProvider)),
      ),
    );
  }
}

/// Stateless dashboard body (widget-tested directly).
class DashboardView extends StatelessWidget {
  const DashboardView({super.key, required this.d, this.onOpenProducts});

  final Dashboard d;

  /// Opens the product list with the given filter.
  final ValueChanged<ProductQuery>? onOpenProducts;

  VoidCallback? open(ProductQuery q) =>
      onOpenProducts == null ? null : () => onOpenProducts!(q);

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
                icon: Icons.inventory_2,
                onTap: open(const ProductQuery())),
            _StatTile(
              key: const Key('stat_active'),
              label: 'Aktif garanti',
              value: '${d.activeWarranties}',
              icon: Icons.verified_user,
              color: const Color(0xFF2E7D32),
              onTap: open(const ProductQuery(status: 'active')),
            ),
            _StatTile(
              key: const Key('stat_expired'),
              label: 'Garanti dışı',
              value: '${d.expired}',
              icon: Icons.gpp_bad,
              color: const Color(0xFFC62828),
              onTap: open(const ProductQuery(status: 'expired')),
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
        if (d.analytics != null) AnalyticsSection(a: d.analytics!),
      ],
    );
  }
}

/// V2 trends: protected value, upcoming expiries, purchases, brands, service.
class AnalyticsSection extends StatelessWidget {
  const AnalyticsSection({super.key, required this.a});

  final DashboardAnalytics a;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final value = a.protectedValue.entries.where((e) => e.value > 0).toList();
    final percent = (a.coverageRate * 100).round();
    Widget header(String text) => Padding(
          padding: const EdgeInsets.only(top: 24, bottom: 8),
          child: Semantics(
              header: true,
              child: Text(text, style: theme.textTheme.titleMedium)),
        );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 16),
        Card(
          key: const Key('protected_value'),
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: MergeSemantics(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Garanti altındaki değer',
                      style: theme.textTheme.bodySmall),
                  const SizedBox(height: 4),
                  Text(
                      value.isEmpty
                          ? formatMoney(0)
                          : value
                              .map((e) => formatMoney(e.value, e.key))
                              .join(' + '),
                      style: theme.textTheme.titleLarge
                          ?.copyWith(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 8),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(4),
                    child: LinearProgressIndicator(
                        value: a.coverageRate.clamp(0, 1), minHeight: 8),
                  ),
                  const SizedBox(height: 4),
                  Text('Ürünlerinizin %$percent kadarının garantisi sürüyor',
                      style: theme.textTheme.bodySmall),
                ],
              ),
            ),
          ),
        ),
        header('Önümüzdeki 12 ayda biten garantiler'),
        MonthBars(
          key: const Key('expiry_chart'),
          months: a.expiryTimeline,
          value: (m) => m.count.toDouble(),
          describe: (m) => '${m.longLabel}: ${m.count} garanti bitiyor',
          color: const Color(0xFFB26A00),
        ),
        header('Son 12 ayın alımları'),
        MonthBars(
          key: const Key('purchase_chart'),
          months: a.monthlyPurchases,
          value: (m) => m.total,
          describe: (m) =>
              '${m.longLabel}: ${m.count} ürün, ${formatMoney(m.total, a.primaryCurrency)}',
        ),
        if (a.serviceTotal > 0) ...[
          header('Servis'),
          Card(
            child: Column(
              children: [
                ListTile(
                  leading: const Icon(Icons.build),
                  title: Text(
                      '${a.serviceTotal} kayıt · ${a.serviceActive} devam ediyor · ${a.serviceResolved} çözüldü'),
                ),
                ListTile(
                  leading: const Icon(Icons.payments_outlined),
                  title: Text(
                      'Onarımlara ödenen: ${formatMoney(a.serviceCost, a.primaryCurrency)}'),
                  subtitle: a.avgResolutionDays == null
                      ? null
                      : Text(
                          'Ortalama çözüm süresi: ${a.avgResolutionDays!.round()} gün'),
                ),
              ],
            ),
          ),
        ],
        if (a.topBrands.isNotEmpty) ...[
          header('En çok ürününüz olan markalar'),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (final (brand, count) in a.topBrands)
                Chip(label: Text('$brand · $count')),
            ],
          ),
        ],
      ],
    );
  }
}

/// Simple accessible bar chart of 12 monthly values (no chart dependency).
class MonthBars extends StatelessWidget {
  const MonthBars({
    super.key,
    required this.months,
    required this.value,
    required this.describe,
    this.color,
  });

  final List<MonthStat> months;
  final double Function(MonthStat) value;
  final String Function(MonthStat) describe;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final c = color ?? theme.colorScheme.primary;
    final maxV = months.fold<double>(0, (m, e) => value(e) > m ? value(e) : m);
    if (maxV == 0) {
      return Text('Bu dönemde kayıt yok.', style: theme.textTheme.bodyMedium);
    }
    return SizedBox(
      height: 120,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          for (final m in months)
            Expanded(
              child: Semantics(
                container: true,
                label: describe(m),
                excludeSemantics: true,
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 2),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      if (m.count > 0)
                        Text('${m.count}', style: theme.textTheme.labelSmall),
                      Container(
                        height: 80 * value(m) / maxV,
                        decoration: BoxDecoration(
                          color: c,
                          borderRadius: const BorderRadius.vertical(
                              top: Radius.circular(3)),
                        ),
                      ),
                      const SizedBox(height: 4),
                      FittedBox(
                        child: Text(m.shortLabel,
                            style: theme.textTheme.labelSmall),
                      ),
                    ],
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _StatTile extends StatelessWidget {
  const _StatTile(
      {super.key,
      required this.label,
      required this.value,
      required this.icon,
      this.color,
      this.onTap});

  final String label;
  final String value;
  final IconData icon;
  final Color? color;

  /// Opens the matching product list; null for tiles without one.
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final c = color ?? theme.colorScheme.primary;
    return MergeSemantics(
      child: Card(
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
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
      ),
    );
  }
}
