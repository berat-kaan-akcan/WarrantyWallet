import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import '../products/add_product_screen.dart';
import '../products/document_viewer.dart';

/// "Ürünüm Arızalandı": sorun seçimi → fotoğraf → servis kaydı → belge kontrol listesi + PDF.
class ReportIssueScreen extends ConsumerStatefulWidget {
  const ReportIssueScreen({super.key, required this.product});

  final Product product;

  @override
  ConsumerState<ReportIssueScreen> createState() => _ReportIssueScreenState();
}

class _ReportIssueScreenState extends ConsumerState<ReportIssueScreen> {
  ProblemType? _problem;
  final _description = TextEditingController();
  final List<PickedImage> _photos = [];
  bool _busy = false;
  ServiceRecord? _created;

  @override
  void dispose() {
    _description.dispose();
    super.dispose();
  }

  Future<void> _addPhoto() async {
    final picked = await ref.read(imageSourceProvider).pick(camera: true);
    if (picked != null && mounted) setState(() => _photos.add(picked));
  }

  Future<void> _submit() async {
    final problem = _problem;
    if (problem == null) {
      showSnack(context, 'Lütfen sorunu seçin', error: true);
      return;
    }
    setState(() => _busy = true);
    final repo = ref.read(repositoryProvider);
    try {
      final rec = await repo.createServiceRecord(
          widget.product.id, problem, _description.text.trim());
      for (final photo in _photos) {
        await repo.uploadDocument(
            widget.product.id, 'photo', photo.bytes, photo.name,
            serviceRecordId: rec.id);
      }
      if (!mounted) return;
      refreshProducts(ref, widget.product.id);
      setState(() => _created = rec);
    } on ApiException catch (e) {
      if (mounted) showSnack(context, e.userMessage, error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _preparePdf() async {
    final rec = _created;
    if (rec == null) return;
    setState(() => _busy = true);
    try {
      final bytes = await ref
          .read(repositoryProvider)
          .servicePack(widget.product.id, serviceRecordId: rec.id);
      await shareBytes(bytes, 'servis-paketi.pdf');
    } on ApiException catch (e) {
      if (mounted) showSnack(context, e.userMessage, error: true);
    } on Exception {
      if (mounted) showSnack(context, 'PDF paylaşılamadı', error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Ürünüm Arızalandı')),
      body: _created == null ? _form(context) : _result(context, _created!),
    );
  }

  Widget _form(BuildContext context) {
    final theme = Theme.of(context);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(widget.product.name, style: theme.textTheme.titleLarge),
        const SizedBox(height: 4),
        Text(
          widget.product.isCovered
              ? 'Garanti kapsamında'
              : 'Garanti süresi dolmuş — ücretli servis gerekebilir',
          style: TextStyle(
              color: widget.product.isCovered
                  ? Colors.green.shade700
                  : theme.colorScheme.error),
        ),
        const SizedBox(height: 16),
        Text('1. Sorun nedir?', style: theme.textTheme.titleMedium),
        for (final p in ProblemType.values)
          Semantics(
            inMutuallyExclusiveGroup: true,
            selected: _problem == p,
            child: ListTile(
              key: Key('problem_${p.api}'),
              leading: Icon(_problem == p
                  ? Icons.radio_button_checked
                  : Icons.radio_button_unchecked),
              title: Text(p.label),
              selected: _problem == p,
              onTap: () => setState(() => _problem = p),
            ),
          ),
        TextField(
          controller: _description,
          maxLines: 3,
          maxLength: 2000,
          decoration: const InputDecoration(
            labelText: 'Açıklama (isteğe bağlı)',
            hintText: 'Örn. Ekranın sol tarafında dikey çizgiler var',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 8),
        Text('2. Fotoğraf ekle', style: theme.textTheme.titleMedium),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (var i = 0; i < _photos.length; i++)
              Stack(
                children: [
                  ClipRRect(
                    borderRadius: BorderRadius.circular(8),
                    child: Image.memory(_photos[i].bytes,
                        width: 88, height: 88, fit: BoxFit.cover),
                  ),
                  Positioned(
                    right: 0,
                    top: 0,
                    child: IconButton(
                      tooltip: 'Fotoğrafı kaldır',
                      icon: const Icon(Icons.cancel),
                      onPressed: () => setState(() => _photos.removeAt(i)),
                    ),
                  ),
                ],
              ),
            OutlinedButton.icon(
              onPressed: _photos.length >= 5 ? null : _addPhoto,
              icon: const Icon(Icons.add_a_photo),
              label: const Text('Fotoğraf'),
            ),
          ],
        ),
        const SizedBox(height: 24),
        FilledButton.icon(
          key: const Key('create_service'),
          onPressed: _busy ? null : _submit,
          icon: _busy
              ? const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2))
              : const Icon(Icons.build),
          label: const Padding(
              padding: EdgeInsets.symmetric(vertical: 12),
              child: Text('3. Servis kaydı oluştur')),
        ),
      ],
    );
  }

  Widget _result(BuildContext context, ServiceRecord rec) {
    final theme = Theme.of(context);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Icon(Icons.check_circle, size: 64, color: Colors.green.shade600),
        const SizedBox(height: 8),
        Text('Servis kaydı oluşturuldu',
            textAlign: TextAlign.center, style: theme.textTheme.titleLarge),
        const SizedBox(height: 8),
        Text('Sorun: ${rec.problem.label}', textAlign: TextAlign.center),
        const SizedBox(height: 24),
        if (rec.underWarranty) ...[
          Text('Servise götürmeniz gereken belgeler',
              style: theme.textTheme.titleMedium),
          const SizedBox(height: 8),
          for (final item in rec.checklist)
            CheckboxListTile(
              value: item.ok,
              onChanged: null,
              title: Text(item.label),
              subtitle: item.ok
                  ? null
                  : const Text('Eksik — ürün detayından ekleyebilirsiniz'),
              controlAffinity: ListTileControlAffinity.leading,
            ),
          const SizedBox(height: 16),
          FilledButton.icon(
            key: const Key('prepare_pdf'),
            onPressed: _busy ? null : _preparePdf,
            icon: const Icon(Icons.picture_as_pdf),
            label: const Padding(
              padding: EdgeInsets.symmetric(vertical: 12),
              child: Text('Belgeleri PDF olarak hazırla'),
            ),
          ),
        ] else
          const Card(
            child: ListTile(
              leading: Icon(Icons.info_outline),
              title: Text('Garanti süresi dolmuş'),
              subtitle: Text(
                  'Onarım ücretli olabilir. Servis kaydınız geçmişinizde saklanır.'),
            ),
          ),
        const SizedBox(height: 12),
        OutlinedButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Kapat')),
      ],
    );
  }
}
