import 'dart:typed_data';

import 'package:warranty_wallet/core/api_client.dart';
import 'package:warranty_wallet/core/warranty.dart';
import 'package:warranty_wallet/data/repository.dart';
import 'package:warranty_wallet/models/models.dart';
import 'package:warranty_wallet/services/services.dart';

const testUser =
    AppUser(id: 'u1', email: 'berat@test.io', name: 'Berat Kaan', role: 'user');

Product monitorProduct({int daysLeft = 697}) => Product(
      id: 'p1',
      name: 'ASUS TUF VG27AQ',
      brand: 'ASUS',
      model: 'VG27AQ',
      category: 'monitor',
      seller: 'MediaMarkt',
      purchaseDate: DateTime.utc(2026, 9, 12),
      price: 12499,
      warrantyMonths: 24,
      warrantyEnd: DateTime.utc(2028, 9, 12),
      serialNumber: 'M1LMQS123',
      status:
          daysLeft <= 30 ? WarrantyStatus.expiringSoon : WarrantyStatus.active,
      daysLeft: daysLeft,
    );

final headphones = Product(
  id: 'p2',
  name: 'Sony WH-1000XM5',
  brand: 'Sony',
  category: 'headphones',
  purchaseDate: DateTime.utc(2024, 11, 1),
  price: 8999.9,
  warrantyEnd: DateTime.utc(2026, 11, 1),
  status: WarrantyStatus.expiringSoon,
  daysLeft: 16,
);

/// In-memory fake used by widget tests.
class FakeRepository implements WarrantyRepository {
  FakeRepository({List<Product>? products, this.extraction})
      : items = products ?? [monitorProduct(), headphones];

  List<Product> items;
  ExtractionResult? extraction;
  final created = <ProductDraft>[];
  final uploads = <String>[];
  String? loginError;
  Object? diagnoseError;
  final diagnoseCalls = <String>[];
  Diagnosis diagnosis = const Diagnosis(
    summary: 'Panel kablosu gevşemiş olabilir.',
    causes: [
      DiagnosisCause('Panel kablosu', 'high', 'Bağlantı gevşemiş olabilir.')
    ],
    steps: [DiagnosisStep('Yeniden başlatın', 'Kapatıp 1 dakika sonra açın.')],
    safetyWarning: 'Cihazı fişten çekin.',
    recommendService: true,
    warrantyAdvice: 'Ürününüz garanti kapsamında.',
    provider: 'gemini',
    model: 'gemini-test',
    disclaimer: 'Genel bilgilendirme amaçlıdır.',
  );

  @override
  Future<AuthSession> login(String email, String password) async {
    if (loginError != null) throw ApiException(401, loginError!, 'bad');
    return const AuthSession('tok', testUser, refreshToken: 'rtok');
  }

  @override
  Future<AuthSession> register(
          String email, String password, String name) async =>
      const AuthSession('tok', testUser);

  AppUser user = testUser;
  bool resendResult = true;
  int resendCalls = 0;

  @override
  Future<AppUser> me() async => user;

  @override
  Future<bool> resendVerification() async {
    resendCalls++;
    return resendResult;
  }

  @override
  Future<AppUser> updateProfile({required String name}) async => AppUser(
      id: testUser.id, email: testUser.email, name: name, role: testUser.role);

  @override
  Future<void> deleteAccount() async {}

  final calls = <String>[];

  @override
  Future<void> logout(String? refreshToken) async =>
      calls.add('logout:$refreshToken');

  Uint8List backup = Uint8List.fromList([80, 75, 5, 6]);
  ImportSummary importResult = const ImportSummary(
      products: 2,
      serviceRecords: 1,
      documents: 3,
      skippedDuplicates: 1,
      failed: 1,
      errors: ['"Kettle" belgesi atlandı: dosya türü desteklenmiyor']);
  final imported = <Uint8List>[];

  final centerQueries = <String>[];

  @override
  Future<ServiceCenterSearch> serviceCenters(String productId,
      {String city = ''}) async {
    centerQueries.add('$productId:$city');
    final items = [
      const ServiceCenter(
          id: 'c1',
          brand: 'ASUS',
          name: 'ASUS Kadıköy Servis',
          city: 'İstanbul',
          district: 'Kadıköy',
          phone: '+90 (216) 000 00 00',
          distanceKm: 2.4),
    ];
    return ServiceCenterSearch(
      brand: 'ASUS',
      items: city.isEmpty || city == 'İstanbul' ? items : const [],
      mapsSearchUrl:
          'https://www.google.com/maps/search/?api=1&query=ASUS+yetkili+servis+${Uri.encodeQueryComponent(city)}',
    );
  }

