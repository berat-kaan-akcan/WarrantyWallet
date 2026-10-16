/// API data models with hand-written JSON mapping (no code generation needed).
library;

import '../core/formatters.dart';
import '../core/warranty.dart';

String _s(Object? v) => v is String ? v : (v?.toString() ?? '');
double _d(Object? v) => v is num ? v.toDouble() : (double.tryParse(_s(v)) ?? 0);
int _i(Object? v) => v is num ? v.toInt() : (int.tryParse(_s(v)) ?? 0);
DateTime? _date(Object? v) => v is String ? parseIsoDate(v) : null;
DateTime? _ts(Object? v) => v is String ? DateTime.tryParse(v) : null;
List<Map<String, dynamic>> _list(Object? v) =>
    v is List ? v.whereType<Map<String, dynamic>>().toList() : const [];

/// Logged-in user.
class AppUser {
  const AppUser(
      {required this.id,
      required this.email,
      required this.name,
      required this.role});

  factory AppUser.fromJson(Map<String, dynamic> j) => AppUser(
      id: _s(j['id']),
      email: _s(j['email']),
      name: _s(j['name']),
      role: _s(j['role']));

  final String id;
  final String email;
  final String name;
  final String role;

  bool get isAdmin => role == 'admin';

  /// First name for the greeting.
  String get firstName {
    final parts = name.trim().split(RegExp(r'\s+'));
    return parts.isEmpty || parts.first.isEmpty ? '' : parts.first;
  }
}

/// Product category.
class ProductCategory {
  const ProductCategory(this.slug, this.name);

  factory ProductCategory.fromJson(Map<String, dynamic> j) =>
      ProductCategory(_s(j['slug']), _s(j['name_tr']));

  final String slug;
  final String name;
}

/// Fallback list used before the API responds (matches backend defaults).
const defaultCategories = <ProductCategory>[
  ProductCategory('phone', 'Telefon'),
  ProductCategory('laptop', 'Laptop'),
  ProductCategory('monitor', 'Monitör'),
  ProductCategory('tv', 'Televizyon'),
  ProductCategory('headphones', 'Kulaklık'),
  ProductCategory('tablet', 'Tablet'),
  ProductCategory('appliance', 'Beyaz Eşya'),
  ProductCategory('small_appliance', 'Küçük Ev Aleti'),
  ProductCategory('camera', 'Kamera'),
  ProductCategory('console', 'Oyun Konsolu'),
  ProductCategory('wearable', 'Akıllı Saat'),
  ProductCategory('other', 'Diğer'),
];

/// Turkish name for a category slug.
String categoryName(String slug,
    [List<ProductCategory> cats = defaultCategories]) {
  for (final c in cats) {
    if (c.slug == slug) return c.name;
  }
  return slug.isEmpty ? 'Diğer' : slug;
}

/// A purchased product with derived warranty info.
class Product {
  const Product({
    required this.id,
    required this.name,
    this.brand = '',
    this.model = '',
    this.category = 'other',
    this.seller = '',
    this.purchaseDate,
    this.price = 0,
    this.currency = 'TRY',
    this.warrantyMonths = defaultWarrantyMonths,
    this.warrantyEnd,
    this.serialNumber = '',
    this.notes = '',
    this.status = WarrantyStatus.unknown,
    this.daysLeft,
  });

  factory Product.fromJson(Map<String, dynamic> j) => Product(
        id: _s(j['id']),
        name: _s(j['name']),
        brand: _s(j['brand']),
        model: _s(j['model']),
        category: _s(j['category']).isEmpty ? 'other' : _s(j['category']),
        seller: _s(j['seller']),
        purchaseDate: _date(j['purchase_date']),
        price: _d(j['price']),
        currency: _s(j['currency']).isEmpty ? 'TRY' : _s(j['currency']),
        warrantyMonths: _i(j['warranty_months']),
        warrantyEnd: _date(j['warranty_end']),
        serialNumber: _s(j['serial_number']),
        notes: _s(j['notes']),
        status: warrantyStatusFromApi(j['status'] as String?),
        daysLeft:
            j['days_left'] is num ? (j['days_left'] as num).toInt() : null,
      );

  final String id;
  final String name;
  final String brand;
  final String model;
  final String category;
  final String seller;
  final DateTime? purchaseDate;
  final double price;
  final String currency;
  final int warrantyMonths;
  final DateTime? warrantyEnd;
  final String serialNumber;
  final String notes;
  final WarrantyStatus status;
  final int? daysLeft;

