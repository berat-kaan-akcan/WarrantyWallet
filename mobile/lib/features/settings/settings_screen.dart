import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import 'backup_screen.dart';
import 'change_password_screen.dart';
import 'family_screen.dart';

/// Hesap ve uygulama ayarları.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  Future<void> _deleteAccount(BuildContext context, WidgetRef ref) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Hesabı sil'),
        content: const Text(
            'Hesabınız, tüm ürünleriniz ve belgeleriniz kalıcı olarak silinecek. Bu işlem geri alınamaz.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Vazgeç')),
          FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Hesabı sil')),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).deleteAccount();
      await ref.read(authControllerProvider.notifier).logout();
      navigator.popUntil((r) => r.isFirst);
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  Future<void> _editName(BuildContext context, WidgetRef ref) async {
    final current = ref.read(authControllerProvider).user?.name ?? '';
    final controller = TextEditingController(text: current);
    final name = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Adınızı düzenleyin'),
        content: TextField(
          key: const Key('name_field'),
          controller: controller,
          autofocus: true,
          maxLength: 100,
          decoration: const InputDecoration(labelText: 'Ad Soyad'),
        ),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx), child: const Text('Vazgeç')),
          FilledButton(
              key: const Key('name_save'),
              onPressed: () => Navigator.pop(ctx, controller.text),
              child: const Text('Kaydet')),
        ],
      ),
    );
    controller.dispose();
    if (name == null || name.trim() == current || !context.mounted) return;
    try {
      final user = await ref.read(repositoryProvider).updateProfile(name: name);
      ref.read(authControllerProvider.notifier).userUpdated(user);
      ref.invalidate(dashboardProvider);
      if (context.mounted) showSnack(context, 'Profil güncellendi.');
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
            trailing: const Icon(Icons.edit),
            onTap: () => _editName(context, ref),
          ),
          ListTile(
            key: const Key('change_password'),
            leading: const Icon(Icons.lock_reset),
            title: const Text('Şifreyi değiştir'),
            onTap: () => Navigator.of(context).push(MaterialPageRoute<void>(
                builder: (_) => const ChangePasswordScreen())),
          ),
          ListTile(
            key: const Key('family'),
            leading: const Icon(Icons.family_restroom),
            title: const Text('Aile hesabı'),
            subtitle: const Text('Ailenizin garantilerini birlikte takip edin'),
            onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(builder: (_) => const FamilyScreen())),
          ),
          ListTile(
            key: const Key('backup'),
            leading: const Icon(Icons.backup_outlined),
            title: const Text('Yedekleme'),
            subtitle: const Text('Tüm kayıtları dışa aktar veya geri yükle'),
            onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(builder: (_) => const BackupScreen())),
          ),
          const ListTile(
            leading: Icon(Icons.notifications_active),
            title: Text('Garanti hatırlatıcıları'),
            subtitle: Text('Bitişe 90, 30 ve 7 gün kala saat 10:00'),
          ),
          if (kDebugMode) // developer detail, hidden in release builds
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
              final ok = await showDialog<bool>(
                context: context,
                builder: (ctx) => AlertDialog(
                  title: const Text('Çıkış yapılsın mı?'),
                  content: const Text(
                      'Bu cihazdaki garanti hatırlatıcıları da kapatılır. Tekrar giriş yaptığınızda geri gelir.'),
                  actions: [
                    TextButton(
                        onPressed: () => Navigator.pop(ctx, false),
                        child: const Text('Vazgeç')),
                    FilledButton(
                        key: const Key('confirm_logout'),
                        onPressed: () => Navigator.pop(ctx, true),
                        child: const Text('Çıkış yap')),
                  ],
                ),
              );
              if (ok != true) return;
              await ref.read(authControllerProvider.notifier).logout();
              navigator.popUntil((r) => r.isFirst);
            },
          ),
          ListTile(
            leading: Icon(Icons.delete_forever,
                color: Theme.of(context).colorScheme.error),
            title: Text('Hesabı sil',
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
            onTap: () => _deleteAccount(context, ref),
          ),
        ],
      ),
    );
  }
}
