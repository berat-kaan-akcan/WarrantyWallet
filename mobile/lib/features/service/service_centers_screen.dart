import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Yetkili servis bulma: yöneticinin eklediği servis merkezleri + Google Haritalar araması.
class ServiceCentersScreen extends ConsumerStatefulWidget {
  const ServiceCentersScreen({super.key, required this.product});

  final Product product;

  @override
  ConsumerState<ServiceCentersScreen> createState() =>
      _ServiceCentersScreenState();
}

class _ServiceCentersScreenState extends ConsumerState<ServiceCentersScreen> {
  late final _city = TextEditingController(text: ref.read(serviceCityProvider));

  @override
  void dispose() {
    _city.dispose();
    super.dispose();
  }

  Future<void> _open(String url) async {
    final ok = await ref.read(linkOpenerProvider).open(Uri.parse(url));
    if (!ok && mounted) showSnack(context, 'Bağlantı açılamadı', error: true);
  }

  void _search() =>
      ref.read(serviceCityProvider.notifier).state = _city.text.trim();

  @override
  Widget build(BuildContext context) {
    final p = widget.product;
    final result = ref.watch(serviceCentersProvider(p.id));
    final label = p.brand.isEmpty ? p.name : p.brand;
    return Scaffold(
      appBar: AppBar(title: const Text('Yetkili servis bul')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text('$label yetkili servisleri',
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: TextField(
                  key: const Key('service_city'),
                  controller: _city,
                  textInputAction: TextInputAction.search,
                  onSubmitted: (_) => _search(),
                  decoration: const InputDecoration(
                      labelText: 'Şehir', hintText: 'Örn. İstanbul'),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(
                key: const Key('service_search'),
                tooltip: 'Ara',
                onPressed: _search,
                icon: const Icon(Icons.search),
              ),
            ],
          ),
          const SizedBox(height: 16),
          ...result.when(
            loading: () => const [LoadingView()],
            error: (e, _) => [
              InlineError(
                  error: e,
                  onRetry: () => ref.invalidate(serviceCentersProvider(p.id)))
            ],
            data: (r) => [
              if (r.items.isEmpty)
                Text(
                    _city.text.trim().isEmpty
                        ? 'Bu marka için kayıtlı servis merkezi yok. Haritada arayabilirsiniz.'
                        : '${_city.text.trim()} için kayıtlı servis merkezi yok. Haritada arayabilirsiniz.',
                    key: const Key('no_centers')),
              for (final c in r.items) _CenterCard(center: c, onOpen: _open),
              const SizedBox(height: 12),
              OutlinedButton.icon(
                key: const Key('maps_search'),
                onPressed: () => _open(r.mapsSearchUrl),
                icon: const Icon(Icons.map_outlined),
                label: const Text('Google Haritalar\'da ara'),
              ),
              const SizedBox(height: 12),
              Text(
                  'Servise giderken "Servis Kaydı Oluştur" ile fatura ve garanti belgenizi içeren servis paketini hazırlayın.',
                  style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ],
      ),
    );
  }
}

class _CenterCard extends StatelessWidget {
  const _CenterCard({required this.center, required this.onOpen});

  final ServiceCenter center;
  final Future<void> Function(String url) onOpen;

  @override
  Widget build(BuildContext context) {
    final c = center;
    final d = c.distanceKm;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(c.name, style: Theme.of(context).textTheme.titleSmall),
            Text(d == null
                ? c.place
                : '${c.place} • ${d.toStringAsFixed(1)} km'),
            if (c.address.isNotEmpty) Text(c.address),
            Wrap(
              spacing: 8,
              children: [
                if (c.phone.isNotEmpty)
                  TextButton.icon(
                    key: Key('call_${c.id}'),
                    onPressed: () => onOpen(
                        'tel:${c.phone.replaceAll(RegExp(r'[^0-9+]'), '')}'),
                    icon: const Icon(Icons.call),
                    label: Text(c.phone),
                  ),
                if (c.website.isNotEmpty)
                  TextButton.icon(
                    onPressed: () => onOpen(c.website),
                    icon: const Icon(Icons.open_in_new),
                    label: const Text('Web sitesi'),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
