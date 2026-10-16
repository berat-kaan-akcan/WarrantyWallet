import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// AI fault assistant: likely causes and safe first steps for a broken product.
/// Pops `true` when the user wants to continue with a service record.
class DiagnosisScreen extends ConsumerStatefulWidget {
  const DiagnosisScreen(
      {super.key,
      required this.product,
      required this.problem,
      this.description = ''});

  final Product product;
  final ProblemType problem;
  final String description;

  @override
  ConsumerState<DiagnosisScreen> createState() => _DiagnosisScreenState();
}

class _DiagnosisScreenState extends ConsumerState<DiagnosisScreen> {
  late Future<Diagnosis> _future;

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<Diagnosis> _load() => ref
      .read(repositoryProvider)
      .diagnose(widget.product.id, widget.problem, widget.description);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Arıza asistanı')),
      body: FutureBuilder<Diagnosis>(
        future: _future,
        builder: (context, snap) {
          if (snap.connectionState != ConnectionState.done) {
            return const Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                LoadingView(),
                SizedBox(height: 16),
                Text('Olası nedenler hazırlanıyor…'),
              ],
            );
          }
          if (snap.hasError) {
            return ErrorState(
                error: snap.error!,
                onRetry: () => setState(() {
                      _future = _load();
                    }));
          }
          return _result(context, snap.data!);
        },
      ),
    );
  }

  Widget _result(BuildContext context, Diagnosis d) {
    final theme = Theme.of(context);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        if (d.safetyWarning.isNotEmpty)
          Card(
            key: const Key('safety_warning'),
            color: theme.colorScheme.errorContainer,
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Semantics(
                liveRegion: true,
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(Icons.warning_amber,
                        color: theme.colorScheme.onErrorContainer),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text('Güvenlik uyarısı: ${d.safetyWarning}',
                          style: TextStyle(
                              color: theme.colorScheme.onErrorContainer,
                              fontWeight: FontWeight.w600)),
                    ),
                  ],
                ),
              ),
            ),
          ),
        Text('${widget.product.name} · ${widget.problem.label}',
            style: theme.textTheme.labelLarge),
        const SizedBox(height: 8),
        Text(d.summary, style: theme.textTheme.bodyLarge),
        const SizedBox(height: 16),
        Semantics(
            header: true,
            child: Text('Olası nedenler', style: theme.textTheme.titleMedium)),
        for (final c in d.causes)
          Card(
            child: ListTile(
              title: Text(c.cause),
              subtitle: Text([c.likelihoodLabel, c.explanation]
                  .where((s) => s.isNotEmpty)
                  .join('\n')),
            ),
          ),
        const SizedBox(height: 16),
        Semantics(
            header: true,
            child: Text('Deneyebilecekleriniz',
                style: theme.textTheme.titleMedium)),
        for (var i = 0; i < d.steps.length; i++)
          ListTile(
            leading: CircleAvatar(radius: 14, child: Text('${i + 1}')),
            title: Text(d.steps[i].title),
            subtitle:
                d.steps[i].detail.isEmpty ? null : Text(d.steps[i].detail),
          ),
        const SizedBox(height: 8),
        if (d.warrantyAdvice.isNotEmpty)
          Card(
            child: ListTile(
              leading: Icon(widget.product.isCovered
                  ? Icons.verified_user
                  : Icons.info_outline),
              title: const Text('Garanti'),
              subtitle: Text(d.warrantyAdvice),
            ),
          ),
        const SizedBox(height: 16),
        FilledButton.icon(
          key: const Key('diagnosis_create_service'),
          onPressed: () => Navigator.of(context).pop(true),
          icon: const Icon(Icons.build),
          label: Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Text(d.recommendService
                ? 'Servis kaydı oluştur (önerilir)'
                : 'Sorun sürüyor, servis kaydı oluştur'),
          ),
        ),
        const SizedBox(height: 8),
        OutlinedButton(
          key: const Key('diagnosis_done'),
          onPressed: () => Navigator.of(context).pop(false),
          child: const Text('Sorun çözüldü'),
        ),
        const SizedBox(height: 16),
        Text(
          [
            d.isAi
                ? 'Kaynak: Gemini yapay zekâ${d.model.isEmpty ? '' : ' (${d.model})'}'
                : 'Kaynak: yerleşik öneriler',
            if (d.aiUnavailable)
              'Yapay zekâ şu an yanıt veremedi; genel öneriler gösteriliyor.',
            d.disclaimer,
          ].where((s) => s.isNotEmpty).join('\n'),
          style: theme.textTheme.bodySmall,
        ),
      ],
    );
  }
}
