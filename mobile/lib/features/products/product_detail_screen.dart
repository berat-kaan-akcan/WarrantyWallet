import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../scan/code_scanner.dart';
import '../../core/api_client.dart';
import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../services/services.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import '../service/report_issue_screen.dart';
import '../service/service_centers_screen.dart';
import '../service/service_history_screen.dart';
import 'add_product_screen.dart';
import 'document_viewer.dart';
import 'product_form.dart';

/// Ürün detay ekranı.
class ProductDetailScreen extends ConsumerWidget {
  const ProductDetailScreen({super.key, required this.productId});

  final String productId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final product = ref.watch(productProvider(productId));
    return product.when(
      loading: () => Scaffold(appBar: AppBar(), body: const LoadingView()),
      error: (e, _) => Scaffold(
        appBar: AppBar(),
        body: ErrorState(
            error: e,
            onRetry: () => ref.invalidate(productProvider(productId))),
      ),
      data: (p) => _ProductDetail(product: p),
    );
  }
}

class _ProductDetail extends ConsumerWidget {
  const _ProductDetail({required this.product});

  final Product product;

  Future<void> _delete(BuildContext context, WidgetRef ref) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Ürünü sil'),
        content:
            Text('"${product.name}" ve tüm belgeleri silinecek. Emin misiniz?'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Vazgeç')),
          FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Sil')),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).deleteProduct(product.id);
      await ref.read(reminderSchedulerProvider).cancelProduct(product.id);
      refreshProducts(ref);
      navigator.pop();
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  Future<void> _edit(BuildContext context, WidgetRef ref) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (ctx) => Scaffold(
          appBar: AppBar(title: const Text('Ürünü düzenle')),
          body: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              ProductForm(
                initial: ProductDraft.fromProduct(product),
                onScan: () => ref.read(codeScannerProvider).scan(context),
                categories: ref.read(categoriesProvider).valueOrNull ??
                    defaultCategories,
                onSubmit: (draft) async {
                  try {
                    final updated = await ref
                        .read(repositoryProvider)
                        .updateProduct(product.id, draft);
                    await ref
                        .read(reminderSchedulerProvider)
                        .syncProduct(updated);
                    refreshProducts(ref, product.id);
                    if (ctx.mounted) Navigator.of(ctx).pop();
                  } on ApiException catch (e) {
                    if (ctx.mounted) showSnack(ctx, e.userMessage, error: true);
                  }
                },
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _openDocument(
      BuildContext context, WidgetRef ref, String type, String label) async {
    final docs = await ref.read(documentsProvider(product.id).future);
    if (!context.mounted) return;
    final matching = docs.where((d) => d.type == type).toList();
    if (matching.isEmpty) {
      final add = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text('$label yok'),
          content: Text(
              'Bu ürün için kayıtlı $label bulunmuyor. Şimdi fotoğrafını eklemek ister misiniz?'),
          actions: [
            TextButton(
                onPressed: () => Navigator.pop(ctx, false),
                child: const Text('Vazgeç')),
            FilledButton(
                onPressed: () => Navigator.pop(ctx, true),
                child: const Text('Ekle')),
          ],
        ),
      );
      if (add == true && context.mounted) {
        await _addDocument(context, ref, type);
      }
      return;
    }
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
          builder: (_) => DocumentViewer(
              documents: matching, title: label, productId: product.id)),
    );
  }

  Future<void> _addDocument(
      BuildContext context, WidgetRef ref, String type) async {
    final camera = await showModalBottomSheet<bool>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              key: const Key('source_camera'),
              leading: const Icon(Icons.photo_camera),
              title: const Text('Fotoğraf çek'),
              onTap: () => Navigator.pop(ctx, true),
            ),
            ListTile(
              key: const Key('source_gallery'),
              leading: const Icon(Icons.photo_library),
              title: const Text('Galeriden seç'),
              onTap: () => Navigator.pop(ctx, false),
            ),
          ],
        ),
      ),
    );
    if (camera == null || !context.mounted) return;
    final picked = await ref.read(imageSourceProvider).pick(camera: camera);
    if (picked == null || !context.mounted) return;
    try {
      await ref
          .read(repositoryProvider)
          .uploadDocument(product.id, type, picked.bytes, picked.name);
      ref.invalidate(documentsProvider(product.id));
      if (context.mounted) showSnack(context, 'Belge kaydedildi');
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  void _showReminders(BuildContext context) {
    final planned = planReminders(product, DateTime.now());
    showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const ListTile(
                title: Text('Hatırlatıcılar'),
                subtitle: Text('Garanti bitiminden 90, 30 ve 7 gün önce')),
            if (planned.isEmpty)
              const ListTile(
                  leading: Icon(Icons.notifications_off),
                  title: Text('Planlanmış hatırlatıcı yok')),
            for (final r in planned)
              ListTile(
                leading: const Icon(Icons.notifications_active),
                title: Text(formatDate(r.at)),
                subtitle: Text(r.body),
              ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final p = product;
    final theme = Theme.of(context);
    final services = ref.watch(productServiceRecordsProvider(p.id));
    return Scaffold(
      appBar: AppBar(
        title: Text(p.name, overflow: TextOverflow.fade),
        actions: [
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'edit') unawaited(_edit(context, ref));
              if (v == 'delete') unawaited(_delete(context, ref));
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'edit', child: Text('Düzenle')),
              PopupMenuItem(value: 'delete', child: Text('Sil')),
            ],
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          refreshProducts(ref, p.id);
          await ref.read(productProvider(p.id).future);
        },
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Row(
              children: [
                CircleAvatar(
                    radius: 28,
                    child: Icon(categoryIcon(p.category), size: 28)),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(p.name, style: theme.textTheme.titleLarge),
                      Text(categoryName(p.category),
                          style: theme.textTheme.bodyMedium),
                    ],
                  ),
                ),
                StatusChip(p.status),
              ],
            ),
            if (p.daysLeft != null)
              Padding(
                padding: const EdgeInsets.only(top: 12),
                child: DaysLeftText(p.daysLeft, p.status),
              ),
            const Divider(height: 32),
            InfoRow('Satın alma tarihi', formatDate(p.purchaseDate),
                icon: Icons.calendar_today),
            InfoRow('Satıcı', p.seller.isEmpty ? '-' : p.seller,
                icon: Icons.storefront),
            InfoRow('Fiyat', formatMoney(p.price, p.currency),
                icon: Icons.payments_outlined),
            InfoRow('Garanti süresi',
                p.warrantyMonths == 0 ? 'Yok' : '${p.warrantyMonths} ay',
                icon: Icons.timelapse),
            InfoRow('Bitiş tarihi', formatDate(p.warrantyEnd),
                icon: Icons.event),
            InfoRow('Durum', statusStyle(p.status).label,
                icon: statusStyle(p.status).icon),
            InfoRow(
                'Seri numarası', p.serialNumber.isEmpty ? '-' : p.serialNumber,
                icon: Icons.qr_code),
            if (p.barcode.isNotEmpty)
              InfoRow('Barkod', p.barcode, icon: Icons.barcode_reader),
            if (p.brand.isNotEmpty || p.model.isNotEmpty)
              InfoRow('Marka / Model',
                  [p.brand, p.model].where((s) => s.isNotEmpty).join(' / '),
                  icon: Icons.label),
            if (p.notes.isNotEmpty)
              InfoRow('Notlar', p.notes, icon: Icons.notes),
            const Divider(height: 32),
            _ActionTile(
              icon: Icons.receipt_long,
              title: 'Faturayı Gör',
              onTap: () => _openDocument(context, ref, 'invoice', 'Fatura'),
            ),
            _ActionTile(
              icon: Icons.verified,
              title: 'Garanti Belgesini Gör',
              onTap: () =>
                  _openDocument(context, ref, 'warranty', 'Garanti belgesi'),
            ),
            _ActionTile(
              icon: Icons.photo_library,
              title: 'Ürün Fotoğrafları',
              onTap: () =>
                  _openDocument(context, ref, 'photo', 'Ürün fotoğrafı'),
            ),
            _ActionTile(
              icon: Icons.add_photo_alternate,
              title: 'Belge Ekle',
              subtitle: 'Fatura, garanti belgesi veya ürün fotoğrafı',
              onTap: () => _chooseDocType(context, ref),
            ),
            _ActionTile(
              icon: Icons.build_circle,
              title: 'Servis Kaydı Oluştur',
              subtitle:
                  p.isCovered ? 'Ürünüm arızalandı' : 'Garanti süresi dolmuş',
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                    builder: (_) => ReportIssueScreen(product: p)),
              ),
            ),
            _ActionTile(
                icon: Icons.home_repair_service,
                title: 'Yetkili Servis Bul',
                subtitle: 'Servis merkezleri ve harita araması',
                onTap: () => Navigator.of(context).push(MaterialPageRoute<void>(
                    builder: (_) => ServiceCentersScreen(product: p)))),
            _ActionTile(
              icon: Icons.notifications,
              title: 'Hatırlatıcılar',
              subtitle: nextReminderText(p, DateTime.now()),
              onTap: () => _showReminders(context),
            ),
            const SizedBox(height: 16),
            Text('Servis geçmişi', style: theme.textTheme.titleMedium),
            services.when(
              data: (items) => items.isEmpty
                  ? const Padding(
                      padding: EdgeInsets.symmetric(vertical: 8),
                      child: Text('Servis kaydı yok.'))
                  : Column(
                      children: [
                        for (final s in items)
                          ListTile(
                            contentPadding: EdgeInsets.zero,
                            leading: const Icon(Icons.history),
                            title: Text(s.problem.label),
                            subtitle: Text(
                                '${s.status.label} • ${formatDate(s.createdAt)}'),
                            trailing: const Icon(Icons.chevron_right),
                            onTap: () => Navigator.of(context).push(
                                MaterialPageRoute<void>(
                                    builder: (_) => ServiceRecordDetailScreen(
                                        recordId: s.id))),
                          ),
                      ],
                    ),
              loading: () =>
                  const LinearProgressIndicator(semanticsLabel: 'Yükleniyor'),
              error: (e, _) => InlineError(
                  error: e,
                  onRetry: () =>
                      ref.invalidate(productServiceRecordsProvider(p.id))),
            ),
            const SizedBox(height: 32),
          ],
        ),
      ),
    );
  }

  Future<void> _chooseDocType(BuildContext context, WidgetRef ref) async {
    final type = await showModalBottomSheet<String>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.receipt_long),
              title: const Text('Fatura / fiş'),
              onTap: () => Navigator.pop(ctx, 'invoice'),
            ),
            ListTile(
              leading: const Icon(Icons.verified),
              title: const Text('Garanti belgesi'),
              onTap: () => Navigator.pop(ctx, 'warranty'),
            ),
            ListTile(
              leading: const Icon(Icons.photo),
              title: const Text('Ürün fotoğrafı'),
              onTap: () => Navigator.pop(ctx, 'photo'),
            ),
          ],
        ),
      ),
    );
    if (type != null && context.mounted) await _addDocument(context, ref, type);
  }
}

class _ActionTile extends StatelessWidget {
  const _ActionTile(
      {required this.icon,
      required this.title,
      required this.onTap,
      this.subtitle});

  final IconData icon;
  final String title;
  final String? subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: Icon(icon, color: Theme.of(context).colorScheme.primary),
      title: Text(title),
      subtitle: subtitle == null ? null : Text(subtitle!),
      trailing: const Icon(Icons.chevron_right),
      onTap: onTap,
    );
  }
}
