import 'package:flutter/material.dart';

import '../core/api_client.dart';
import '../core/formatters.dart';
import '../core/warranty.dart';

/// Brand colours.
const brandColor = Color(0xFF1B5E9E);

/// Colour + label + icon for a warranty status (never colour alone, for accessibility).
({Color color, String label, IconData icon}) statusStyle(WarrantyStatus s) {
  switch (s) {
    case WarrantyStatus.active:
      return (
        color: const Color(0xFF2E7D32),
        label: 'Aktif',
        icon: Icons.verified_user
      );
    case WarrantyStatus.expiringSoon:
      return (
        color: const Color(0xFFB26A00),
        label: 'Yakında bitiyor',
        icon: Icons.schedule
      );
    case WarrantyStatus.expired:
      return (
        color: const Color(0xFFC62828),
        label: 'Bitti',
        icon: Icons.gpp_bad
      );
    case WarrantyStatus.unknown:
      return (
        color: Colors.grey,
        label: 'Bilinmiyor',
        icon: Icons.help_outline
      );
  }
}

/// Pill showing the warranty status.
class StatusChip extends StatelessWidget {
  const StatusChip(this.status, {super.key});

  final WarrantyStatus status;

  @override
  Widget build(BuildContext context) {
    final st = statusStyle(status);
    return Semantics(
      label: 'Garanti durumu: ${st.label}',
      excludeSemantics: true,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          color: st.color.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(st.icon, size: 16, color: st.color),
            const SizedBox(width: 4),
            Text(st.label,
                style: TextStyle(color: st.color, fontWeight: FontWeight.w600)),
          ],
        ),
      ),
    );
  }
}

/// Days-left badge used in lists.
class DaysLeftText extends StatelessWidget {
  const DaysLeftText(this.days, this.status, {super.key});

  final int? days;
  final WarrantyStatus status;

  @override
  Widget build(BuildContext context) {
    if (days == null) return const Text('-');
    return Text(
      formatDaysLeft(days!),
      style: TextStyle(
          color: statusStyle(status).color, fontWeight: FontWeight.w600),
    );
  }
}

/// Category icon.
IconData categoryIcon(String slug) {
  switch (slug) {
    case 'phone':
      return Icons.smartphone;
    case 'laptop':
      return Icons.laptop_mac;
    case 'monitor':
      return Icons.desktop_windows;
    case 'tv':
      return Icons.tv;
    case 'headphones':
      return Icons.headphones;
    case 'tablet':
      return Icons.tablet_mac;
    case 'appliance':
      return Icons.kitchen;
    case 'small_appliance':
      return Icons.coffee;
    case 'camera':
      return Icons.photo_camera;
    case 'console':
      return Icons.sports_esports;
    case 'wearable':
      return Icons.watch;
    default:
      return Icons.inventory_2;
  }
}

/// Empty state with an optional action. Scrolls when the text is enlarged.
class EmptyState extends StatelessWidget {
  const EmptyState(
      {super.key,
      required this.icon,
      required this.title,
      this.message,
      this.action});

  final IconData icon;
  final String title;
  final String? message;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final content = Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ExcludeSemantics(
                child: Icon(icon, size: 56, color: theme.colorScheme.outline)),
            const SizedBox(height: 12),
            MergeSemantics(
              child: Column(
                children: [
                  Semantics(
                    header: true,
                    child: Text(title,
                        style: theme.textTheme.titleMedium,
                        textAlign: TextAlign.center),
                  ),
                  if (message != null) ...[
                    const SizedBox(height: 6),
                    Text(message!,
                        style: theme.textTheme.bodyMedium,
                        textAlign: TextAlign.center),
                  ],
                ],
              ),
            ),
            if (action != null) ...[const SizedBox(height: 16), action!],
          ],
        ));
    // Inside another scroll view (unbounded height) the content is laid out as is.
    return LayoutBuilder(
      builder: (context, c) => c.maxHeight.isFinite
          ? Center(child: SingleChildScrollView(child: content))
          : content,
    );
  }
}

/// Full-screen error state with retry; offline errors get their own wording.
class ErrorState extends StatelessWidget {
  const ErrorState({super.key, required this.error, required this.onRetry});

  final Object error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final offline = error is ApiException && (error as ApiException).isNetwork;
    return EmptyState(
      icon: offline ? Icons.wifi_off : Icons.cloud_off,
      title: offline ? 'Bağlantı yok' : 'Bir sorun oluştu',
      message: errorMessage(error),
      action: OutlinedButton.icon(
          key: const Key('retry'),
          onPressed: onRetry,
          icon: const Icon(Icons.refresh),
          label: const Text('Tekrar dene')),
    );
  }
}

