import 'dart:async';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';

import '../../core/scan_parser.dart';
import '../scan/code_scanner.dart';
import '../../core/api_client.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';
import 'product_detail_screen.dart';
import 'product_form.dart';

/// Picks an image from camera or gallery. Abstracted for widget tests.
abstract class ReceiptImagePicker {
  Future<PickedImage?> pick({required bool camera});
}

/// A picked image.
class PickedImage {
  const PickedImage(this.path, this.name, this.bytes);

  final String path;
  final String name;
  final Uint8List bytes;
}

/// image_picker implementation.
class DeviceImageSource implements ReceiptImagePicker {
  final _picker = ImagePicker();

  @override
  Future<PickedImage?> pick({required bool camera}) async {
    final x = await _picker.pickImage(
      source: camera ? ImageSource.camera : ImageSource.gallery,
      maxWidth: 2000,
      imageQuality: 85,
    );
    if (x == null) return null;
    return PickedImage(x.path, x.name, await x.readAsBytes());
  }
}

final imageSourceProvider =
    Provider<ReceiptImagePicker>((ref) => DeviceImageSource());

enum _Step { choose, reading, review }

/// "Ürün Ekle": fiş fotoğrafı → OCR → AI → kontrol → Kaydet.
class AddProductScreen extends ConsumerStatefulWidget {
  const AddProductScreen({super.key, this.scanned});

  /// Code scanned before adding (e.g. "not found" after a barcode lookup).
  final ScannedCode? scanned;

  @override
  ConsumerState<AddProductScreen> createState() => _AddProductScreenState();
}

class _AddProductScreenState extends ConsumerState<AddProductScreen> {
  _Step _step = _Step.choose;
  PickedImage? _image;
  ExtractionResult? _result;
  String? _error;

