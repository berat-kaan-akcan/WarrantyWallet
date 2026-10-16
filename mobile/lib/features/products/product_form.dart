import 'package:flutter/material.dart';

import '../../core/formatters.dart';
import '../../core/scan_parser.dart';
import '../../core/warranty.dart';
import '../../models/models.dart';

/// Warranty periods offered in the dropdown (months).
const warrantyChoices = [0, 6, 12, 24, 36, 48, 60, 120];

/// Label for a warranty period.
String warrantyLabel(int months) {
  if (months == 0) return 'Garanti yok';
  if (months % 12 == 0 && months >= 12) {
    return '$months Ay (${months ~/ 12} yıl)';
  }
  return '$months Ay';
}

/// Editable product form shared by "Ürün Ekle" and "Düzenle".
/// Fields the extractor could not fill are highlighted so the user checks them.
class ProductForm extends StatefulWidget {
  const ProductForm({
    super.key,
    required this.initial,
    required this.onSubmit,
    this.categories = defaultCategories,
    this.highlight = const {},
    this.submitLabel = 'Kaydet',
    this.onScan,
  });

  final ProductDraft initial;
  final Future<void> Function(ProductDraft draft) onSubmit;
  final List<ProductCategory> categories;
  final Set<String> highlight;
  final String submitLabel;

  /// Opens the barcode/QR scanner; null hides the scan buttons.
  final Future<ScannedCode?> Function()? onScan;

  @override
  State<ProductForm> createState() => ProductFormState();
}

/// State is public so tests can inspect the draft.
class ProductFormState extends State<ProductForm> {
  final _formKey = GlobalKey<FormState>();
  late final ProductDraft draft;
  late final TextEditingController _name;
  late final TextEditingController _brand;
  late final TextEditingController _model;
  late final TextEditingController _seller;
  late final TextEditingController _price;
  late final TextEditingController _serial;
  late final TextEditingController _barcode;
  late final TextEditingController _notes;
  bool _saving = false;
  String? _dateError;

  @override
  void initState() {
    super.initState();
    final i = widget.initial;
    draft = ProductDraft(
      name: i.name,
      brand: i.brand,
      model: i.model,
      category: widget.categories.any((c) => c.slug == i.category)
          ? i.category
          : 'other',
      seller: i.seller,
      purchaseDate: i.purchaseDate,
      price: i.price,
      currency: i.currency,
      warrantyMonths: i.warrantyMonths,
      serialNumber: i.serialNumber,
      barcode: i.barcode,
      notes: i.notes,
    );
    _name = TextEditingController(text: i.name);
    _brand = TextEditingController(text: i.brand);
    _model = TextEditingController(text: i.model);
    _seller = TextEditingController(text: i.seller);
    _price = TextEditingController(
        text: i.price > 0 ? formatMoney(i.price, '').trim() : '');
    _serial = TextEditingController(text: i.serialNumber);
    _barcode = TextEditingController(text: i.barcode);
    _notes = TextEditingController(text: i.notes);
  }

