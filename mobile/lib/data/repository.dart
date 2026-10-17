import 'dart:typed_data';

import '../core/api_client.dart';
import '../core/formatters.dart';
import '../models/models.dart';

/// Result of login/register.
class AuthSession {
  const AuthSession(this.token, this.user, {this.refreshToken});

  final String token;
  final AppUser user;
  final String? refreshToken;
}

/// Service checklist response.
class ChecklistResult {
  const ChecklistResult(this.underWarranty, this.items, this.daysLeft);

  final bool underWarranty;
  final List<ChecklistItem> items;
  final int? daysLeft;
}

/// All backend operations used by the UI. Widget tests substitute a fake implementation.
abstract class WarrantyRepository {
  Future<AuthSession> login(String email, String password);
  Future<AuthSession> register(String email, String password, String name);
  Future<AppUser> me();
  Future<AppUser> updateProfile({required String name});
  Future<void> deleteAccount();
  Future<void> logout(String? refreshToken);
  Future<void> registerDevice(String token, String platform);
  Future<Uint8List> exportBackup();
  Future<FamilyInfo> family();
  Future<FamilyInfo> createFamily(String name);
  Future<String> createFamilyInvite();
  Future<FamilyInfo> joinFamily(String code);
  Future<void> removeFamilyMember(String userId);
  Future<List<FamilyProduct>> familyProducts();
  Future<ImportSummary> importBackup(Uint8List zip);
  Future<void> unregisterDevice(String token);
  Future<void> forgotPassword(String email);

  /// Mails a new verification link; returns false when already verified.
  Future<bool> resendVerification();
  Future<void> resetPassword(String token, String newPassword);
  Future<void> changePassword(String currentPassword, String newPassword);

  Future<List<ProductCategory>> categories();
  Future<List<Product>> products(
      {String category = '',
      String status = '',
      String query = '',
      String code = ''});
  Future<List<Product>> expiring({int days = 90});
  Future<Product> product(String id);
  Future<Product> createProduct(ProductDraft draft);
  Future<Product> updateProduct(String id, ProductDraft draft);
  Future<void> deleteProduct(String id);

  Future<ExtractionResult> extract(
      {Uint8List? image, String imageName = 'fis.jpg', String text = ''});

  Future<List<ProductDocument>> documents(String productId, {String type = ''});
  Future<ProductDocument> uploadDocument(
      String productId, String type, Uint8List bytes, String filename,
      {String serviceRecordId = ''});
  Future<Uint8List> documentBytes(String documentId);
  Future<void> deleteDocument(String documentId);
  String documentUrl(String documentId);
  Future<Map<String, String>> authHeaders();

  Future<ServiceRecord> createServiceRecord(
      String productId, ProblemType problem, String description);
  Future<List<ServiceRecord>> serviceRecords({String productId = ''});
  Future<ServiceHistory> serviceHistory({ServiceStatus? status});
  Future<ServiceRecord> serviceRecord(String id);
  Future<void> deleteServiceRecord(String id);
  Future<ServiceRecord> updateServiceRecord(String id,
      {ServiceStatus? status,
      String? serviceName,
      DateTime? serviceDate,
      String? notes,
      double? cost,
      String? eventNote});
  Future<ChecklistResult> checklist(String productId);
  Future<Diagnosis> diagnose(
      String productId, ProblemType problem, String description);
  Future<Uint8List> servicePack(String productId,
      {String serviceRecordId = ''});

  Future<Dashboard> dashboard();
}

List<Map<String, dynamic>> _items(Map<String, dynamic> body) =>
    (body['items'] as List? ?? const [])
        .whereType<Map<String, dynamic>>()
        .toList();

/// REST implementation.
class ApiWarrantyRepository implements WarrantyRepository {
  ApiWarrantyRepository(this.api);

  final ApiClient api;

  AuthSession _session(Map<String, dynamic> j) => AuthSession(
      j['token'] as String, AppUser.fromJson(j['user'] as Map<String, dynamic>),
      refreshToken: j['refresh_token'] as String?);

  @override
  Future<AuthSession> login(String email, String password) async =>
      _session(await api
          .post('/auth/login', {'email': email.trim(), 'password': password}));

