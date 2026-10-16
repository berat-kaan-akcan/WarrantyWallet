import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

/// Backend base URL, configurable at build time:
/// `flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080`
const String apiBaseUrl = String.fromEnvironment('API_BASE_URL',
    defaultValue: 'http://10.0.2.2:8080');

/// Error returned by the API (or a network failure with [status] 0).
class ApiException implements Exception {
  const ApiException(this.status, this.code, this.message,
      [this.fields = const {}]);

  final int status;
  final String code;
  final String message;
  final Map<String, String> fields;

  bool get isUnauthorized => status == 401;
  bool get isNetwork => status == 0;

  /// User-facing Turkish message.
  String get userMessage {
    if (isNetwork) {
      return 'Sunucuya ulaşılamıyor. İnternet bağlantınızı kontrol edin.';
    }
    switch (code) {
      case 'invalid_credentials':
        return 'E-posta veya şifre hatalı.';
      case 'email_taken':
        return 'Bu e-posta ile kayıtlı bir hesap zaten var.';
      case 'rate_limited':
        return 'Çok fazla deneme yapıldı. Lütfen biraz bekleyin.';
      case 'too_large':
        return 'Dosya çok büyük (en fazla 10 MB).';
      case 'unsupported_type':
        return 'Desteklenmeyen dosya türü. JPEG, PNG, HEIC veya PDF yükleyin.';
      case 'validation_failed':
        return 'Lütfen işaretli alanları kontrol edin.';
      case 'not_found':
        return 'Kayıt bulunamadı.';
      case 'unauthorized':
      case 'invalid_refresh_token':
        return 'Oturumunuzun süresi doldu. Lütfen tekrar giriş yapın.';
      case 'invalid_reset_token':
        return 'Sıfırlama kodu geçersiz veya süresi dolmuş.';
      case 'extraction_failed':
        return 'Fiş okunamadı. Bilgileri elle girebilirsiniz.';
      case 'timeout':
        return 'İstek zaman aşımına uğradı. Lütfen tekrar deneyin.';
    }
    return status >= 500
        ? 'Sunucu hatası. Lütfen daha sonra tekrar deneyin.'
        : message;
  }

  @override
  String toString() => 'ApiException($status, $code, $message)';
}

/// Thin JSON-over-HTTP client that attaches the bearer token.
class ApiClient {
  ApiClient(
      {required this.baseUrl,
      required this.tokenProvider,
      http.Client? client,
      this.onUnauthorized,
      this.refreshSession})
      : _client = client ?? http.Client();

  final String baseUrl;
  final Future<String?> Function() tokenProvider;
  final void Function()? onUnauthorized;

  /// Called once when a request gets 401; returns true if a new access token was stored,
  /// in which case the request is retried.
  final Future<bool> Function()? refreshSession;
  final http.Client _client;

  static const _timeout = Duration(seconds: 30);
  static const _uploadTimeout = Duration(seconds: 90);

  Uri uri(String path, [Map<String, String>? query]) {
    final q = query == null
        ? null
        : (Map.of(query)..removeWhere((_, v) => v.isEmpty));
    return Uri.parse('$baseUrl/api/v1$path')
        .replace(queryParameters: q == null || q.isEmpty ? null : q);
  }

  Future<Map<String, String>> _headers({bool json = true}) async {
    final token = await tokenProvider();
    return {
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      if (token != null && token.isNotEmpty) 'Authorization': 'Bearer $token',
    };
  }

  /// Headers for authenticated image loading (Image.network).
  Future<Map<String, String>> authHeaders() => _headers(json: false);

  Future<Map<String, dynamic>> get(String path,
          {Map<String, String>? query}) async =>
      _decode(await _send(() async =>
          _client.get(uri(path, query), headers: await _headers())));

  Future<Map<String, dynamic>> post(String path, Object? body,
          {Duration timeout = _timeout}) async =>
      _decode(
        await _send(
            () async => _client.post(uri(path),
                headers: await _headers(), body: jsonEncode(body)),
            timeout: timeout),
      );

