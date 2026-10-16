import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Servis geçmişi listesi.
class ServiceHistoryScreen extends ConsumerWidget {
  const ServiceHistoryScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final records = ref.watch(serviceHistoryProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Servis Geçmişi')),
      body: records.when(
        data: (items) => RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(serviceHistoryProvider);
            await ref.read(serviceHistoryProvider.future);
          },
          child: items.isEmpty
              ? ListView(
                  children: const [
                    EmptyState(
                      icon: Icons.build_outlined,
                      title: 'Servis kaydı yok',
                      message:
                          'Bir ürün arızalandığında ürün detayından "Servis Kaydı Oluştur"a dokunun.',
                    ),
                  ],
                )
              : ListView.separated(
                  itemCount: items.length,
                  separatorBuilder: (_, __) => const Divider(height: 1),
                  itemBuilder: (context, i) =>
                      ServiceRecordTile(record: items[i]),
                ),
        ),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => ErrorState(
            error: e, onRetry: () => ref.invalidate(serviceHistoryProvider)),
      ),
    );
  }
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

/// One service record row; tap to update its status.
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
        formatDate(r.serviceDate ?? r.createdAt),
      ].join(' • ')),
      trailing: Text(r.status.label,
          style: TextStyle(color: color, fontWeight: FontWeight.w600)),
      onTap: () => showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        builder: (_) => ServiceUpdateSheet(record: r),
      ),
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
  late DateTime? _date = widget.record.serviceDate;
  bool _saving = false;

  @override
  void dispose() {
    _serviceName.dispose();
    _notes.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).updateServiceRecord(
            widget.record.id,
            status: _status,
            serviceName: _serviceName.text.trim(),
            serviceDate: _date,
            notes: _notes.text.trim(),
          );
      ref.invalidate(serviceHistoryProvider);
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
                controller: _notes,
                maxLines: 3,
                decoration: const InputDecoration(
                    labelText: 'Notlar', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 16),
              FilledButton(
                  onPressed: _saving ? null : _save,
                  child: const Text('Kaydet')),
            ],
          ),
        ),
      ),
    );
  }
}