  @override
  Future<AuthSession> register(
          String email, String password, String name) async =>
      _session(
        await api.post('/auth/register',
            {'email': email.trim(), 'password': password, 'name': name.trim()}),
      );

  @override
  Future<AppUser> me() async => AppUser.fromJson(await api.get('/me'));

  @override
  Future<AppUser> updateProfile({required String name}) async =>
      AppUser.fromJson(await api.patch('/me', {'name': name.trim()}));

  @override
  Future<void> deleteAccount() => api.delete('/me');

  @override
  Future<void> logout(String? refreshToken) async {
    await api.post('/auth/logout', {'refresh_token': refreshToken ?? ''});
  }

  @override
  Future<FamilyInfo> family() async =>
      FamilyInfo.fromJson(await api.get('/family'));

  @override
  Future<FamilyInfo> createFamily(String name) async =>
      FamilyInfo.fromJson(await api.post('/family', {'name': name.trim()}));

  @override
  Future<String> createFamilyInvite() async =>
      '${(await api.post('/family/invites', null))['code']}';

  @override
  Future<FamilyInfo> joinFamily(String code) async => FamilyInfo.fromJson(
      await api.post('/family/join', {'code': code.trim()}));

  @override
  Future<void> removeFamilyMember(String userId) =>
      api.delete('/family/members/${Uri.encodeComponent(userId)}');

  @override
  Future<List<FamilyProduct>> familyProducts() async => [
        for (final j
            in ((await api.get('/family/products'))['items'] as List?) ??
                const [])
          FamilyProduct.fromJson(j as Map<String, dynamic>)
      ];

  @override
  Future<Uint8List> exportBackup() => api.download('/me/export');

  @override
  Future<ImportSummary> importBackup(Uint8List zip) async =>
      ImportSummary.fromJson(
          await api.postBytes('/me/import', zip, 'application/zip'));

  @override
  Future<void> registerDevice(String token, String platform) async {
    await api.post('/devices', {'token': token, 'platform': platform});
  }

  @override
  Future<void> unregisterDevice(String token) =>
      api.delete('/devices/${Uri.encodeComponent(token)}');

  @override
  Future<bool> resendVerification() async =>
      (await api.post('/auth/email/resend', null))['status'] !=
      'already_verified';

  @override
  Future<void> forgotPassword(String email) async {
    await api.post('/auth/password/forgot', {'email': email.trim()});
  }

  @override
  Future<void> resetPassword(String token, String newPassword) async {
    await api.post('/auth/password/reset',
        {'token': token.trim(), 'password': newPassword});
  }

  @override
  Future<void> changePassword(
      String currentPassword, String newPassword) async {
    await api.put('/me/password', {
      'current_password': currentPassword,
      'new_password': newPassword,
    });
  }

  @override
  Future<List<ProductCategory>> categories() async =>
      _items(await api.get('/categories'))
          .map(ProductCategory.fromJson)
          .toList();

  @override
  Future<List<Product>> products(
          {String category = '',
          String status = '',
          String query = '',
          String code = ''}) async =>
      _items(await api.get('/products', query: {
        'category': category,
        'status': status,
        'q': query,
        'code': code
      }))
          .map(Product.fromJson)
          .toList();

  @override
  Future<List<Product>> expiring({int days = 90}) async =>
      _items(await api.get('/products/expiring', query: {'days': '$days'}))
          .map(Product.fromJson)
          .toList();

  @override
  Future<Product> product(String id) async =>
      Product.fromJson(await api.get('/products/$id'));

  @override
  Future<Product> createProduct(ProductDraft draft) async =>
      Product.fromJson(await api.post('/products', draft.toJson()));

  @override
  Future<Product> updateProduct(String id, ProductDraft draft) async =>
      Product.fromJson(await api.put('/products/$id', draft.toJson()));

  @override
  Future<void> deleteProduct(String id) => api.delete('/products/$id');

