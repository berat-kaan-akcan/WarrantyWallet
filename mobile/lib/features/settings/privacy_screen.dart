import 'package:flutter/material.dart';

/// The privacy notice (KVKK aydınlatma metni) shown before registration and in Settings.
/// It describes what the app actually stores and sends; the operator must complete the
/// controller details and have it reviewed before publishing.
class PrivacyScreen extends StatelessWidget {
  const PrivacyScreen({super.key});

  static const sections = <(String, String)>[
    (
      'Hangi verileri işliyoruz?',
      'Hesap bilgileriniz (e-posta, ad), eklediğiniz ürünler (ad, marka, model, seri numarası, '
          'barkod, satın alma tarihi, fiyat, satıcı, notlar), yüklediğiniz belgeler (fatura, garanti '
          'belgesi, ürün ve arıza fotoğrafları), servis kayıtlarınız ve bildirim için cihaz kimliğiniz.'
    ),
    (
      'Neden işliyoruz?',
      'Garanti sürelerinizi takip etmek, bitmeden önce hatırlatmak, belgelerinizi saklamak, servis '
          'sürecinizi kaydetmek ve isterseniz aile üyelerinizle ürün listesini paylaşmak için.'
    ),
    (
      'Yapay zekâ ve yurt dışına aktarım',
      'Fiş fotoğrafından bilgi okuma ve arıza asistanı özelliklerini kullandığınızda fiş görüntüsü, '
          'fişten okunan metin veya yazdığınız arıza açıklaması işlenmek üzere Google\'ın Gemini '
          'hizmetine gönderilir. Bu hizmet yurt dışında çalışır. Bu özellikleri kullanmak zorunda '
          'değilsiniz; bilgileri elle de girebilirsiniz.'
    ),
    (
      'Kimlerle paylaşılır?',
      'Verileriniz satılmaz. Aile hesabına katılırsanız ürünleriniz ve garanti durumları aile '
          'üyelerine salt okunur gösterilir; belgeler ve servis kayıtları paylaşılmaz.'
    ),
    (
      'Haklarınız',
      'KVKK madde 11 kapsamındaki haklarınızı kullanabilirsiniz. Verilerinizin tamamını Ayarlar → '
          'Yedekleme bölümünden indirebilir, Ayarlar → Hesabı sil ile hesabınızı ve tüm verilerinizi '
          'kalıcı olarak silebilirsiniz.'
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Aydınlatma metni')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            for (final (title, body) in sections) ...[
              Semantics(
                  header: true,
                  child: Text(title, style: theme.textTheme.titleMedium)),
              const SizedBox(height: 6),
              Text(body),
              const SizedBox(height: 16),
            ],
          ],
        ),
      ),
    );
  }
}
