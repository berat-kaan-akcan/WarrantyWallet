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
      required this.role,
      this.emailVerified = true});

  factory AppUser.fromJson(Map<String, dynamic> j) => AppUser(
      id: _s(j['id']),
      email: _s(j['email']),
      name: _s(j['name']),
      role: _s(j['role']),
      // Older backends do not send the field: treat as verified (no banner).
      emailVerified: !j.containsKey('email_verified_at') ||
          j['email_verified_at'] != null);

  final String id;
  final String email;
  final String name;
  final String role;
  final bool emailVerified;

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
    this.barcode = '',
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
        barcode: _s(j['barcode']),
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
  final String barcode;
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
    this.barcode = '',
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
        barcode: p.barcode,
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
  String barcode;
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
        'barcode': barcode.trim(),
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
    this.cost = 0,
    this.resolvedAt,
    this.events = const [],
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
        cost: _d(j['cost']),
        resolvedAt: _ts(j['resolved_at']),
        events: _list(j['events']).map(ServiceEvent.fromJson).toList(),
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
  final double cost;
  final DateTime? resolvedAt;
  final List<ServiceEvent> events; // only filled by the detail endpoint

  bool get isClosed =>
      status == ServiceStatus.resolved || status == ServiceStatus.cancelled;

  /// Whole days from opening to resolution (null while open).
  int? get resolutionDays => createdAt == null || resolvedAt == null
      ? null
      : resolvedAt!.difference(createdAt!).inDays;
}

/// One status change on a service record's timeline.
class ServiceEvent {
  const ServiceEvent(this.status, this.note, this.createdAt);

  factory ServiceEvent.fromJson(Map<String, dynamic> j) => ServiceEvent(
      ServiceStatus.fromApi(_s(j['status'])),
      _s(j['note']),
      _ts(j['created_at']));

  final ServiceStatus status;
  final String note;
  final DateTime? createdAt;
}

/// Aggregates for the service history screen.
class ServiceSummary {
  const ServiceSummary({
    this.total = 0,
    this.active = 0,
    this.byStatus = const {},
    this.totalCost = 0,
    this.avgResolutionDays,
  });

  factory ServiceSummary.fromJson(Map<String, dynamic> j) => ServiceSummary(
        total: _i(j['total']),
        active: _i(j['active']),
        byStatus: {
          for (final e in ((j['by_status'] as Map?) ?? const {}).entries)
            ServiceStatus.fromApi(_s(e.key)): _i(e.value),
        },
        totalCost: _d(j['total_cost']),
        avgResolutionDays: j['avg_resolution_days'] is num
            ? (j['avg_resolution_days'] as num).toDouble()
            : null,
      );

  final int total;
  final int active;
  final Map<ServiceStatus, int> byStatus;
  final double totalCost;
  final double? avgResolutionDays;

  int count(ServiceStatus s) => byStatus[s] ?? 0;
}

/// Result of GET /service-records.
class ServiceHistory {
  const ServiceHistory(this.items, this.summary);

  final List<ServiceRecord> items;
  final ServiceSummary summary;
}

/// One possible cause from the AI fault assistant.
class DiagnosisCause {
  const DiagnosisCause(this.cause, this.likelihood, this.explanation);

  factory DiagnosisCause.fromJson(Map<String, dynamic> j) =>
      DiagnosisCause(_s(j['cause']), _s(j['likelihood']), _s(j['explanation']));

  final String cause;
  final String likelihood; // high | medium | low
  final String explanation;

  String get likelihoodLabel => switch (likelihood) {
        'high' => 'Yüksek olasılık',
        'low' => 'Düşük olasılık',
        _ => 'Orta olasılık',
      };
}

/// A safe step the user can try.
class DiagnosisStep {
  const DiagnosisStep(this.title, this.detail);

  factory DiagnosisStep.fromJson(Map<String, dynamic> j) =>
      DiagnosisStep(_s(j['title']), _s(j['detail']));

  final String title;
  final String detail;
}

/// Result of POST /products/{id}/diagnose.
class Diagnosis {
  const Diagnosis({
    required this.summary,
    this.causes = const [],
    this.steps = const [],
    this.safetyWarning = '',
    this.recommendService = false,
    this.warrantyAdvice = '',
    this.provider = 'rules',
    this.model = '',
    this.disclaimer = '',
    this.aiUnavailable = false,
  });

