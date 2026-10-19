import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../state/providers.dart';
import '../../widgets/common.dart';
import 'password_reset_screen.dart';

/// Login / register screen.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _name = TextEditingController();
  bool _registerMode = false;
  bool _obscure = true;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    _name.dispose();
    super.dispose();
  }

  static String? validateEmail(String? v) => validateEmailInput(v);

  static String? validatePassword(String? v) => PasswordField.validate(v);

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    final auth = ref.read(authControllerProvider.notifier);
    if (_registerMode) {
      await auth.register(_email.text, _password.text, _name.text);
    } else {
      await auth.login(_email.text, _password.text);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(authControllerProvider);
    final theme = Theme.of(context);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Form(
                key: _formKey,
                child: AutofillGroup(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Icon(Icons.shield_outlined,
                          size: 64, color: theme.colorScheme.primary),
                      const SizedBox(height: 12),
                      Text('Warranty Wallet',
                          textAlign: TextAlign.center,
                          style: theme.textTheme.headlineMedium),
                      const SizedBox(height: 4),
                      Text(
                        'Satın aldığınız ürünlerin garantisini takip edin, haklarınızı kaybetmeyin.',
                        textAlign: TextAlign.center,
                        style: theme.textTheme.bodyMedium,
                      ),
                      const SizedBox(height: 32),
                      if (_registerMode) ...[
                        TextFormField(
                          key: const Key('name'),
                          controller: _name,
                          decoration: const InputDecoration(
                              labelText: 'Ad Soyad',
                              border: OutlineInputBorder()),
                          textInputAction: TextInputAction.next,
                          autofillHints: const [AutofillHints.name],
                        ),
                        const SizedBox(height: 12),
                      ],
                      TextFormField(
                        key: const Key('email'),
                        controller: _email,
                        decoration: const InputDecoration(
                            labelText: 'E-posta', border: OutlineInputBorder()),
                        keyboardType: TextInputType.emailAddress,
                        textInputAction: TextInputAction.next,
                        autofillHints: const [AutofillHints.email],
                        validator: validateEmail,
                      ),
                      const SizedBox(height: 12),
                      TextFormField(
                        key: const Key('password'),
                        controller: _password,
                        obscureText: _obscure,
                        decoration: InputDecoration(
                          labelText: 'Şifre',
                          border: const OutlineInputBorder(),
                          suffixIcon: IconButton(
                            tooltip:
                                _obscure ? 'Şifreyi göster' : 'Şifreyi gizle',
                            icon: Icon(_obscure
                                ? Icons.visibility
                                : Icons.visibility_off),
                            onPressed: () =>
                                setState(() => _obscure = !_obscure),
                          ),
                        ),
                        autofillHints: [
                          _registerMode
                              ? AutofillHints.newPassword
                              : AutofillHints.password
                        ],
                        validator: validatePassword,
                        onFieldSubmitted: (_) => _submit(),
                      ),
                      if (!_registerMode)
                        Align(
                          alignment: Alignment.centerRight,
                          child: TextButton(
                            key: const Key('forgot_password'),
                            onPressed: () => Navigator.of(context).push(
                              MaterialPageRoute<void>(
                                builder: (_) => PasswordResetScreen(
                                    initialEmail: _email.text.trim()),
                              ),
                            ),
                            child: const Text('Şifremi unuttum'),
                          ),
                        ),
                      if (state.error != null) ...[
                        const SizedBox(height: 12),
                        ErrorText(state.error!, key: const Key('auth_error')),
                      ],
                      const SizedBox(height: 20),
                      FilledButton(
                        key: const Key('submit'),
                        onPressed: state.loading ? null : _submit,
                        child: Padding(
                          padding: const EdgeInsets.symmetric(vertical: 12),
                          child: state.loading
                              ? const SizedBox(
                                  height: 20,
                                  width: 20,
                                  child:
                                      CircularProgressIndicator(strokeWidth: 2))
                              : Text(_registerMode ? 'Kayıt ol' : 'Giriş yap'),
                        ),
                      ),
                      const SizedBox(height: 8),
                      TextButton(
                        key: const Key('toggle_mode'),
                        onPressed: () =>
                            setState(() => _registerMode = !_registerMode),
                        child: Text(_registerMode
                            ? 'Zaten hesabım var — giriş yap'
                            : 'Hesabınız yok mu? Kayıt olun'),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
