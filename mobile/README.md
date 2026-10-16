# Warranty Wallet — Mobil (Flutter)

## Yapı
```
lib/
  main.dart, app.dart          → giriş noktası, tema, oturuma göre yönlendirme
  core/                        → api_client (HTTP + hata), warranty (tarih kuralları), formatters (TR biçimleri)
  models/                      → API modelleri (elle yazılmış JSON eşleme)
  data/repository.dart         → WarrantyRepository arayüzü + REST uygulaması
  services/services.dart       → TokenStore, OcrService (ML Kit), ReminderScheduler (yerel bildirim)
  state/providers.dart         → Riverpod provider'ları ve AuthController
  features/                    → auth, home, products, service, dashboard, settings ekranları
  widgets/common.dart          → ortak bileşenler (durum rozeti, boş/hata durumları)
test/                          → birim + widget testleri (FakeRepository ile, ağ yok)
```

## Platform klasörleri
`android/` ve `ios/` klasörleri repodadır (Flutter 3.47.7 ile üretildi). Eklenen ayarlar:
- **Android** `AndroidManifest.xml`: `INTERNET`, `CAMERA`, `POST_NOTIFICATIONS`, `RECEIVE_BOOT_COMPLETED`,
  `SCHEDULE_EXACT_ALARM` izinleri; geliştirmede HTTP backend için `android:usesCleartextTraffic="true"`.
  `build.gradle.kts`: `flutter_local_notifications` için core library desugaring.
- **iOS** `Info.plist`: `NSCameraUsageDescription`, `NSPhotoLibraryUsageDescription`.

## Çalıştırma
```bash
flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080   # Android emülatörü
flutter run --dart-define=API_BASE_URL=http://localhost:8080  # iOS simülatörü
```

## Test
```bash
flutter analyze
flutter test
```
