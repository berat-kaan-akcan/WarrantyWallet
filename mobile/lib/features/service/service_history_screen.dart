import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import '../products/document_viewer.dart';

/// Servis geçmişi: özet, durum filtresi ve kayıt listesi.
class ServiceHistoryScreen extends ConsumerWidget {
  const ServiceHistoryScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(serviceHistoryProvider);
    final filter = ref.watch(serviceStatusFilterProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Servis Geçmişi')),
      body: history.when(
        skipLoadingOnReload: true,
        data: (h) => RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(serviceHistoryProvider);
            await ref.read(serviceHistoryProvider.future);
          },
          child: ListView(
            children: [
              if (h.summary.total > 0) ServiceSummaryCard(summary: h.summary),
              if (h.summary.total > 0)
                SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: Row(
                    children: [
                      for (final s in <ServiceStatus?>[
                        null,
                        ...ServiceStatus.values
                      ])
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 4),
                          child: ChoiceChip(
                            key: Key('filter_${s?.api ?? 'all'}'),
                            label: Text(s == null
                                ? 'Tümü (${h.summary.total})'
                                : '${s.label} (${h.summary.count(s)})'),
                            selected: filter == s,
                            onSelected: (_) => ref
                                .read(serviceStatusFilterProvider.notifier)
                                .state = s,
                          ),
                        ),
                    ],
                  ),
                ),
              if (h.items.isEmpty)
                EmptyState(
                  icon: Icons.build_outlined,
                  title: filter == null
                      ? 'Servis kaydı yok'
                      : 'Bu durumda kayıt yok',
                  message: filter == null
                      ? 'Bir ürün arızalandığında ürün detayından "Servis Kaydı Oluştur"a dokunun.'
                      : null,
                )
              else
                for (final r in h.items) ...[
                  ServiceRecordTile(record: r),
                  const Divider(height: 1),
                ],
            ],
          ),
        ),
        loading: () => const LoadingView(),
        error: (e, _) => ErrorState(
            error: e, onRetry: () => ref.invalidate(serviceHistoryProvider)),
      ),
    );
  }
}

/// Totals across all service records.
class ServiceSummaryCard extends StatelessWidget {
  const ServiceSummaryCard({super.key, required this.summary});

  final ServiceSummary summary;