  bool get isCovered =>
      status == WarrantyStatus.active || status == WarrantyStatus.expiringSoon;
}

/// Editable product fields sent to the API (warranty_end is computed server-side).
class ProductDraft {
  ProductDraft({
    this.name = '',
    this.brand = '',
    this.model = '',
    this.category = 'other',
    this.seller = '',
    this.purchaseDate,
    this.price = 0,
    this.currency = 'TRY',
    this.warrantyMonths = defaultWarrantyMonths,
    this.serialNumber = '',
    this.notes = '',
  });

  factory ProductDraft.fromProduct(Product p) => ProductDraft(
        name: p.name,
        brand: p.brand,
        model: p.model,
        category: p.category,
        seller: p.seller,
        purchaseDate: p.purchaseDate,
        price: p.price,
        currency: p.currency,
        warrantyMonths: p.warrantyMonths,
        serialNumber: p.serialNumber,
        notes: p.notes,
      );

  String name;
  String brand;
  String model;
  String category;
  String seller;
  DateTime? purchaseDate;
  double price;
  String currency;
  int warrantyMonths;
  String serialNumber;
  String notes;

  /// Local preview of the end date.
  DateTime? get warrantyEndPreview => warrantyEnd(purchaseDate, warrantyMonths);

  Map<String, dynamic> toJson() => {
        'name': name.trim(),
        'brand': brand.trim(),
        'model': model.trim(),
        'category': category,
        'seller': seller.trim(),
        'purchase_date': purchaseDate == null ? null : isoDate(purchaseDate!),
        'price': price,
        'currency': currency,
        'warranty_months': warrantyMonths,
        'serial_number': serialNumber.trim(),
        'notes': notes.trim(),
      };
}

/// Stored file.
class ProductDocument {
  const ProductDocument({
    required this.id,
    required this.type,
    required this.contentType,
    required this.originalName,
    this.sizeBytes = 0,
    this.serviceRecordId = '',
  });

  factory ProductDocument.fromJson(Map<String, dynamic> j) => ProductDocument(
        id: _s(j['id']),
        type: _s(j['type']),
        contentType: _s(j['content_type']),
        originalName: _s(j['original_name']),
        sizeBytes: _i(j['size_bytes']),
        serviceRecordId: _s(j['service_record_id']),
      );

  final String id;
  final String type; // invoice | warranty | service | photo
  final String contentType;
  final String originalName;
  final int sizeBytes;
  final String serviceRecordId;

  bool get isImage => contentType.startsWith('image/');
}

/// Problem choices for the "Ürünüm Arızalandı" flow.
enum ProblemType {
  wontTurnOn('wont_turn_on', 'Açılmıyor'),
  screenLines('screen_lines', 'Ekranda çizgiler'),
  physicalDamage('physical_damage', 'Fiziksel hasar'),
  other('other', 'Diğer');

  const ProblemType(this.api, this.label);

  final String api;
  final String label;

  static ProblemType fromApi(String v) => ProblemType.values
      .firstWhere((p) => p.api == v, orElse: () => ProblemType.other);
}

/// Service lifecycle.
enum ServiceStatus {
  open('open', 'Açık'),
  inService('in_service', 'Serviste'),
  resolved('resolved', 'Çözüldü'),
  cancelled('cancelled', 'İptal');

  const ServiceStatus(this.api, this.label);

  final String api;
  final String label;

  static ServiceStatus fromApi(String v) => ServiceStatus.values
      .firstWhere((s) => s.api == v, orElse: () => ServiceStatus.open);
}

/// Document checklist entry for the service visit.
class ChecklistItem {
  const ChecklistItem(this.key, this.label, this.ok);

  factory ChecklistItem.fromJson(Map<String, dynamic> j) =>
      ChecklistItem(_s(j['key']), _s(j['label']), j['ok'] == true);

  final String key;
  final String label;
  final bool ok;
}

/// A service/repair case.
class ServiceRecord {
  const ServiceRecord({
    required this.id,
    required this.productId,
    required this.problem,
    required this.status,
    this.description = '',
    this.serviceName = '',
    this.serviceDate,
    this.notes = '',
    this.underWarranty = false,
    this.productName = '',
    this.createdAt,
    this.checklist = const [],
  });

