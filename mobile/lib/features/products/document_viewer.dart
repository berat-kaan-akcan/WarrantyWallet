import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';

import '../../core/api_client.dart';
import '../../models/models.dart';
import '../../state/providers.dart';
import '../../widgets/common.dart';

/// Saves bytes to a temp file and opens the system share sheet (PDFs, export).
Future<void> shareBytes(Uint8List bytes, String filename) async {
  final dir = await getTemporaryDirectory();
  final file = File('${dir.path}/$filename');
  await file.writeAsBytes(bytes, flush: true);
  await Share.shareXFiles([XFile(file.path)], subject: filename);
}

/// Shows stored invoices / warranty cards. Images are zoomable; PDFs can be shared.
class DocumentViewer extends ConsumerWidget {
  const DocumentViewer(
      {super.key,
      required this.documents,
      required this.title,
      this.productId});

  final List<ProductDocument> documents;
  final String title;

  /// When given, the shown document can be deleted (a wrong photo, a duplicate invoice).
  final String? productId;

  Future<void> _delete(
      BuildContext context, WidgetRef ref, ProductDocument doc) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Belge silinsin mi?'),
        content: Text('"${doc.originalName}" kalıcı olarak silinir.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Vazgeç')),
          TextButton(
              key: const Key('confirm_delete_document'),
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Sil')),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    final navigator = Navigator.of(context);
    try {
      await ref.read(repositoryProvider).deleteDocument(doc.id);
      ref.invalidate(documentsProvider(productId!));
      if (context.mounted) showSnack(context, 'Belge silindi');
      navigator.pop();
    } on ApiException catch (e) {
      if (context.mounted) showSnack(context, e.userMessage, error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return DefaultTabController(
      length: documents.length,
      child: Scaffold(
        appBar: AppBar(
          title: Text(title),
          actions: [
            if (productId != null)
              Builder(
                builder: (context) => IconButton(
                  key: const Key('delete_document'),
                  tooltip: 'Belgeyi sil',
                  icon: const Icon(Icons.delete_outline),
                  onPressed: () => _delete(context, ref,
                      documents[DefaultTabController.of(context).index]),
                ),
              ),
          ],
          bottom: documents.length > 1
              ? TabBar(
                  isScrollable: true,
                  tabs: [for (final d in documents) Tab(text: d.originalName)])
              : null,
        ),
        body: TabBarView(
            children: [for (final d in documents) _DocumentPage(doc: d)]),
      ),
    );
  }
}

class _DocumentPage extends ConsumerStatefulWidget {
  const _DocumentPage({required this.doc});

  final ProductDocument doc;

  @override
  ConsumerState<_DocumentPage> createState() => _DocumentPageState();
}

class _DocumentPageState extends ConsumerState<_DocumentPage> {
  late final Future<Uint8List> _bytes =
      ref.read(repositoryProvider).documentBytes(widget.doc.id);

  Future<void> _share(Uint8List data) async {
    try {
      await shareBytes(data,
          widget.doc.originalName.isEmpty ? 'belge' : widget.doc.originalName);
    } on Exception {
      if (mounted) showSnack(context, 'Paylaşılamadı', error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<Uint8List>(
      future: _bytes,
      builder: (context, snap) {
        if (snap.hasError) {
          return EmptyState(
              icon: Icons.broken_image,
              title: 'Belge yüklenemedi',
              message: errorMessage(snap.error!));
        }
        if (!snap.hasData) {
          return const LoadingView();
        }
        final data = snap.data!;
        if (widget.doc.isImage && widget.doc.contentType != 'image/heic') {
          return Stack(
            children: [
              Positioned.fill(
                child: InteractiveViewer(
                  maxScale: 5,
                  child: Image.memory(data,
                      fit: BoxFit.contain,
                      semanticLabel: widget.doc.originalName),
                ),
              ),
              Positioned(
                right: 16,
                bottom: 16,
                child: FloatingActionButton.small(
                  heroTag: widget.doc.id,
                  tooltip: 'Paylaş',
                  onPressed: () => _share(data),
                  child: const Icon(Icons.share),
                ),
              ),
            ],
          );
        }
        return EmptyState(
          icon: Icons.picture_as_pdf,
          title: widget.doc.originalName,
          message: 'Bu belge türü uygulama içinde önizlenemiyor.',
          action: FilledButton.icon(
            onPressed: () => _share(data),
            icon: const Icon(Icons.open_in_new),
            label: const Text('Aç / Paylaş'),
          ),
        );
      },
    );
  }
}