  @override
  Widget build(BuildContext context) {
    final avg = summary.avgResolutionDays;
    return Card(
      margin: const EdgeInsets.fromLTRB(16, 12, 16, 8),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 12),
        child: Row(
          children: [
            _stat(context, '${summary.active}', 'Devam eden'),
            _stat(
                context, '${summary.count(ServiceStatus.resolved)}', 'Çözülen'),
            _stat(context, formatMoney(summary.totalCost), 'Toplam ücret'),
            _stat(context, avg == null ? '-' : '${avg.round()} gün',
                'Ort. çözüm'),
          ],
        ),
      ),
    );
  }

  Widget _stat(BuildContext context, String value, String label) => Expanded(
        child: MergeSemantics(
          child: Column(
            children: [
              Text(value,
                  textAlign: TextAlign.center,
                  style: Theme.of(context)
                      .textTheme
                      .titleMedium
                      ?.copyWith(fontWeight: FontWeight.w700)),
              Text(label,
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      );
}

/// Colour for a service status.
Color serviceStatusColor(ServiceStatus s) {
  switch (s) {
    case ServiceStatus.open:
      return const Color(0xFF1565C0);
    case ServiceStatus.inService:
      return const Color(0xFFB26A00);
    case ServiceStatus.resolved:
      return const Color(0xFF2E7D32);
    case ServiceStatus.cancelled:
      return Colors.grey;
  }
}

/// One service record row; tap to open its detail and timeline.
class ServiceRecordTile extends ConsumerWidget {
  const ServiceRecordTile({super.key, required this.record});

  final ServiceRecord record;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final r = record;
    final color = serviceStatusColor(r.status);
    return ListTile(
      leading: CircleAvatar(
        backgroundColor: color.withValues(alpha: 0.15),
        child: Icon(Icons.build, color: color),
      ),
      title: Text(r.productName.isEmpty ? r.problem.label : r.productName),
      subtitle: Text([
        r.problem.label,
        if (r.serviceName.isNotEmpty) r.serviceName,
        if (r.cost > 0) formatMoney(r.cost),
        formatDate(r.serviceDate ?? r.createdAt),
      ].join(' • ')),
      trailing: Text(r.status.label,
          style: TextStyle(color: color, fontWeight: FontWeight.w600)),
      onTap: () => Navigator.of(context).push(MaterialPageRoute<void>(
          builder: (_) => ServiceRecordDetailScreen(recordId: r.id))),
    );
  }
}

/// Bottom sheet to update status / service name / date / notes.
class ServiceUpdateSheet extends ConsumerStatefulWidget {
  const ServiceUpdateSheet({super.key, required this.record});

  final ServiceRecord record;

  @override
  ConsumerState<ServiceUpdateSheet> createState() => _ServiceUpdateSheetState();
}

class _ServiceUpdateSheetState extends ConsumerState<ServiceUpdateSheet> {
  late ServiceStatus _status = widget.record.status;
  late final _serviceName =
      TextEditingController(text: widget.record.serviceName);
  late final _notes = TextEditingController(text: widget.record.notes);
  late final _cost = TextEditingController(
      text: widget.record.cost > 0
          ? widget.record.cost.toStringAsFixed(2).replaceAll('.', ',')
          : '');
  final _eventNote = TextEditingController();
  String? _costError;
  late DateTime? _date = widget.record.serviceDate;
  bool _saving = false;

  @override
  void dispose() {
    _serviceName.dispose();
    _notes.dispose();
    _cost.dispose();
    _eventNote.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final cost = parseMoneyInput(_cost.text);
    if (cost == null) {
      setState(() => _costError = 'Geçerli bir tutar girin');
      return;
    }
    setState(() {
      _costError = null;
      _saving = true;
    });
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).updateServiceRecord(
            widget.record.id,
            status: _status,
            serviceName: _serviceName.text.trim(),
            serviceDate: _date,
            notes: _notes.text.trim(),
            cost: cost,
            eventNote:
                _status != widget.record.status ? _eventNote.text.trim() : null,
          );
      ref.invalidate(serviceHistoryProvider);
      ref.invalidate(serviceRecordProvider(widget.record.id));
      ref.invalidate(productServiceRecordsProvider(widget.record.productId));
      ref.invalidate(dashboardProvider);
      navigator.pop();
    } on ApiException catch (e) {
      if (mounted) showSnack(context, e.userMessage, error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text('Servis kaydını güncelle',
                  style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                children: [
                  for (final s in ServiceStatus.values)
                    ChoiceChip(
                      label: Text(s.label),
                      selected: _status == s,
                      onSelected: (_) => setState(() => _status = s),
                    ),
                ],
              ),
              if (_status != widget.record.status) ...[
                const SizedBox(height: 12),
                TextField(
                  key: const Key('event_note'),
                  controller: _eventNote,
                  maxLength: 500,
                  decoration: const InputDecoration(
                      labelText: 'Durum notu (isteğe bağlı)',
                      hintText: 'Örn. Servise teslim edildi',
                      border: OutlineInputBorder()),
                ),
              ],
              const SizedBox(height: 12),
              TextField(
                controller: _serviceName,
                decoration: const InputDecoration(
                    labelText: 'Servis adı', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 12),
              OutlinedButton.icon(
                icon: const Icon(Icons.event),
                label: Text(_date == null
                    ? 'Servis tarihi seç'
                    : 'Servis tarihi: ${formatDate(_date)}'),
                onPressed: () async {
                  final now = DateTime.now();
                  final picked = await showDatePicker(
                    context: context,
                    initialDate: _date ?? now,
                    firstDate: DateTime(2000),
                    lastDate: DateTime(now.year + 1),
                  );
                  if (picked != null && mounted) {
                    setState(() => _date =
                        DateTime.utc(picked.year, picked.month, picked.day));
                  }
                },
              ),
              const SizedBox(height: 12),
              TextField(
                key: const Key('service_cost'),
                controller: _cost,
                keyboardType:
                    const TextInputType.numberWithOptions(decimal: true),
                decoration: InputDecoration(
                    labelText: 'Ödenen ücret (TL)',
                    helperText: 'Garanti kapsamında ücretsizse boş bırakın',
                    errorText: _costError,
                    border: const OutlineInputBorder()),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _notes,
                maxLines: 3,
                decoration: const InputDecoration(
                    labelText: 'Notlar', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 16),
              FilledButton(
                  key: const Key('service_save'),
                  onPressed: _saving ? null : _save,
                  child: const Text('Kaydet')),
            ],
          ),
        ),
      ),
    );
  }
}

/// One service record with its status timeline.
class ServiceRecordDetailScreen extends ConsumerWidget {
  const ServiceRecordDetailScreen({super.key, required this.recordId});