/// Compact error inside a section, with a retry button.
class InlineError extends StatelessWidget {
  const InlineError({super.key, required this.error, required this.onRetry});

  final Object error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          children: [
            Expanded(child: ErrorText(errorMessage(error))),
            TextButton(onPressed: onRetry, child: const Text('Tekrar dene')),
          ],
        ),
      );
}

/// Centered progress indicator announced as "Yükleniyor".
class LoadingView extends StatelessWidget {
  const LoadingView({super.key});

  @override
  Widget build(BuildContext context) => const Center(
      child: CircularProgressIndicator(semanticsLabel: 'Yükleniyor'));
}

/// Shows a snackbar message.
void showSnack(BuildContext context, String message, {bool error = false}) {
  // Replace the current message instead of queueing behind it.
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(
      content: Text(message),
      backgroundColor: error ? Theme.of(context).colorScheme.error : null,
    ));
}

/// User-facing message for any error.
String errorMessage(Object e) =>
    e is ApiException ? e.userMessage : 'Beklenmeyen bir hata oluştu.';

/// Labelled value row on the detail screen.
class InfoRow extends StatelessWidget {
  const InfoRow(this.label, this.value, {super.key, this.icon});

  final String label;
  final String value;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return MergeSemantics(
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 20, color: theme.colorScheme.primary),
              const SizedBox(width: 12)
            ],
            Expanded(
                flex: 2, child: Text(label, style: theme.textTheme.bodyMedium)),
            Expanded(
              flex: 3,
              child: Text(value,
                  style: theme.textTheme.bodyLarge
                      ?.copyWith(fontWeight: FontWeight.w600)),
            ),
          ],
        ),
      ),
    );
  }
}

/// Password input with a show/hide toggle and the shared length rule.
class PasswordField extends StatefulWidget {
  const PasswordField({
    super.key,
    required this.controller,
    this.label = 'Şifre',
    this.newPassword = false,
    this.errorText,
    this.onSubmitted,
  });

  final TextEditingController controller;
  final String label;
  final bool newPassword;
  final String? errorText;
  final ValueChanged<String>? onSubmitted;

  /// Same rule as the backend (8–128 characters).
  static String? validate(String? v) {
    final s = v ?? '';
    if (s.length < 8) return 'Şifre en az 8 karakter olmalı';
    if (s.length > 128) return 'Şifre en fazla 128 karakter olabilir';
    if (s.trim().isEmpty) return 'Şifre yalnızca boşluk olamaz';
    return null;
  }

  @override
  State<PasswordField> createState() => _PasswordFieldState();
}

class _PasswordFieldState extends State<PasswordField> {
  bool _obscure = true;

  @override
  Widget build(BuildContext context) => TextFormField(
        controller: widget.controller,
        obscureText: _obscure,
        decoration: InputDecoration(
          labelText: widget.label,
          border: const OutlineInputBorder(),
          errorText: widget.errorText,
          helperText: widget.newPassword ? 'En az 8 karakter' : null,
          suffixIcon: IconButton(
            tooltip: _obscure ? 'Şifreyi göster' : 'Şifreyi gizle',
            icon: Icon(_obscure ? Icons.visibility : Icons.visibility_off),
            onPressed: () => setState(() => _obscure = !_obscure),
          ),
        ),
        autofillHints: [
          widget.newPassword
              ? AutofillHints.newPassword
              : AutofillHints.password
        ],
        validator: PasswordField.validate,
        onFieldSubmitted: widget.onSubmitted,
      );
}

/// Inline error message announced by screen readers.
class ErrorText extends StatelessWidget {
  const ErrorText(this.message, {super.key});

  final String message;

  @override
  Widget build(BuildContext context) => Semantics(
        liveRegion: true,
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.error_outline,
                size: 20, color: Theme.of(context).colorScheme.error),
            const SizedBox(width: 8),
            Expanded(
              child: Text(message,
                  style: TextStyle(color: Theme.of(context).colorScheme.error)),
            ),
          ],
        ),
      );
}

/// E-mail form validator shared by the auth screens.
String? validateEmailInput(String? v) {
  final s = (v ?? '').trim();
  if (s.isEmpty) return 'E-posta gerekli';
  if (s.length > 254 || !RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$').hasMatch(s)) {
    return 'Geçerli bir e-posta girin';
  }
  return null;
}