  @override
  Future<ExtractionResult> extract(
      {Uint8List? image,
      String imageName = 'fis.jpg',
      String text = ''}) async {
    if (image == null) {
      return ExtractionResult.fromJson(
          await api.post('/extract', {'text': text}));
    }
    return ExtractionResult.fromJson(
      await api.multipart('/extract',
          fields: {'text': text}, files: {'image': (image, imageName)}),
    );
  }

  @override
  Future<List<ProductDocument>> documents(String productId,
          {String type = ''}) async =>
      _items(await api
              .get('/products/$productId/documents', query: {'type': type}))
          .map(ProductDocument.fromJson)
          .toList();

  @override
  Future<ProductDocument> uploadDocument(
          String productId, String type, Uint8List bytes, String filename,
          {String serviceRecordId = ''}) async =>
      ProductDocument.fromJson(
        await api.multipart(
          '/products/$productId/documents',
          fields: {
            'type': type,
            if (serviceRecordId.isNotEmpty) 'service_record_id': serviceRecordId
          },
          files: {'file': (bytes, filename)},
        ),
      );

  @override
  Future<Uint8List> documentBytes(String documentId) =>
      api.bytes('/documents/$documentId/file');

  @override
  Future<void> deleteDocument(String documentId) =>
      api.delete('/documents/$documentId');

  @override
  String documentUrl(String documentId) =>
      api.uri('/documents/$documentId/file').toString();

  @override
  Future<Map<String, String>> authHeaders() => api.authHeaders();

  @override
  Future<ServiceRecord> createServiceRecord(
          String productId, ProblemType problem, String description) async =>
      ServiceRecord.fromJson(
        await api.post('/products/$productId/service-records',
            {'problem': problem.api, 'description': description}),
      );

  @override
  Future<List<ServiceRecord>> serviceRecords({String productId = ''}) async {
    final path = productId.isEmpty
        ? '/service-records'
        : '/products/$productId/service-records';
    return _items(await api.get(path)).map(ServiceRecord.fromJson).toList();
  }

  @override
  Future<ServiceHistory> serviceHistory({ServiceStatus? status}) async {
    final j = await api.get('/service-records',
        query: {if (status != null) 'status': status.api});
    return ServiceHistory(
      _items(j).map(ServiceRecord.fromJson).toList(),
      ServiceSummary.fromJson(
          (j['summary'] as Map<String, dynamic>?) ?? const {}),
    );
  }

  @override
  Future<ServiceRecord> serviceRecord(String id) async =>
      ServiceRecord.fromJson(await api.get('/service-records/$id'));

  @override
  Future<void> deleteServiceRecord(String id) =>
      api.delete('/service-records/$id');

  @override
  Future<ServiceRecord> updateServiceRecord(String id,
          {ServiceStatus? status,
          String? serviceName,
          DateTime? serviceDate,
          String? notes,
          double? cost,
          String? eventNote}) async =>
      ServiceRecord.fromJson(await api.patch('/service-records/$id', {
        if (status != null) 'status': status.api,
        if (serviceName != null) 'service_name': serviceName,
        if (serviceDate != null) 'service_date': isoDate(serviceDate),
        if (notes != null) 'notes': notes,
        if (cost != null) 'cost': cost,
        if (eventNote != null && eventNote.isNotEmpty) 'event_note': eventNote,
      }));

  @override
  Future<ChecklistResult> checklist(String productId) async {
    final j = await api.get('/products/$productId/checklist');
    return ChecklistResult(
      j['under_warranty'] == true,
      _items(j).map(ChecklistItem.fromJson).toList(),
      j['days_left'] is num ? (j['days_left'] as num).toInt() : null,
    );
  }

  @override
  Future<Diagnosis> diagnose(
          String productId, ProblemType problem, String description) async =>
      Diagnosis.fromJson(await api.post('/products/$productId/diagnose',
          {'problem': problem.api, 'description': description},
          timeout: ApiClient.aiTimeout));

  @override
  Future<Uint8List> servicePack(String productId,
          {String serviceRecordId = ''}) =>
      api.bytes('/products/$productId/service-pack.pdf',
          query: {'service_record_id': serviceRecordId});

  @override
  Future<Dashboard> dashboard() async =>
      Dashboard.fromJson(await api.get('/dashboard'));
}
