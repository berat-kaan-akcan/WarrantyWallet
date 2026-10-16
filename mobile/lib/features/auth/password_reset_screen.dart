import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api_client.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Two-step password reset: request a code by e-mail, then set a new password with it.
class PasswordResetScreen extends ConsumerStatefulWidget {
  const PasswordResetScreen({super.key, this.initialEmail = ''});

  final String initialEmail;

  @override
  ConsumerState<PasswordResetScreen> createState() =>
      _PasswordResetScreenState();
}

class _PasswordResetScreenState extends ConsumerState<PasswordResetScreen> {
  final _requestKey = GlobalKey<FormState>();
  final _resetKey = GlobalKey<FormState>();
  late final _email = TextEditingController(text: widget.initialEmail);
  final _code = TextEditingController();
  final _password = TextEditingController();
  bool _codeSent = false;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _email.dispose();
    _code.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _request() async {
    if (!(_requestKey.currentState?.validate() ?? false)) return;
    await _run(() async {
      await ref.read(repositoryProvider).forgotPassword(_email.text);
      if (mounted) setState(() => _codeSent = true);
    });
  }

  Future<void> _reset() async {
    if (!(_resetKey.currentState?.validate() ?? false)) return;
    final navigator = Navigator.of(context);
    final messenger = ScaffoldMessenger.of(context);
    await _run(() async {
      await ref
          .read(repositoryProvider)
          .resetPassword(_code.text, _password.text);
      messenger.showSnackBar(const SnackBar(
          content: Text('Şifreniz güncellendi. Yeni şifrenizle giriş yapın.')));
      navigator.pop();
    });
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await action();
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.userMessage);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Şifremi unuttum')),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Form(
                    key: _requestKey,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(
                          'E-posta adresinizi girin. Hesabınız varsa size bir sıfırlama kodu göndereceğiz.',
                          style: theme.textTheme.bodyMedium,
                        ),
                        const SizedBox(height: 16),
                        TextFormField(
                          key: const Key('reset_email'),
                          controller: _email,
                          enabled: !_codeSent,
                          decoration: const InputDecoration(
                              labelText: 'E-posta',
                              border: OutlineInputBorder()),
                          keyboardType: TextInputType.emailAddress,
                          autofillHints: const [AutofillHints.email],
                          validator: validateEmailInput,
                        ),
                        const SizedBox(height: 12),
                        if (!_codeSent)
                          FilledButton(
                            key: const Key('send_code'),
                            onPressed: _busy ? null : _request,
                            child: const Text('Kod gönder'),
                          ),
                      ],
                    ),
                  ),
                  if (_codeSent)
                    Form(
                      key: _resetKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Semantics(
                            liveRegion: true,
                            child: Text(
                              'Hesabınız varsa e-postanıza bir kod gönderildi. Kodu ve yeni şifrenizi girin.',
                              key: const Key('code_sent'),
                              style: theme.textTheme.bodyMedium,
                            ),
                          ),
                          const SizedBox(height: 12),
                          TextFormField(
                            key: const Key('reset_code'),
                            controller: _code,
                            decoration: const InputDecoration(
                                labelText: 'Sıfırlama kodu',
                                border: OutlineInputBorder()),
                            autofillHints: const [AutofillHints.oneTimeCode],
                            validator: (v) =>
                                (v ?? '').trim().isEmpty ? 'Kod gerekli' : null,
                          ),
                          const SizedBox(height: 12),
                          PasswordField(
                            key: const Key('reset_password'),
                            controller: _password,
                            label: 'Yeni şifre',
                            newPassword: true,
                          ),
                          const SizedBox(height: 12),
                          FilledButton(
                            key: const Key('reset_submit'),
                            onPressed: _busy ? null : _reset,
                            child: const Text('Şifreyi güncelle'),
                          ),
                        ],
                      ),
                    ),
                  if (_error != null) ...[
                    const SizedBox(height: 12),
                    ErrorText(_error!, key: const Key('reset_error')),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
