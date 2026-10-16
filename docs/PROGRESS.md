# İlerleme (PROGRESS)

Son güncelleme: 2026-10-16

## Durum özeti
- **Backend (Go):** V1 uç noktalarının tamamı yazıldı; `go vet` temiz, `go test ./...` yeşil (birim + handler/entegrasyon testleri).
- **PostgreSQL:** migration ve tüm SQL sorguları yerel PostgreSQL 16'da `PREPARE` ile doğrulandı; pgx sürücüsüyle store sözleşme testleri CI'da çalışır.
- **Mobil (Flutter):** Flutter 3.47.7 (Dart 3.13.5) ile `flutter analyze` 0 sorun, `flutter test` 19/19 geçti. `android/` ve `ios/` üretildi, izinler eklendi. Yerelde Android SDK indirilemediği için (dl.google.com engelli) APK derlemesi CI'daki `mobile-android` işine bırakıldı.
- **Gemini:** entegrasyon + mock testleri hazır. Bu oturumda `generativelanguage.googleapis.com` ağ politikası tarafından engellendiği için canlı çağrı başarısız oldu; yedek ayrıştırıcıya otomatik geçiş canlı doğrulandı.

## Tamamlananlar
- [x] Repo iskeleti, README, ARCHITECTURE, .gitignore, .env.example
- [x] Domain modeli, garanti bitiş tarihi hesabı (ay sonu/artık yıl), durum hesabı
- [x] Auth: kayıt/giriş, PBKDF2-SHA256 parola, HS256 JWT, hız sınırlama, hesap silme
- [x] Ürün CRUD + filtre/arama + "yakında bitiyor" listesi, sunucu tarafı bitiş hesabı
- [x] Belge yükleme/indirme/silme (içerik türü koklama, 10 MB sınırı, yerel depolama)
- [x] ReceiptExtractor: Gemini (structured output, retry, timeout, model yedeği) + Türkçe kural tabanlı ayrıştırıcı + Fallback
- [x] Bildirim planlama 90/30/7 gün (sunucu tablosu + işçi) ve cihazda yerel bildirim
- [x] Servis kaydı, arıza akışı, kontrol listesi, servis paketi PDF (bağımlılıksız PDF üretici)
- [x] Dashboard, Admin uç noktaları (kullanıcılar, kategoriler, çıkarım hata kayıtları, istatistik)
- [x] Docker (çok aşamalı), docker-compose (API + PostgreSQL), GitHub Actions CI
- [x] Flutter: giriş, ana ekran, ürün listesi, ürün ekleme (kamera → OCR → AI → form), detay, belge görüntüleme, arıza akışı + PDF, servis geçmişi, özet, ayarlar
- [x] Mobil birim + widget testleri (FakeRepository)

## Oturum günlüğü (2026-10-16, otonom oturum)
1. **Mobil doğrulama** — Flutter 3.47.7 kuruldu; `pub get` OK. Tek analiz uyarısı (kullanılmayan import) giderildi, `dart format` + `dart fix` uygulandı (yalnızca biçim ve `if` blokları). `flutter test`: 19 test, hepsi geçti.
   `flutter create --platforms=android,ios .` ile platform klasörleri üretildi; AndroidManifest'e INTERNET, CAMERA, POST_NOTIFICATIONS, RECEIVE_BOOT_COMPLETED, SCHEDULE_EXACT_ALARM ve `usesCleartextTraffic`, Info.plist'e kamera/fotoğraf açıklamaları eklendi; `flutter_local_notifications` için core library desugaring açıldı.
   `pubspec.lock` artık commit ediliyor (uygulama paketi için önerilen pratik). CI: Flutter sürümü sabitlendi, `dart format` artık zorunlu, `flutter build apk --debug` işi eklendi.
2. **Backend doğrulama** — `gofmt -l` boş, `go vet` temiz, `go test ./...` yeşil (`-race` ile de). pgx **v5.8.0** eklendi (`go.mod`/`go.sum` commit edildi). Karar: v5.9+ Go 1.25 istediği için Go 1.24 ile uyumlu son sürüm v5.8.0 seçildi.
   Yerel PostgreSQL 16.15 üzerinde `TEST_DATABASE_URL=... go test -tags pgx ./internal/store/postgres/` → tüm sözleşme testleri geçti (hata çıkmadı).
   Dockerfile artık `go.mod/go.sum` ile katmanlı `go mod download` yapıyor (derleme sırasında `go get` yok). CI: `go-version-file`, `go mod tidy` farkı kontrolü, `-tags pgx` ile build + vet.
   Not: bu ortamda Docker Hub 429 verdi ve konteyner içinden `apk` erişemedi; build aşaması (Go derleme) yerelde doğrulandı, tam imaj CI'daki `docker` işinde derlenir.

## Kalanlar / sonraki adımlar
- [ ] CI'da mobil analiz/test sonuçlarını kontrol edip olası hataları düzeltmek
- [ ] V2: barkod/QR ve seri numarası tarama, AI arıza asistanı, servis merkezi bulma, aile hesabı, bulut yedekleme
- [ ] Refresh token, e-posta doğrulama, şifre sıfırlama
- [ ] S3/MinIO depolama uygulaması, FCM push

## Alınan kararlar
- Monorepo: backend (Go), mobile (Flutter), docs.
- Go: yalnızca standart kütüphane (net/http, crypto/pbkdf2, elle HS256 JWT). PostgreSQL sürücüsü `-tags pgx` ile bağlanır.
- V1'de Redis yok; bildirimler cihazda yerel + sunucuda tablo/işçi.
- Gemini çağrısı yalnızca backend'de; anahtar yoksa/kota dolarsa kural tabanlı ayrıştırıcı.
- Varsayılan model `gemini-3.8-flash` (ücretsiz katmanda, görsel + structured output destekli); 404 alınırsa `gemini-3.5-flash-lite`, `gemini-2.5-flash` denenir.
- `android/`/`ios/` klasörleri artık repoda (2026-10-16): izinlerin herkes için aynı olması için.
- Flutter sürümü CI'da 3.47.7'ye sabitlendi.