  FamilyInfo familyInfo = const FamilyInfo();
  String inviteCode = 'K7QM-2XRP';
  List<FamilyProduct> familyItems = [];

  @override
  Future<FamilyInfo> family() async => familyInfo;

  @override
  Future<FamilyInfo> createFamily(String name) async {
    calls.add('family+:$name');
    return familyInfo =
        FamilyInfo(id: 'f1', name: name, isOwner: true, members: [
      FamilyMember(
          id: testUser.id,
          name: testUser.name,
          email: testUser.email,
          isOwner: true)
    ]);
  }

  @override
  Future<String> createFamilyInvite() async => inviteCode;

  @override
  Future<FamilyInfo> joinFamily(String code) async {
    calls.add('join:$code');
    if (code.trim().toUpperCase() != inviteCode) {
      throw ApiException(400, 'invalid_invite_code', 'bad');
    }
    return familyInfo = FamilyInfo(id: 'f1', name: 'Akcan ailesi', members: [
      const FamilyMember(
          id: 'u0', name: 'Ayşe', email: 'ayse@test.io', isOwner: true),
      FamilyMember(
          id: testUser.id,
          name: testUser.name,
          email: testUser.email,
          isOwner: false),
    ]);
  }

  @override
  Future<void> removeFamilyMember(String userId) async {
    calls.add('family-:$userId');
    if (userId == testUser.id) {
      familyInfo = const FamilyInfo();
    } else {
      familyInfo = FamilyInfo(
          id: familyInfo.id,
          name: familyInfo.name,
          isOwner: familyInfo.isOwner,
          members: [
            for (final m in familyInfo.members)
              if (m.id != userId) m
          ]);
    }
  }

  @override
  Future<List<FamilyProduct>> familyProducts() async => familyItems;

  @override
  Future<Uint8List> exportBackup() async => backup;

  @override
  Future<ImportSummary> importBackup(Uint8List zip) async {
    imported.add(zip);
    return importResult;
  }

  @override
  Future<void> registerDevice(String token, String platform) async =>
      calls.add('device+:$token:$platform');

  @override
  Future<void> unregisterDevice(String token) async =>
      calls.add('device-:$token');

  @override
  Future<void> forgotPassword(String email) async => calls.add('forgot:$email');

  @override
  Future<void> resetPassword(String token, String newPassword) async {
    if (token != 'good') {
      throw const ApiException(400, 'invalid_reset_token', 'bad');
    }
    calls.add('reset:$token');
  }

  @override
  Future<void> changePassword(
      String currentPassword, String newPassword) async {
    if (currentPassword != 'supersecret') {
      throw const ApiException(422, 'validation_failed', 'invalid input',
          {'current_password': 'incorrect password'});
    }
    calls.add('change');
  }

  @override
  Future<List<ProductCategory>> categories() async => defaultCategories;

  @override
  Future<List<Product>> products(
          {String category = '',
          String status = '',
          String query = '',
          String code = ''}) async =>
      items
          .where((p) => category.isEmpty || p.category == category)
          .where((p) =>
              code.isEmpty ||
              p.serialNumber.toLowerCase() == code.toLowerCase() ||
              p.barcode.toLowerCase() == code.toLowerCase())
          .where((p) =>
              query.isEmpty ||
              p.name.toLowerCase().contains(query.toLowerCase()))
          .toList();

  @override
  Future<List<Product>> expiring({int days = 90}) async => items
      .where(
          (p) => p.daysLeft != null && p.daysLeft! >= 0 && p.daysLeft! <= days)
      .toList();

  @override
  Future<Product> product(String id) async =>
      items.firstWhere((p) => p.id == id);

  @override
  Future<Product> createProduct(ProductDraft draft) async {
    created.add(draft);
    final p = Product(
      id: 'new${created.length}',
      name: draft.name,
      category: draft.category,
      purchaseDate: draft.purchaseDate,
      warrantyMonths: draft.warrantyMonths,
      warrantyEnd: draft.warrantyEndPreview,
      status: WarrantyStatus.active,
      daysLeft: 700,
    );
    items = [...items, p];
    return p;
  }

  @override
  Future<Product> updateProduct(String id, ProductDraft draft) async =>
      product(id);

  @override
  Future<void> deleteProduct(String id) async =>
      items = items.where((p) => p.id != id).toList();