  Future<void> _capture({required bool camera}) async {
    final picked = await ref.read(imageSourceProvider).pick(camera: camera);
    if (picked == null || !mounted) return;
    setState(() {
      _image = picked;
      _step = _Step.reading;
      _error = null;
    });
    final ocr = ref.read(ocrServiceProvider);
    final repo = ref.read(repositoryProvider);
    try {
      final text = await ocr.recognize(picked.path);
      final result = await repo.extract(
          image: picked.bytes, imageName: picked.name, text: text);
      if (!mounted) return;
      setState(() {
        _result = result;
        _step = _Step.review;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      // Extraction failed: continue with an empty form, the photo is still saved as invoice.
      setState(() {
        _error = '${e.userMessage} Bilgileri elle girebilirsiniz.';
        _result = null;
        _step = _Step.review;
      });
    }
  }

  ProductDraft _initialDraft() {
    final d = _result?.draft ?? ProductDraft();
    final code = widget.scanned;
    if (code != null) {
      if (d.serialNumber.isEmpty) d.serialNumber = code.serial;
      if (d.barcode.isEmpty) d.barcode = code.barcode;
    }
    return d;
  }

  void _manual() => setState(() {
        _image = null;
        _result = null;
        _step = _Step.review;
      });

  Future<void> _save(ProductDraft draft) async {
    final repo = ref.read(repositoryProvider);
    final scheduler = ref.read(reminderSchedulerProvider);
    final navigator = Navigator.of(context);
    try {
      final product = await repo.createProduct(draft);
      final image = _image;
      if (image != null) {
        try {
          await repo.uploadDocument(
              product.id, 'invoice', image.bytes, image.name);
        } on ApiException catch (e) {
          if (mounted) {
            showSnack(context,
                'Ürün kaydedildi ama fatura yüklenemedi: ${e.userMessage}',
                error: true);
          }
        }
      }
      await scheduler.syncProduct(product);
      if (!mounted) return;
      refreshProducts(ref);
      unawaited(navigator.pushReplacement(
        MaterialPageRoute<void>(
            builder: (_) => ProductDetailScreen(productId: product.id)),
      ));
    } on ApiException catch (e) {
      if (!mounted) return;
      final fields = e.fieldList.isEmpty ? '' : ' (${e.fieldList})';
      showSnack(context, '${e.userMessage}$fields', error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final cats = ref.watch(categoriesProvider).valueOrNull ?? defaultCategories;
    return Scaffold(
      appBar: AppBar(title: const Text('Ürün Ekle')),
      body: switch (_step) {
        _Step.choose => _ChooseSource(
            onCamera: () => _capture(camera: true),
            onGallery: () => _capture(camera: false),
            onManual: _manual,
          ),
        _Step.reading => const _Reading(),
        _Step.review => ListView(
            padding: const EdgeInsets.all(16),
            children: [
              if (_image != null)
                ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: Image.memory(_image!.bytes,
                      height: 160,
                      fit: BoxFit.cover,
                      semanticLabel: 'Fiş fotoğrafı'),
                ),
              const SizedBox(height: 12),
              _ExtractionBanner(result: _result, error: _error),
              const SizedBox(height: 12),
              ProductForm(
                initial: _initialDraft(),
                categories: cats,
                highlight: {...?_result?.missing},
                onSubmit: _save,
                onScan: () => ref.read(codeScannerProvider).scan(context),
              ),
            ],
          ),
      },
    );
  }
}

class _ChooseSource extends StatelessWidget {
  const _ChooseSource(
      {required this.onCamera,
      required this.onGallery,
      required this.onManual});

  final VoidCallback onCamera;
  final VoidCallback onGallery;
  final VoidCallback onManual;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Icon(Icons.receipt_long, size: 72, color: theme.colorScheme.primary),
          const SizedBox(height: 16),
          Text('Fişin fotoğrafını çek, gerisini uygulama halletsin.',
              textAlign: TextAlign.center, style: theme.textTheme.titleMedium),
          const SizedBox(height: 32),
          FilledButton.icon(
            key: const Key('pick_camera'),
            onPressed: onCamera,
            icon: const Icon(Icons.photo_camera),
            label: const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: Text('Fiş / fatura fotoğrafı çek')),
          ),
          const SizedBox(height: 12),
          OutlinedButton.icon(
            key: const Key('pick_gallery'),
            onPressed: onGallery,
            icon: const Icon(Icons.photo_library),
            label: const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: Text('Galeriden seç')),
          ),
          const SizedBox(height: 12),
          TextButton(
              key: const Key('manual'),
              onPressed: onManual,
              child: const Text('Bilgileri elle gir')),
        ],
      ),
    );
  }
}

class _Reading extends StatelessWidget {
  const _Reading();

  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          CircularProgressIndicator(),
          SizedBox(height: 16),
          Text('Fiş okunuyor…',
              semanticsLabel: 'Fiş okunuyor, lütfen bekleyin'),
        ],
      ),
    );
  }
}

class _ExtractionBanner extends StatelessWidget {
  const _ExtractionBanner({required this.result, required this.error});

  final ExtractionResult? result;
  final String? error;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    String text;
    IconData icon;
    if (error != null) {
      text = error!;
      icon = Icons.warning_amber;
    } else if (result == null) {
      text =
          'Ürün bilgilerini girin. Garanti bitiş tarihi otomatik hesaplanır.';
      icon = Icons.edit;
    } else if (result!.provider == 'none') {
      text =
          'Fiş otomatik okunamadı. Lütfen bilgileri girin; fotoğraf fatura olarak kaydedilecek.';
      icon = Icons.warning_amber;
    } else {
      final who =
          result!.provider == 'gemini' ? 'Yapay zekâ' : 'Otomatik okuma';
      final missing =
          result!.missing.isEmpty ? '' : ' İşaretli alanları kontrol edin.';
      final warranty = result!.warrantyStated
          ? ''
          : ' Garanti süresi belgede yok, varsayılan 24 ay seçildi.';
      text = '$who fişten bilgileri çıkardı.$missing$warranty';
      icon = Icons.auto_awesome;
    }
    return Card(
      color: theme.colorScheme.secondaryContainer,
      child: ListTile(
          leading: Icon(icon),
          title: Text(text, key: const Key('extract_banner'))),
    );
  }
}
