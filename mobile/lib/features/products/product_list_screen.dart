import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import 'product_detail_screen.dart';

/// Ürün listesi: arama, kategori ve durum filtresi.
class ProductListScreen extends ConsumerStatefulWidget {
  const ProductListScreen({super.key});

  @override
  ConsumerState<ProductListScreen> createState() => _ProductListScreenState();
}

class _ProductListScreenState extends ConsumerState<ProductListScreen> {
  ProductQuery _query = const ProductQuery();
  final _search = TextEditingController();

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  static const _statuses = {
    '': 'Tümü',
    'active': 'Aktif',
    'expiring_soon': 'Yakında bitiyor',
    'expired': 'Bitti'
  };

  @override
  Widget build(BuildContext context) {
    final products = ref.watch(productsProvider(_query));
    final cats = ref.watch(categoriesProvider).valueOrNull ?? defaultCategories;
    return Scaffold(
      appBar: AppBar(title: const Text('Ürünlerim')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
            child: TextField(
              controller: _search,
              decoration: InputDecoration(
                hintText: 'Ürün, marka veya satıcı ara',
                prefixIcon: const Icon(Icons.search),
                border: const OutlineInputBorder(),
                isDense: true,
                suffixIcon: _search.text.isEmpty
                    ? null
                    : IconButton(
                        tooltip: 'Aramayı temizle',
                        icon: const Icon(Icons.clear),
                        onPressed: () {
                          _search.clear();
                          setState(() => _query = ProductQuery(
                              category: _query.category,
                              status: _query.status));
                        },
                      ),
              ),
              textInputAction: TextInputAction.search,
              onSubmitted: (v) => setState(() => _query = ProductQuery(
                  category: _query.category,
                  status: _query.status,
                  text: v.trim())),
            ),
          ),
          SizedBox(
            height: 52,
            child: ListView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              children: [
                for (final e in _statuses.entries)
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: ChoiceChip(
                      label: Text(e.value),
                      selected: _query.status == e.key,
                      onSelected: (_) => setState(() => _query = ProductQuery(
                          category: _query.category,
                          status: e.key,
                          text: _query.text)),
                    ),
                  ),
                const VerticalDivider(),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  child: DropdownButton<String>(
                    value: _query.category,
                    underline: const SizedBox.shrink(),
                    items: [
                      const DropdownMenuItem(
                          value: '', child: Text('Tüm kategoriler')),
                      for (final c in cats)
                        DropdownMenuItem(value: c.slug, child: Text(c.name)),
                    ],
                    onChanged: (v) => setState(() => _query = ProductQuery(
                        category: v ?? '',
                        status: _query.status,
                        text: _query.text)),
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            child: products.when(
              data: (items) => RefreshIndicator(
                onRefresh: () async {
                  refreshProducts(ref);
                  await ref.read(productsProvider(_query).future);
                },
                child: items.isEmpty
                    ? ListView(
                        children: [
                          EmptyState(
                            icon: Icons.inventory_2_outlined,
                            title: _query == const ProductQuery()
                                ? 'Henüz ürün yok'
                                : 'Sonuç bulunamadı',
                            message: _query == const ProductQuery()
                                ? '"Ürün Ekle" ile fişinin fotoğrafını çek.'
                                : 'Filtreleri değiştirmeyi deneyin.',
                          ),
                        ],
                      )
                    : ListView.separated(
                        padding: const EdgeInsets.only(bottom: 96),
                        itemCount: items.length,
                        separatorBuilder: (_, __) => const Divider(height: 1),
                        itemBuilder: (_, i) =>
                            ProductTile(product: items[i], categories: cats),
                      ),
              ),
              loading: () => const LoadingView(),
              error: (e, _) => ErrorState(
                  error: e,
                  onRetry: () => ref.invalidate(productsProvider(_query))),
            ),
          ),
        ],
      ),
    );
  }
}

/// One product row.
class ProductTile extends StatelessWidget {
  const ProductTile(
      {super.key, required this.product, this.categories = defaultCategories});

  final Product product;
  final List<ProductCategory> categories;

  @override
  Widget build(BuildContext context) {
    final p = product;
    return ListTile(
      leading: CircleAvatar(child: Icon(categoryIcon(p.category))),
      title: Text(p.name, maxLines: 2),
      subtitle: Text(
          '${categoryName(p.category, categories)} • Bitiş: ${formatDate(p.warrantyEnd)}'),
      trailing: StatusChip(p.status),
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute<void>(
            builder: (_) => ProductDetailScreen(productId: p.id)),
      ),
    );
  }
}