  @override
  void dispose() {
    for (final c in [
      _name,
      _brand,
      _model,
      _seller,
      _price,
      _serial,
      _barcode,
      _notes
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  InputDecoration _dec(String label, String field, {String? helper}) {
    final warn = widget.highlight.contains(field);
    return InputDecoration(
      labelText: label,
      helperText: warn ? 'Fişten okunamadı, lütfen kontrol edin' : helper,
      helperStyle: warn
          ? TextStyle(color: Theme.of(context).colorScheme.tertiary)
          : null,
      border: const OutlineInputBorder(),
      suffixIcon: warn ? const Icon(Icons.edit_note) : null,
    );
  }

  Widget? _scanButton(String tooltip) {
    final scan = widget.onScan;
    if (scan == null) return null;
    return IconButton(
      tooltip: tooltip,
      icon: const Icon(Icons.qr_code_scanner),
      onPressed: () async {
        final code = await scan();
        if (code == null || !mounted) return;
        setState(() {
          if (code.serial.isNotEmpty) _serial.text = code.serial;
          if (code.barcode.isNotEmpty) _barcode.text = code.barcode;
        });
      },
    );
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: draft.purchaseDate ?? now,
      firstDate: DateTime(1990),
      lastDate: now,
      helpText: 'Satın alma tarihi',
    );
    if (picked != null) {
      setState(() {
        draft.purchaseDate =
            DateTime.utc(picked.year, picked.month, picked.day);
        _dateError = null;
      });
    }
  }

  Future<void> _submit() async {
    final valid = _formKey.currentState?.validate() ?? false;
    if (draft.purchaseDate == null) {
      setState(() => _dateError = 'Satın alma tarihi gerekli');
    }
    if (!valid || draft.purchaseDate == null) return;
    draft
      ..name = _name.text
      ..brand = _brand.text
      ..model = _model.text
      ..seller = _seller.text
      ..price = parseMoney(_price.text) ?? 0
      ..serialNumber = _serial.text
      ..barcode = _barcode.text
      ..notes = _notes.text;
    setState(() => _saving = true);
    try {
      await widget.onSubmit(draft);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final end = draft.warrantyEndPreview;
    final months = warrantyChoices.contains(draft.warrantyMonths)
        ? warrantyChoices
        : ([...warrantyChoices, draft.warrantyMonths]..sort());
    const gap = SizedBox(height: 12);
    return Form(
      key: _formKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextFormField(
            key: const Key('f_name'),
            controller: _name,
            decoration: _dec('Ürün adı *', 'product_name'),
            textCapitalization: TextCapitalization.words,
            validator: (v) =>
                (v ?? '').trim().isEmpty ? 'Ürün adı gerekli' : null,
          ),
          gap,
          Row(
            children: [
              Expanded(
                  child: TextFormField(
                      controller: _brand, decoration: _dec('Marka', 'brand'))),
              const SizedBox(width: 12),
              Expanded(
                  child: TextFormField(
                      controller: _model, decoration: _dec('Model', 'model'))),
            ],
          ),
          gap,
          DropdownButtonFormField<String>(
            key: const Key('f_category'),
            initialValue: draft.category,
            decoration: _dec('Kategori', 'category'),
            items: [
              for (final c in widget.categories)
                DropdownMenuItem(value: c.slug, child: Text(c.name))
            ],
            onChanged: (v) => setState(() => draft.category = v ?? 'other'),
          ),
          gap,
          TextFormField(
              controller: _seller, decoration: _dec('Satıcı', 'seller')),
          gap,
          InkWell(
            key: const Key('f_date'),
            onTap: _pickDate,
            child: InputDecorator(
              decoration: _dec('Satın alma tarihi *', 'purchase_date')
                  .copyWith(errorText: _dateError),
              child: Text(draft.purchaseDate == null
                  ? 'Tarih seçin'
                  : formatDate(draft.purchaseDate)),
            ),
          ),
          gap,
          Row(
            children: [
              Expanded(
                flex: 3,
                child: TextFormField(
                  key: const Key('f_price'),
                  controller: _price,
                  decoration: _dec('Fiyat', 'price'),
                  keyboardType:
                      const TextInputType.numberWithOptions(decimal: true),
                  validator: (v) =>
                      (v ?? '').trim().isNotEmpty && parseMoney(v!) == null
                          ? 'Geçerli bir tutar girin'
                          : null,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                flex: 2,
                child: DropdownButtonFormField<String>(
                  initialValue:
                      const ['TRY', 'USD', 'EUR'].contains(draft.currency)
                          ? draft.currency
                          : 'TRY',
                  decoration: const InputDecoration(
                      labelText: 'Para birimi', border: OutlineInputBorder()),
                  items: const [
                    DropdownMenuItem(value: 'TRY', child: Text('TL')),
                    DropdownMenuItem(value: 'USD', child: Text('USD')),
                    DropdownMenuItem(value: 'EUR', child: Text('EUR')),
                  ],
                  onChanged: (v) => setState(() => draft.currency = v ?? 'TRY'),
                ),
              ),
            ],
          ),
          gap,
          DropdownButtonFormField<int>(
            key: const Key('f_warranty'),
            initialValue: draft.warrantyMonths,
            decoration: _dec('Garanti süresi', 'warranty_months'),
            items: [
              for (final m in months)
                DropdownMenuItem(value: m, child: Text(warrantyLabel(m)))
            ],
            onChanged: (v) => setState(
                () => draft.warrantyMonths = v ?? defaultWarrantyMonths),
          ),
          const SizedBox(height: 8),
          Semantics(
            liveRegion: true,
            child: Card(
              child: ListTile(
                leading: const Icon(Icons.event_available),
                title: const Text('Garanti bitiş tarihi'),
                subtitle: Text(
                  end == null
                      ? 'Tarih ve süre seçince hesaplanır'
                      : formatDate(end),
                  key: const Key('f_end_preview'),
                ),
              ),
            ),
          ),
          gap,
          TextFormField(
            key: const Key('f_serial'),
            controller: _serial,
            decoration: _dec('Seri numarası', 'serial_number')
                .copyWith(suffixIcon: _scanButton('Seri numarasını tara')),
          ),
          gap,
          TextFormField(
            key: const Key('f_barcode'),
            controller: _barcode,
            maxLength: 128,
            decoration: _dec('Barkod (EAN / QR)', 'barcode',
                    helper: 'Kutudaki barkod; ürünü taratarak bulmanızı sağlar')
                .copyWith(suffixIcon: _scanButton('Barkodu tara')),
          ),
          gap,
          TextFormField(
            controller: _notes,
            decoration: _dec('Notlar', 'notes'),
            maxLines: 3,
            maxLength: 2000,
          ),
          gap,
          FilledButton.icon(
            key: const Key('f_submit'),
            onPressed: _saving ? null : _submit,
            icon: _saving
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.save),
            label: Padding(
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Text(widget.submitLabel),
            ),
          ),
        ],
      ),
    );
  }
}
