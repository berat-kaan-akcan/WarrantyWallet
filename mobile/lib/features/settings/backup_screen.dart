import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Yedekleme: tüm ürünler, servis geçmişi ve belgeler tek bir .zip dosyasında.
class BackupScreen extends ConsumerStatefulWidget {
  const BackupScreen({super.key});

  @override
  ConsumerState<BackupScreen> createState() => _BackupScreenState();
}

class _BackupScreenState extends ConsumerState<BackupScreen> {
  bool _busy = false;
  ImportSummary? _result;

  Future<void> _run(Future<void> Function() action) async {
    setState(() => _busy = true);
    try {
      await action();
    } on ApiException catch (e) {
      if (mounted) showSnack(context, e.userMessage, error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _export() => _run(() async {
        final bytes = await ref.read(repositoryProvider).exportBackup();
        final name = 'warranty-wallet-yedek-${isoDate(DateTime.now())}.zip';
        await ref.read(backupFilesProvider).share(bytes, name);
      });

  Future<void> _import() => _run(() async {
        final zip = await ref.read(backupFilesProvider).pickZip();
        if (zip == null) return;
        final res = await ref.read(repositoryProvider).importBackup(zip);
        ref
          ..invalidate(productsProvider)
          ..invalidate(expiringProvider)
          ..invalidate(dashboardProvider)
          ..invalidate(serviceHistoryProvider);
        if (!mounted) return;
        setState(() => _result = res);
        showSnack(
            context,
            res.products == 0 && res.skippedDuplicates > 0
                ? 'Yedekteki ürünler zaten hesabınızda'
                : '${res.products} ürün geri yüklendi');
      });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final r = _result;
    return Scaffold(
      appBar: AppBar(title: const Text('Yedekleme')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const Text(
              'Yedek; tüm ürünlerinizi, servis geçmişinizi ve fatura, garanti belgesi gibi dosyalarınızı tek bir .zip dosyasında toplar. '
              'Dosyayı Google Drive, iCloud Drive veya e-postaya kaydedebilirsiniz.'),
          const SizedBox(height: 16),
          FilledButton.icon(
            key: const Key('backup_export'),
            onPressed: _busy ? null : _export,
            icon: const Icon(Icons.cloud_upload_outlined),
            label: const Text('Yedek oluştur ve kaydet'),
          ),
          const SizedBox(height: 8),
          OutlinedButton.icon(
            key: const Key('backup_import'),
            onPressed: _busy ? null : _import,
            icon: const Icon(Icons.restore),
            label: const Text('Yedekten geri yükle'),
          ),
          const SizedBox(height: 8),
          Text(
              'Geri yükleme mevcut kayıtlarınızı silmez; hesabınızda zaten olan ürünler (aynı ad, alış tarihi ve seri numarası) atlanır.',
              style: theme.textTheme.bodySmall),
          if (_busy)
            const Padding(
              padding: EdgeInsets.only(top: 24),
              child: Center(child: CircularProgressIndicator()),
            ),
          if (r != null)
            Card(
              key: const Key('backup_result'),
              margin: const EdgeInsets.only(top: 24),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Geri yükleme tamamlandı',
                        style: theme.textTheme.titleMedium),
                    const SizedBox(height: 8),
                    Text(
                        '${r.products} ürün, ${r.serviceRecords} servis kaydı, '
                        '${r.documents} belge eklendi.'),
                    if (r.skippedDuplicates > 0)
                      Text('${r.skippedDuplicates} ürün zaten vardı, atlandı.'),
                    if (r.failed > 0) ...[
                      Text('${r.failed} kayıt eklenemedi:',
                          style: TextStyle(color: theme.colorScheme.error)),
                      for (final e in r.errors)
                        Text('• $e', style: theme.textTheme.bodySmall),
                    ],
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}