  final String recordId;

  Future<void> _delete(
      BuildContext context, WidgetRef ref, ServiceRecord r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Servis kaydı silinsin mi?'),
        content: const Text(
            'Kayıt ve zaman çizelgesi silinir. Eklenen fotoğraflar ürünün belgelerinde kalır.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Vazgeç')),
          TextButton(
              key: const Key('confirm_delete'),
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Sil')),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).deleteServiceRecord(r.id);
      ref.invalidate(serviceHistoryProvider);
      ref.invalidate(productServiceRecordsProvider(r.productId));
      ref.invalidate(dashboardProvider);
      navigator.pop();
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  /// The service pack (invoice, warranty card, timeline) is needed again at the service desk.
  Future<void> _sharePdf(
      BuildContext context, WidgetRef ref, ServiceRecord r) async {
    try {
      final bytes = await ref
          .read(repositoryProvider)
          .servicePack(r.productId, serviceRecordId: r.id);
      await shareBytes(bytes, 'servis-paketi.pdf');
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    } on Exception {
      if (context.mounted) showSnack(context, 'PDF paylaşılamadı', error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final record = ref.watch(serviceRecordProvider(recordId));
    return Scaffold(
      appBar: AppBar(
        title: const Text('Servis kaydı'),
        actions: [
          if (record.valueOrNull != null)
            IconButton(
              key: const Key('service_pdf'),
              tooltip: 'Servis paketi PDF',
              icon: const Icon(Icons.picture_as_pdf_outlined),
              onPressed: () => _sharePdf(context, ref, record.value!),
            ),
          if (record.valueOrNull != null)
            IconButton(
              key: const Key('delete_service'),
              tooltip: 'Sil',
              icon: const Icon(Icons.delete_outline),
              onPressed: () => _delete(context, ref, record.value!),
            ),
        ],
      ),
      body: record.when(
        data: (r) => _body(context, r),
        loading: () => const LoadingView(),
        error: (e, _) => ErrorState(
            error: e,
            onRetry: () => ref.invalidate(serviceRecordProvider(recordId))),
      ),
    );
  }

  Widget _body(BuildContext context, ServiceRecord r) {
    final theme = Theme.of(context);
    final color = serviceStatusColor(r.status);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(r.productName.isEmpty ? r.problem.label : r.productName,
            style: theme.textTheme.titleLarge),
        const SizedBox(height: 4),
        Text(r.status.label,
            style: TextStyle(color: color, fontWeight: FontWeight.w700)),
        const SizedBox(height: 8),
        InfoRow('Sorun', r.problem.label),
        if (r.description.isNotEmpty) InfoRow('Açıklama', r.description),
        InfoRow('Garanti', r.underWarranty ? 'Kapsamında' : 'Kapsam dışı'),
        if (r.serviceName.isNotEmpty) InfoRow('Servis', r.serviceName),
        if (r.serviceDate != null)
          InfoRow('Servis tarihi', formatDate(r.serviceDate)),
        InfoRow(
            'Ücret', r.cost > 0 ? formatMoney(r.cost) : 'Ücretsiz / girilmedi'),
        InfoRow('Açılış', formatDate(r.createdAt)),
        if (r.resolutionDays != null)
          InfoRow('Çözüm süresi', '${r.resolutionDays} gün'),
        if (r.notes.isNotEmpty) InfoRow('Notlar', r.notes),
        const SizedBox(height: 12),
        FilledButton.icon(
          key: const Key('update_service'),
          onPressed: () => showModalBottomSheet<void>(
            context: context,
            isScrollControlled: true,
            builder: (_) => ServiceUpdateSheet(record: r),
          ),
          icon: const Icon(Icons.edit),
          label: const Text('Durumu güncelle'),
        ),
        const SizedBox(height: 24),
        Semantics(
            header: true,
            child: Text('Zaman çizelgesi', style: theme.textTheme.titleMedium)),
        if (r.events.isEmpty)
          const Padding(
              padding: EdgeInsets.symmetric(vertical: 8),
              child: Text('Henüz durum değişikliği yok.'))
        else
          for (final e in r.events.reversed)
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: Icon(Icons.circle,
                  size: 14, color: serviceStatusColor(e.status)),
              title: Text(e.status.label),
              subtitle: Text([
                formatDateTime(e.createdAt),
                if (e.note.isNotEmpty) e.note,
              ].join('\n')),
            ),
      ],
    );
  }
}
