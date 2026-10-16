import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Hesap ve uygulama ayarları.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  Future<void> _deleteAccount(BuildContext context, WidgetRef ref) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Hesabı sil'),
        content: const Text('Hesabınız, tüm ürünleriniz ve belgeleriniz kalıcı olarak silinecek. Bu işlem geri alınamaz.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Vazgeç')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Hesabı sil')),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).deleteAccount();
      await ref.read(reminderSchedulerProvider).syncAll(const []);
      await ref.read(authControllerProvider.notifier).logout();
      navigator.popUntil((r) => r.isFirst);
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final user = ref.watch(authControllerProvider).user;
    return Scaffold(
      appBar: AppBar(title: const Text('Ayarlar')),
      body: ListView(
        children: [
          ListTile(
            leading: const Icon(Icons.person),
            title: Text(user?.name.isNotEmpty == true ? user!.name : 'Hesabım'),
            subtitle: Text(user?.email ?? ''),
          ),
          const ListTile(
            leading: Icon(Icons.notifications_active),
            title: Text('Garanti hatırlatıcıları'),
            subtitle: Text('Bitişe 90, 30 ve 7 gün kala saat 10:00'),
          ),
          const ListTile(
            leading: Icon(Icons.dns_outlined),
            title: Text('Sunucu'),
            subtitle: Text(apiBaseUrl),
          ),
          const Divider(),
          ListTile(
            key: const Key('logout'),
            leading: const Icon(Icons.logout),
            title: const Text('Çıkış yap'),
            onTap: () async {
              final navigator = Navigator.of(context);
              await ref.read(authControllerProvider.notifier).logout();
              navigator.popUntil((r) => r.isFirst);
            },
          ),
          ListTile(
            leading: Icon(Icons.delete_forever, color: Theme.of(context).colorScheme.error),
            title: Text('Hesabı sil', style: TextStyle(color: Theme.of(context).colorScheme.error)),
            onTap: () => _deleteAccount(context, ref),
          ),
        ],
      ),
    );
  }
}
