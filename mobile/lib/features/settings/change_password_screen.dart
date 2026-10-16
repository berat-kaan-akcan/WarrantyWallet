import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Changes the password of the signed-in user. Other devices are signed out.
class ChangePasswordScreen extends ConsumerStatefulWidget {
  const ChangePasswordScreen({super.key});

  @override
  ConsumerState<ChangePasswordScreen> createState() =>
      _ChangePasswordScreenState();
}

class _ChangePasswordScreenState extends ConsumerState<ChangePasswordScreen> {
  final _formKey = GlobalKey<FormState>();
  final _current = TextEditingController();
  final _next = TextEditingController();
  bool _busy = false;
  Map<String, String> _fieldErrors = const {};
  String? _error;

  @override
  void dispose() {
    _current.dispose();
    _next.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    final navigator = Navigator.of(context);
    final messenger = ScaffoldMessenger.of(context);
    setState(() {
      _busy = true;
      _error = null;
      _fieldErrors = const {};
    });
    try {
      await ref
          .read(repositoryProvider)
          .changePassword(_current.text, _next.text);
      messenger.showSnackBar(
          const SnackBar(content: Text('Şifreniz değiştirildi.')));
      navigator.pop();
    } on ApiException catch (e) {
      if (mounted) {
        setState(() {
          _fieldErrors = e.fields;
          _error = e.fields.isEmpty ? e.userMessage : null;
        });
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String? _translate(String? apiMessage) => switch (apiMessage) {
        null => null,
        'incorrect password' => 'Mevcut şifre hatalı',
        'must differ from the current password' =>
          'Yeni şifre mevcut şifreden farklı olmalı',
        _ => 'Geçersiz değer',
      };

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Şifreyi değiştir')),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PasswordField(
                  key: const Key('current_password'),
                  controller: _current,
                  label: 'Mevcut şifre',
                  errorText: _translate(_fieldErrors['current_password']),
                ),
                const SizedBox(height: 12),
                PasswordField(
                  key: const Key('new_password'),
                  controller: _next,
                  label: 'Yeni şifre',
                  newPassword: true,
                  errorText: _translate(_fieldErrors['new_password']),
                ),
                const SizedBox(height: 8),
                const Text(
                    'Şifre değişince diğer cihazlardaki oturumlar kapatılır.'),
                const SizedBox(height: 16),
                FilledButton(
                  key: const Key('change_submit'),
                  onPressed: _busy ? null : _submit,
                  child: const Text('Kaydet'),
                ),
                if (_error != null) ...[
                  const SizedBox(height: 12),
                  ErrorText(_error!),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}
