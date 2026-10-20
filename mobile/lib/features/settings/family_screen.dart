import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../core/formatters.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Aile hesabı: üyeler birbirinin ürünlerini ve garanti durumlarını görür (salt okunur).
class FamilyScreen extends ConsumerStatefulWidget {
  const FamilyScreen({super.key});

  @override
  ConsumerState<FamilyScreen> createState() => _FamilyScreenState();
}

class _FamilyScreenState extends ConsumerState<FamilyScreen> {
  final _name = TextEditingController();
  final _code = TextEditingController();
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() => _busy = true);
    try {
      await action();
      ref
        ..invalidate(familyProvider)
        ..invalidate(familyProductsProvider);
    } on ApiException catch (e) {
      if (mounted) showSnack(context, e.userMessage, error: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _create() async {
    if (_name.text.trim().isEmpty) {
      showSnack(context, 'Aile adı girin', error: true);
      return;
    }
    await _run(
        () => ref.read(repositoryProvider).createFamily(_name.text.trim()));
  }

  Future<void> _join() async {
    if (_code.text.trim().isEmpty) {
      showSnack(context, 'Davet kodunu girin', error: true);
      return;
    }
    await _run(() async {
      await ref.read(repositoryProvider).joinFamily(_code.text);
      if (mounted) showSnack(context, 'Aileye katıldınız');
    });
  }

  Future<void> _invite() => _run(() async {
        final code = await ref.read(repositoryProvider).createFamilyInvite();
        if (!mounted) return;
        await showDialog<void>(
          context: context,
          builder: (ctx) => AlertDialog(
            title: const Text('Davet kodu'),
            content: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SelectableText(code,
                    key: const Key('invite_code'),
                    style: Theme.of(ctx).textTheme.headlineMedium),
                const SizedBox(height: 8),
                const Text(
                    'Kodu aile üyenize iletin. Uygulamada Ayarlar → Aile hesabı → "Davet koduyla katıl" bölümüne girsin. '
                    'Kod 7 gün geçerli ve tek kullanımlık.'),
              ],
            ),
            actions: [
              TextButton(
                key: const Key('share_invite'),
                onPressed: () async {
                  try {
                    await ref.read(textSharerProvider)(
                        'Warranty Wallet aile hesabıma katıl: Ayarlar → Aile hesabı → '
                        '"Davet koduyla katıl" bölümüne $code kodunu gir. Kod 7 gün geçerli.');
                  } on Exception {
                    if (ctx.mounted) {
                      showSnack(ctx, 'Paylaşılamadı', error: true);
                    }
                  }
                },
                child: const Text('Paylaş'),
              ),
              TextButton(
                onPressed: () async {
                  await Clipboard.setData(ClipboardData(text: code));
                  if (ctx.mounted) Navigator.of(ctx).pop();
                },
                child: const Text('Kopyala'),
              ),
              TextButton(
                  onPressed: () => Navigator.of(ctx).pop(),
                  child: const Text('Kapat')),
            ],
          ),
        );
      });

  Future<void> _remove(FamilyMember m,
      {required bool self, required bool owner}) async {
    final title = self
        ? (owner ? 'Aile dağıtılsın mı?' : 'Aileden ayrılınsın mı?')
        : 'Üye çıkarılsın mı?';
    final action = self ? (owner ? 'Aileyi dağıt' : 'Ayrıl') : 'Çıkar';
    final text = self
        ? (owner
            ? 'Aile dağıtılacak ve tüm üyeler çıkarılacak. Ürünleriniz silinmez.'
            : 'Aileden ayrılacaksınız. Ürünleriniz silinmez.')
        : '${m.displayName} aileden çıkarılsın mı?';
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: Text(text),
        actions: [
          TextButton(
              onPressed: () => Navigator.of(ctx).pop(false),
              child: const Text('Vazgeç')),
          TextButton(
              key: const Key('confirm'),
              onPressed: () => Navigator.of(ctx).pop(true),
              child: Text(action)),
        ],
      ),
    );
    if (ok != true) return;
    await _run(() => ref.read(repositoryProvider).removeFamilyMember(m.id));
  }

  @override
  Widget build(BuildContext context) {
    final family = ref.watch(familyProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Aile hesabı')),
      body: family.when(
        loading: () => const LoadingView(),
        error: (e, _) =>
            ErrorState(error: e, onRetry: () => ref.invalidate(familyProvider)),
        data: (f) => f.inFamily ? _familyView(f) : _noFamily(),
      ),
    );
  }

  Widget _noFamily() => ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const Text(
              'Aile hesabıyla eşinizin, çocuklarınızın veya ev arkadaşlarınızın ürünlerini ve garanti bitişlerini tek yerden görebilirsiniz. '
              'Herkes yalnızca kendi ürünlerini düzenleyebilir; belgeler paylaşılmaz.'),
          const SizedBox(height: 24),
          Text('Yeni aile oluştur',
              style: Theme.of(context).textTheme.titleMedium),
          TextField(
            key: const Key('family_name'),
            controller: _name,
            maxLength: 60,
            decoration: const InputDecoration(labelText: 'Aile adı'),
          ),
          FilledButton(
              key: const Key('family_create'),
              onPressed: _busy ? null : _create,
              child: const Text('Oluştur')),
          const Divider(height: 48),
          Text('Davet koduyla katıl',
              style: Theme.of(context).textTheme.titleMedium),
          TextField(
            key: const Key('family_code'),
            controller: _code,
            textCapitalization: TextCapitalization.characters,
            decoration: const InputDecoration(
                labelText: 'Davet kodu', hintText: 'Örn. K7QM-2XRP'),
          ),
          const SizedBox(height: 8),
          OutlinedButton(
              key: const Key('family_join'),
              onPressed: _busy ? null : _join,
              child: const Text('Katıl')),
        ],
      );

  Widget _familyView(FamilyInfo f) {
    final me = ref.watch(authControllerProvider).user;
    final products = ref.watch(familyProductsProvider);
    final theme = Theme.of(context);
    return RefreshIndicator(
      onRefresh: () async {
        ref
          ..invalidate(familyProvider)
          ..invalidate(familyProductsProvider);
        await ref.read(familyProductsProvider.future);
      },
      child: ListView(
        padding: const EdgeInsets.symmetric(vertical: 8),
        children: [
          ListTile(
            title: Text(f.name ?? '', style: theme.textTheme.titleLarge),
            subtitle: Text('${f.members.length} üye'),
          ),
          for (final m in f.members)
            ListTile(
              leading: Icon(m.isOwner ? Icons.star : Icons.person_outline),
              title: Text(
                  m.id == me?.id ? '${m.displayName} (siz)' : m.displayName),
              subtitle:
                  Text(m.isOwner ? 'Aile yöneticisi · ${m.email}' : m.email),
              trailing: f.isOwner && m.id != me?.id
                  ? IconButton(
                      key: Key('remove_${m.id}'),
                      tooltip: 'Aileden çıkar',
                      icon: const Icon(Icons.person_remove_outlined),
                      onPressed: _busy
                          ? null
                          : () => _remove(m, self: false, owner: true),
                    )
                  : null,
            ),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Wrap(
              spacing: 8,
              children: [
                if (f.isOwner)
                  FilledButton.icon(
                    key: const Key('family_invite'),
                    onPressed: _busy || f.members.length >= 8 ? null : _invite,
                    icon: const Icon(Icons.person_add_alt),
                    label: const Text('Üye davet et'),
                  ),
                OutlinedButton(
                  key: const Key('family_leave'),
                  onPressed: _busy || me == null
                      ? null
                      : () => _remove(
                          f.members.firstWhere((m) => m.id == me.id,
                              orElse: () => FamilyMember(
                                  id: me.id,
                                  name: me.name,
                                  email: me.email,
                                  isOwner: f.isOwner)),
                          self: true,
                          owner: f.isOwner),
                  child: Text(f.isOwner ? 'Aileyi dağıt' : 'Aileden ayrıl'),
                ),
              ],
            ),
          ),
          const Divider(height: 32),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Text('Ailenin ürünleri', style: theme.textTheme.titleMedium),
          ),
          ...products.when(
            loading: () => [
              const Padding(padding: EdgeInsets.all(24), child: LoadingView())
            ],
            error: (e, _) => [
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: InlineError(
                    error: e,
                    onRetry: () => ref.invalidate(familyProductsProvider)),
              )
            ],
            data: (items) => items.isEmpty
                ? [
                    const Padding(
                        padding: EdgeInsets.all(16),
                        child: Text('Henüz ürün eklenmemiş.'))
                  ]
                : [
                    for (final fp in items)
                      ListTile(
                        leading: CircleAvatar(
                            child: Icon(categoryIcon(fp.product.category))),
                        title: Text(fp.product.name, maxLines: 2),
                        subtitle: Text(
                            '${fp.ownerName} • Bitiş: ${formatDate(fp.product.warrantyEnd)}'),
                        trailing: StatusChip(fp.product.status),
                      ),
                  ],
          ),
        ],
      ),
    );
  }
}
