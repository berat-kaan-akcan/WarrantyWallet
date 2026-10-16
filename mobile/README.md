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
`android/` ve `ios/` klasörleri repoda tutulmuyor (Flutter sürümüne göre üretilir). İlk kurulumda:
```bash
cd mobile
flutter create --org com.warrantywallet --project-name warranty_wallet --platforms=android,ios .
flutter pub get
```
Sonra şunları ekleyin:
- **Android** `android/app/src/main/AndroidManifest.xml`: `CAMERA`, `POST_NOTIFICATIONS`,
  `RECEIVE_BOOT_COMPLETED` izinleri; geliştirmede HTTP backend için `android:usesCleartextTraffic="true"`.
  `minSdkVersion` en az 21 (ML Kit).
- **iOS** `ios/Runner/Info.plist`: `NSCameraUsageDescription`, `NSPhotoLibraryUsageDescription`.

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
