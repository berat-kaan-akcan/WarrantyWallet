import 'dart:io';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:google_mlkit_text_recognition/google_mlkit_text_recognition.dart';
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';
import 'package:timezone/data/latest.dart' as tzdata;
import 'package:timezone/timezone.dart' as tz;

import '../core/formatters.dart';
import '../core/warranty.dart';
import '../models/models.dart';

/// Persists the auth token.
abstract class TokenStore {
  /// Short-lived access token (JWT).
  Future<String?> read();

  /// Long-lived, single-use refresh token.
  Future<String?> readRefresh();
  Future<void> write(String token, {String? refreshToken});
  Future<void> clear();
}

/// Keychain/Keystore backed token storage.
class SecureTokenStore implements TokenStore {
  SecureTokenStore([FlutterSecureStorage? storage])
      : _storage = storage ?? const FlutterSecureStorage();

  static const _key = 'auth_token';
  static const _refreshKey = 'refresh_token';
  final FlutterSecureStorage _storage;
  String? _cache;
  String? _refreshCache;

  @override
  Future<String?> read() async => _cache ??= await _storage.read(key: _key);

  @override
  Future<String?> readRefresh() async =>
      _refreshCache ??= await _storage.read(key: _refreshKey);

  @override
  Future<void> write(String token, {String? refreshToken}) async {
    _cache = token;
    await _storage.write(key: _key, value: token);
    if (refreshToken != null) {
      _refreshCache = refreshToken;
      await _storage.write(key: _refreshKey, value: refreshToken);
    }
  }

  @override
  Future<void> clear() async {
    _cache = null;
    _refreshCache = null;
    await _storage.delete(key: _key);
    await _storage.delete(key: _refreshKey);
  }
}

/// In-memory token store (tests).
class MemoryTokenStore implements TokenStore {
  String? token;
  String? refreshToken;

  @override
  Future<String?> read() async => token;

  @override
  Future<String?> readRefresh() async => refreshToken;

  @override
  Future<void> write(String t, {String? refreshToken}) async {
    token = t;
    if (refreshToken != null) this.refreshToken = refreshToken;
  }

  @override
  Future<void> clear() async {
    token = null;
    refreshToken = null;
  }
}

/// On-device OCR: photo → raw text. The text is sent with the image so the backend's
/// rule-based parser still works when Gemini is unavailable.
abstract class OcrService {
  Future<String> recognize(String imagePath);
}

/// Google ML Kit (Latin script, offline, Android/iOS).
class MlKitOcrService implements OcrService {
  @override
  Future<String> recognize(String imagePath) async {
    final recognizer = TextRecognizer(script: TextRecognitionScript.latin);
    try {
      final result =
          await recognizer.processImage(InputImage.fromFilePath(imagePath));
      return result.text;
    } on Exception catch (e) {
      debugPrint('OCR failed: $e');
      return '';
    } finally {
      await recognizer.close();
    }
  }
}

/// No-op OCR (tests, unsupported platforms).
class NoopOcrService implements OcrService {
  @override
  Future<String> recognize(String imagePath) async => '';
}

/// A reminder to schedule on the device.
class PlannedReminder {
  const PlannedReminder(this.id, this.at, this.title, this.body);

  final int id;
  final DateTime at;
  final String title;
  final String body;
}

const _possessive = {
  'monitor': 'monitörünüzün',
  'laptop': 'laptopunuzun',
  'phone': 'telefonunuzun',
  'headphones': 'kulaklığınızın',
  'tablet': 'tabletinizin',
  'tv': 'televizyonunuzun',
  'appliance': 'cihazınızın',
  'small_appliance': 'cihazınızın',
  'camera': 'kameranızın',
  'console': 'konsolunuzun',
  'wearable': 'saatinizin',
};

/// "ASUS monitörünüzün garantisinin bitmesine 30 gün kaldı." (same text as the backend).
String reminderMessage(Product p, int days) {
  final form = _possessive[p.category];
  final subject = form != null && p.brand.isNotEmpty
      ? '${p.brand} $form'
      : '${p.name} ürününüzün';
  return '$subject garantisinin bitmesine $days gün kaldı.';
}

/// Stable notification id per product+offset (product ids are UUIDs).
/// Uses FNV-1a so ids are identical across app launches (needed to cancel old reminders).
int reminderId(String productId, int days) {
  var h = 0x811c9dc5;
  for (final c in productId.codeUnits) {
    h = ((h ^ c) * 0x01000193) & 0x7fffffff;
  }
  return (h & 0x0fffffff) * 4 + reminderOffsets.indexOf(days);
}

/// Plans 90/30/7-day reminders at 10:00 local time; past ones are skipped.
List<PlannedReminder> planReminders(Product p, DateTime now) {
  final end = p.warrantyEnd;
  if (end == null) return const [];
  final out = <PlannedReminder>[];
  for (final days in reminderOffsets) {
    final day = end.subtract(Duration(days: days));
    final at = DateTime(day.year, day.month, day.day, 10);
    if (!at.isAfter(now)) continue;
    out.add(PlannedReminder(reminderId(p.id, days), at, 'Garanti hatırlatıcı',
        reminderMessage(p, days)));
  }
  return out;
}