  /// Timeout for AI endpoints (model fallbacks can take a while).
  static const aiTimeout = Duration(seconds: 60);

  Future<Map<String, dynamic>> put(String path, Object? body) async => _decode(
        await _send(() async => _client.put(uri(path),
            headers: await _headers(), body: jsonEncode(body))),
      );

  Future<Map<String, dynamic>> patch(String path, Object? body) async =>
      _decode(
        await _send(() async => _client.patch(uri(path),
            headers: await _headers(), body: jsonEncode(body))),
      );

  Future<void> delete(String path) async {
    await _send(
        () async => _client.delete(uri(path), headers: await _headers()));
  }

  /// Downloads raw bytes (documents, PDFs).
  Future<Uint8List> bytes(String path, {Map<String, String>? query}) async {
    final res = await _send(() async =>
        _client.get(uri(path, query), headers: await _headers(json: false)));
    return res.bodyBytes;
  }

  /// Multipart upload. [files] maps field name → (bytes, filename).
  Future<Map<String, dynamic>> multipart(
    String path, {
    Map<String, String> fields = const {},
    Map<String, (Uint8List, String)> files = const {},
  }) async {
    final res = await _send(() async {
      final req = http.MultipartRequest('POST', uri(path))
        ..headers.addAll(await _headers(json: false))
        ..fields.addAll(fields);
      files.forEach((field, file) {
        req.files.add(
            http.MultipartFile.fromBytes(field, file.$1, filename: file.$2));
      });
      final streamed = await _client.send(req).timeout(_uploadTimeout);
      return http.Response.fromStream(streamed);
    }, timeout: _uploadTimeout);
    return _decode(res);
  }

  Future<http.Response> _send(Future<http.Response> Function() fn,
      {Duration timeout = _timeout}) async {
    try {
      return await _sendOnce(fn, timeout);
    } on ApiException catch (e) {
      final refresh = refreshSession;
      if (!e.isUnauthorized || refresh == null || !await refresh()) {
        if (e.isUnauthorized) onUnauthorized?.call();
        rethrow;
      }
    }
    // Retry with the fresh access token (headers are rebuilt inside fn).
    try {
      return await _sendOnce(fn, timeout);
    } on ApiException catch (e) {
      if (e.isUnauthorized) onUnauthorized?.call();
      rethrow;
    }
  }

  Future<http.Response> _sendOnce(
      Future<http.Response> Function() fn, Duration timeout) async {
    http.Response res;
    try {
      res = await fn().timeout(timeout);
    } on TimeoutException {
      throw const ApiException(0, 'timeout', 'request timed out');
    } on http.ClientException catch (e) {
      throw ApiException(0, 'network', e.message);
    } on Exception catch (e) {
      throw ApiException(0, 'network', e.toString());
    }
    if (res.statusCode >= 200 && res.statusCode < 300) return res;
    var code = 'http_${res.statusCode}';
    var message = 'HTTP ${res.statusCode}';
    final fields = <String, String>{};
    try {
      final body = jsonDecode(utf8.decode(res.bodyBytes));
      if (body is Map<String, dynamic> &&
          body['error'] is Map<String, dynamic>) {
        final err = body['error'] as Map<String, dynamic>;
        code = (err['code'] as String?) ?? code;
        message = (err['message'] as String?) ?? message;
        final f = err['fields'];
        if (f is Map) f.forEach((k, v) => fields[k.toString()] = v.toString());
      }
    } on FormatException {
      // non-JSON error body
    }
    throw ApiException(res.statusCode, code, message, fields);
  }

  Map<String, dynamic> _decode(http.Response res) {
    if (res.statusCode == 204 || res.bodyBytes.isEmpty) return const {};
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    return body is Map<String, dynamic> ? body : {'items': body};
  }
}