  factory Diagnosis.fromJson(Map<String, dynamic> j) => Diagnosis(
        summary: _s(j['summary']),
        causes: _list(j['likely_causes']).map(DiagnosisCause.fromJson).toList(),
        steps: _list(j['steps']).map(DiagnosisStep.fromJson).toList(),
        safetyWarning: _s(j['safety_warning']),
        recommendService: j['recommend_service'] == true,
        warrantyAdvice: _s(j['warranty_advice']),
        provider: _s(j['provider']),
        model: _s(j['model']),
        disclaimer: _s(j['disclaimer']),
        aiUnavailable:
            j['warnings'] is List && (j['warnings'] as List).isNotEmpty,
      );

  final String summary;
  final List<DiagnosisCause> causes;
  final List<DiagnosisStep> steps;
  final String safetyWarning;
  final bool recommendService;
  final String warrantyAdvice;
  final String provider;
  final String model;
  final String disclaimer;
  final bool aiUnavailable;

  bool get isAi => provider == 'gemini';
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
    this.analytics,
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
      analytics: j['analytics'] is Map<String, dynamic>
          ? DashboardAnalytics.fromJson(j['analytics'] as Map<String, dynamic>)
          : null,
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
  final DashboardAnalytics? analytics; // null on older backends
}

/// One month bucket (yyyy-MM).
class MonthStat {
  const MonthStat(this.month, this.count, [this.total = 0]);

  factory MonthStat.fromJson(Map<String, dynamic> j) =>
      MonthStat(_s(j['month']), _i(j['count']), _d(j['total']));

  final String month;
  final int count;
  final double total;

  static const _names = [
    'Oca', 'Şub', 'Mar', 'Nis', 'May', 'Haz', //
    'Tem', 'Ağu', 'Eyl', 'Eki', 'Kas', 'Ara',
  ];

  /// Short Turkish month name ("Eki").
  String get shortLabel {
    final m = int.tryParse(month.length >= 7 ? month.substring(5, 7) : '');
    return m == null || m < 1 || m > 12 ? month : _names[m - 1];
  }

  /// "Ekim 2026"-style label for screen readers.
  String get longLabel {
    const long = [
      'Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', //
      'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık',
    ];
    final m = int.tryParse(month.length >= 7 ? month.substring(5, 7) : '');
    if (m == null || m < 1 || m > 12) return month;
    return '${long[m - 1]} ${month.substring(0, 4)}';
  }
}

/// V2 dashboard analytics.
class DashboardAnalytics {
  const DashboardAnalytics({
    this.primaryCurrency = 'TRY',
    this.protectedValue = const {},
    this.coverageRate = 0,
    this.expiryTimeline = const [],
    this.monthlyPurchases = const [],
    this.topBrands = const [],
    this.serviceTotal = 0,
    this.serviceActive = 0,
    this.serviceResolved = 0,
    this.serviceCost = 0,
    this.avgResolutionDays,
  });

  factory DashboardAnalytics.fromJson(Map<String, dynamic> j) {
    final service = (j['service'] as Map<String, dynamic>?) ?? const {};
    final value = <String, double>{};
    final raw = j['protected_value'];
    if (raw is Map) raw.forEach((k, v) => value[k.toString()] = _d(v));
    return DashboardAnalytics(
      primaryCurrency:
          _s(j['primary_currency']).isEmpty ? 'TRY' : _s(j['primary_currency']),
      protectedValue: value,
      coverageRate: _d(j['coverage_rate']),
      expiryTimeline:
          _list(j['expiry_timeline']).map(MonthStat.fromJson).toList(),
      monthlyPurchases:
          _list(j['monthly_purchases']).map(MonthStat.fromJson).toList(),
      topBrands: [
        for (final b in _list(j['top_brands'])) (_s(b['brand']), _i(b['count']))
      ],
      serviceTotal: _i(service['total']),
      serviceActive: _i(service['active']),
      serviceResolved: _i(service['resolved']),
      serviceCost: _d(service['total_cost']),
      avgResolutionDays: service['avg_resolution_days'] is num
          ? (service['avg_resolution_days'] as num).toDouble()
          : null,
    );
  }

  final String primaryCurrency;
  final Map<String, double> protectedValue;
  final double coverageRate;
  final List<MonthStat> expiryTimeline;
  final List<MonthStat> monthlyPurchases;
  final List<(String, int)> topBrands;
  final int serviceTotal;
  final int serviceActive;
  final int serviceResolved;
  final double serviceCost;
  final double? avgResolutionDays;
}