  factory ServiceRecord.fromJson(Map<String, dynamic> j) => ServiceRecord(
        id: _s(j['id']),
        productId: _s(j['product_id']),
        problem: ProblemType.fromApi(_s(j['problem'])),
        status: ServiceStatus.fromApi(_s(j['status'])),
        description: _s(j['description']),
        serviceName: _s(j['service_name']),
        serviceDate: _date(j['service_date']),
        notes: _s(j['notes']),
        underWarranty: j['under_warranty'] == true,
        productName: _s(j['product_name']),
        createdAt: _ts(j['created_at']),
        checklist: _list(j['checklist']).map(ChecklistItem.fromJson).toList(),
      );

  final String id;
  final String productId;
  final ProblemType problem;
  final ServiceStatus status;
  final String description;
  final String serviceName;
  final DateTime? serviceDate;
  final String notes;
  final bool underWarranty;
  final String productName;
  final DateTime? createdAt;
  final List<ChecklistItem> checklist;
}

/// Result of POST /extract.
class ExtractionResult {
  const ExtractionResult({
    required this.draft,
    required this.provider,
    required this.missing,
    required this.warnings,
    required this.warrantyStated,
    this.rawText = '',
  });

  factory ExtractionResult.fromJson(Map<String, dynamic> j) {
    final f = (j['fields'] as Map<String, dynamic>?) ?? const {};
    final months = _i(f['warranty_months']);
    final suggested = _i(j['suggested_warranty_months']);
    return ExtractionResult(
      draft: ProductDraft(
        name: _s(f['product_name']),
        brand: _s(f['brand']),
        model: _s(f['model']),
        category: _s(f['category']).isEmpty ? 'other' : _s(f['category']),
        seller: _s(f['seller']),
        purchaseDate: _date(f['purchase_date']),
        price: _d(f['price']),
        currency: _s(f['currency']).isEmpty ? 'TRY' : _s(f['currency']),
        warrantyMonths: months > 0
            ? months
            : (suggested > 0 ? suggested : defaultWarrantyMonths),
        serialNumber: _s(f['serial_number']),
      ),
      provider: _s(j['provider']),
      missing: (j['missing'] as List?)?.map(_s).toList() ?? const [],
      warnings: (j['warnings'] as List?)?.map(_s).toList() ?? const [],
      warrantyStated: months > 0,
      rawText: _s(j['raw_text']),
    );
  }

  final ProductDraft draft;
  final String provider; // gemini | rules | none
  final List<String> missing;
  final List<String> warnings;

  /// Whether the warranty period came from the document (else the user picks it).
  final bool warrantyStated;
  final String rawText;
}

/// ProductCategory stats on the dashboard.
class CategoryStat {
  const CategoryStat(this.category, this.name, this.count, this.total);

  factory CategoryStat.fromJson(Map<String, dynamic> j) => CategoryStat(
      _s(j['category']), _s(j['name']), _i(j['count']), _d(j['total']));

  final String category;
  final String name;
  final int count;
  final double total;
}

/// GET /dashboard.
class Dashboard {
  const Dashboard({
    this.greeting = 'Merhaba',
    this.totalProducts = 0,
    this.activeWarranties = 0,
    this.expiringSoon = 0,
    this.expired = 0,
    this.totalSpent = const {},
    this.byCategory = const [],
    this.upcoming = const [],
    this.openServiceRecords = 0,
  });

  factory Dashboard.fromJson(Map<String, dynamic> j) {
    final spent = <String, double>{};
    final raw = j['total_spent'];
    if (raw is Map) {
      raw.forEach((k, v) => spent[k.toString()] = _d(v));
    }
    return Dashboard(
      greeting: _s(j['greeting']).isEmpty ? 'Merhaba' : _s(j['greeting']),
      totalProducts: _i(j['total_products']),
      activeWarranties: _i(j['active_warranties']),
      expiringSoon: _i(j['expiring_soon']),
      expired: _i(j['expired']),
      totalSpent: spent,
      byCategory: _list(j['by_category']).map(CategoryStat.fromJson).toList(),
      upcoming: _list(j['upcoming']).map(Product.fromJson).toList(),
      openServiceRecords: _i(j['open_service_records']),
    );
  }

  final String greeting;
  final int totalProducts;
  final int activeWarranties;
  final int expiringSoon;
  final int expired;
  final Map<String, double> totalSpent;
  final List<CategoryStat> byCategory;
  final List<Product> upcoming;
  final int openServiceRecords;
}