/// Schedules warranty reminders on the device.
abstract class ReminderScheduler {
  Future<void> init();
  Future<void> syncProduct(Product p);
  Future<void> cancelProduct(String productId);
  Future<void> syncAll(List<Product> products);
}

/// flutter_local_notifications implementation.
class LocalReminderScheduler implements ReminderScheduler {
  final _plugin = FlutterLocalNotificationsPlugin();
  bool _ready = false;

  static const _details = NotificationDetails(
    android: AndroidNotificationDetails(
      'warranty_reminders',
      'Garanti hatırlatıcıları',
      channelDescription: 'Garanti bitiminden 90, 30 ve 7 gün önce bildirim',
      importance: Importance.high,
      priority: Priority.high,
    ),
    iOS: DarwinNotificationDetails(),
  );

  @override
  Future<void> init() async {
    if (_ready || kIsWeb) return;
    tzdata.initializeTimeZones();
    tz.setLocalLocation(tz.getLocation('Europe/Istanbul'));
    const settings = InitializationSettings(
      android: AndroidInitializationSettings('@mipmap/ic_launcher'),
      iOS: DarwinInitializationSettings(),
    );
    await _plugin.initialize(settings);
    await _plugin
        .resolvePlatformSpecificImplementation<
            AndroidFlutterLocalNotificationsPlugin>()
        ?.requestNotificationsPermission();
    _ready = true;
  }

  @override
  Future<void> syncProduct(Product p) async {
    if (!_ready) return;
    await cancelProduct(p.id);
    for (final r in planReminders(p, DateTime.now())) {
      await _plugin.zonedSchedule(
        r.id,
        r.title,
        r.body,
        tz.TZDateTime.from(r.at, tz.local),
        _details,
        androidScheduleMode: AndroidScheduleMode.inexactAllowWhileIdle,
        uiLocalNotificationDateInterpretation:
            UILocalNotificationDateInterpretation.absoluteTime,
        payload: p.id,
      );
    }
  }

  @override
  Future<void> cancelProduct(String productId) async {
    if (!_ready) return;
    for (final days in reminderOffsets) {
      await _plugin.cancel(reminderId(productId, days));
    }
  }

  @override
  Future<void> syncAll(List<Product> products) async {
    if (!_ready) return;
    await _plugin.cancelAll();
    for (final p in products) {
      if (p.isCovered) await syncProduct(p);
    }
  }
}

/// Records calls instead of scheduling (tests).
class FakeReminderScheduler implements ReminderScheduler {
  final synced = <String>[];
  final cancelled = <String>[];

  @override
  Future<void> init() async {}

  @override
  Future<void> syncProduct(Product p) async => synced.add(p.id);

  @override
  Future<void> cancelProduct(String productId) async =>
      cancelled.add(productId);

  @override
  Future<void> syncAll(List<Product> products) async =>
      synced.addAll(products.map((p) => p.id));
}

/// Human description of the next reminder for the detail screen.
String nextReminderText(Product p, DateTime now) {
  final planned = planReminders(p, now);
  if (planned.isEmpty) return 'Planlanmış hatırlatıcı yok';
  return 'Sonraki hatırlatma: ${formatDate(planned.first.at)}';
}

/// Remote push (FCM) registration. The server sends warranty reminders to the token this
/// returns, in addition to the local notifications scheduled on the device.
///
/// The app ships with [NoopPushService] until a Firebase project is configured: add
/// `firebase_core` + `firebase_messaging`, put `google-services.json` /
/// `GoogleService-Info.plist` in place, and override [pushServiceProvider] with an
/// implementation that returns `FirebaseMessaging.instance.getToken()`.
abstract class PushService {
  /// The device's push token, or null when push is unavailable or not permitted.
  Future<String?> token();

  /// `android` or `ios`.
  String get platform;
}

class NoopPushService implements PushService {
  @override
  Future<String?> token() async => null;

  @override
  String get platform => 'android';
}

/// Saving and picking backup archives. The share sheet lets the user keep the backup in
/// Google Drive, iCloud Drive, e-mail, etc.
abstract class BackupFiles {
  Future<void> share(Uint8List bytes, String filename);

  /// Lets the user choose a backup `.zip`; null when cancelled.
  Future<Uint8List?> pickZip();
}

class DeviceBackupFiles implements BackupFiles {
  @override
  Future<void> share(Uint8List bytes, String filename) async {
    final dir = await getTemporaryDirectory();
    final file = File('${dir.path}/$filename');
    await file.writeAsBytes(bytes, flush: true);
    await Share.shareXFiles([XFile(file.path, mimeType: 'application/zip')],
        subject: filename);
  }

  @override
  Future<Uint8List?> pickZip() async {
    final res = await FilePicker.pickFiles(
        type: FileType.custom, allowedExtensions: ['zip'], withData: true);
    final file = res?.files.singleOrNull;
    if (file == null) return null;
    if (file.bytes != null) return file.bytes;
    final path = file.path;
    return path == null ? null : File(path).readAsBytes();
  }
}