  @override
  Future<ExtractionResult> extract(
          {Uint8List? image,
          String imageName = 'fis.jpg',
          String text = ''}) async =>
      extraction ??
      ExtractionResult.fromJson(const {
        'fields': <String, dynamic>{},
        'provider': 'none',
        'missing': <String>[]
      });

  @override
  Future<List<ProductDocument>> documents(String productId,
          {String type = ''}) async =>
      const [];

  @override
  Future<ProductDocument> uploadDocument(
      String productId, String type, Uint8List bytes, String filename,
      {String serviceRecordId = ''}) async {
    uploads.add('$productId/$type/$filename');
    return ProductDocument(
        id: 'd1',
        type: type,
        contentType: 'image/jpeg',
        originalName: filename);
  }

  @override
  Future<Uint8List> documentBytes(String documentId) async => Uint8List(0);

  @override
  Future<void> deleteDocument(String documentId) async {}

  @override
  String documentUrl(String documentId) => 'http://test/$documentId';

  @override
  Future<Map<String, String>> authHeaders() async => const {};

  @override
  Future<ServiceRecord> createServiceRecord(
          String productId, ProblemType problem, String description) async =>
      ServiceRecord(
        id: 's1',
        productId: productId,
        problem: problem,
        status: ServiceStatus.open,
        underWarranty: true,
        checklist: const [
          ChecklistItem('invoice', 'Fatura / fiş', true),
          ChecklistItem('warranty', 'Garanti belgesi', false),
        ],
      );

  @override
  Future<List<ServiceRecord>> serviceRecords({String productId = ''}) async =>
      const [];

  List<ServiceRecord> services = [];
  ServiceSummary serviceSummary = const ServiceSummary();
  final serviceUpdates = <Map<String, Object?>>[];
  final deletedServices = <String>[];

  @override
  Future<ServiceHistory> serviceHistory({ServiceStatus? status}) async =>
      ServiceHistory(
          services.where((r) => status == null || r.status == status).toList(),
          serviceSummary);

  @override
  Future<ServiceRecord> serviceRecord(String id) async =>
      services.firstWhere((r) => r.id == id,
          orElse: () => throw ApiException(404, 'not_found', 'nf'));

  @override
  Future<void> deleteServiceRecord(String id) async {
    deletedServices.add(id);
    services = services.where((r) => r.id != id).toList();
  }

  @override
  Future<ServiceRecord> updateServiceRecord(String id,
      {ServiceStatus? status,
      String? serviceName,
      DateTime? serviceDate,
      String? notes,
      double? cost,
      String? eventNote}) async {
    serviceUpdates.add({
      'id': id,
      'status': status,
      'cost': cost,
      'event_note': eventNote,
    });
    return ServiceRecord(
        id: id,
        productId: 'p1',
        problem: ProblemType.other,
        status: status ?? ServiceStatus.open);
  }

  @override
  Future<ChecklistResult> checklist(String productId) async =>
      const ChecklistResult(true, [], 10);

  @override
  Future<Diagnosis> diagnose(
      String productId, ProblemType problem, String description) async {
    diagnoseCalls.add('${problem.api}:$description');
    if (diagnoseError != null) throw diagnoseError!;
    return diagnosis;
  }

  @override
  Future<Uint8List> servicePack(String productId,
          {String serviceRecordId = ''}) async =>
      Uint8List(4);

  @override
  Future<Dashboard> dashboard() async => Dashboard(
        greeting: 'Günaydın, Berat',
        totalProducts: items.length,
        activeWarranties: items.where((p) => p.isCovered).length,
        expiringSoon: 1,
        expired: 0,
        totalSpent: const {'TRY': 21498.9},
        byCategory: const [
          CategoryStat('monitor', 'Monitör', 1, 12499),
          CategoryStat('headphones', 'Kulaklık', 1, 8999.9)
        ],
        upcoming: [headphones],
      );
}

class FakePushService implements PushService {
  FakePushService([this.value]);
  final String? value;

  @override
  Future<String?> token() async => value;

  @override
  String get platform => 'android';
}

class FakeBackupFiles implements BackupFiles {
  final shared = <String, Uint8List>{};
  Uint8List? toPick;

  @override
  Future<void> share(Uint8List bytes, String filename) async =>
      shared[filename] = bytes;

  @override
  Future<Uint8List?> pickZip() async => toPick;
}

class FakeLinkOpener implements LinkOpener {
  final opened = <Uri>[];
  bool result = true;

  @override
  Future<bool> open(Uri uri) async {
    opened.add(uri);
    return result;
  }
}
