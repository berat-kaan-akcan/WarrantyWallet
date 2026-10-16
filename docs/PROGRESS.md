# İlerleme (PROGRESS)

Son güncelleme: 2026-10-16

## Durum özeti
- **Backend (Go):** V1 uç noktalarının tamamı yazıldı; `go vet` temiz, `go test ./...` yeşil (birim + handler/entegrasyon testleri).
- **PostgreSQL:** migration ve tüm SQL sorguları yerel PostgreSQL 16'da `PREPARE` ile doğrulandı; pgx sürücüsüyle store sözleşme testleri CI'da çalışır.
- **Mobil (Flutter):** Flutter 3.47.7 (Dart 3.13.5) ile `flutter analyze` 0 sorun, `flutter test` 19/19 geçti. `android/` ve `ios/` üretildi, izinler eklendi. Yerelde Android SDK indirilemediği için (dl.google.com engelli) APK derlemesi CI'daki `mobile-android` işine bırakıldı.
- **Gemini:** canlı doğrulandı (2026-10-16). Varsayılan model `gemini-3.5-flash`, yedek `gemini-2.5-flash`. Türkçe fiş metni ve fiş görseli doğru alanlarla çıkarıldı.

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
3. **Uçtan uca test** — Docker Hub 429 verdiği ve imaj içinden `apk` erişemediği için API `-tags pgx` ile doğrudan, yerel PostgreSQL 16'ya bağlı çalıştırıldı (migration açılışta uygulandı). curl ile denenenler, hepsi beklenen yanıtı verdi:
   register → login → ürün oluşturma (bitiş 2028-09-12, 697 gün) → belge yükleme (JPEG, 55 KB) → belge listesi → `/extract` (metin + görsel) → servis kaydı (`screen_lines`) → PATCH `in_service` → checklist → `service-pack.pdf` (200, `application/pdf`, 58 KB) → dashboard → bildirimler (90/30/7) → expiring → admin `stats`/`extraction-logs` (yetkisiz token → 401).
   **Canlı Gemini:** `gemini-3.8-flash` ve `gemini-3.7-flash` bu saatlerde sürekli `503 UNAVAILABLE (high demand)` döndü; ilk denemede metin istekleri kural tabanlı ayrıştırıcıya, görsel istek zaman aşımına düştü. `gemini-3.5-flash` ve `gemini-2.5-flash` çalıştı.
   Kararlar: (a) varsayılan model **`gemini-3.5-flash`** yapıldı (görsel ~5-12 sn, metin ~8 sn); (b) yedek zinciri artık 404'e ek olarak 503/429'da da sonraki modele geçiyor, model başına 1 yeniden deneme; `GEMINI_FALLBACK_MODELS` ile ayarlanabilir (varsayılan `gemini-2.5-flash`); (c) `/extract` yanıtına `model` alanı ve log satırı eklendi.
   Sonuçlar (gemini-3.5-flash): Teknosa metni → Sony WH-1000XM5, kulaklık, 03.10.2026, 11.999 TL, 24 ay, seri 5023417. MediaMarkt fiş görseli → ASUS VG27AQ, monitör, 12.09.2026, 12.499 TL, 24 ay, seri R9LMQS123456. Eksik alan yok.
   Ek düzeltme: kural tabanlı ayrıştırıcı `SERİ NO` (Türkçe İ) etiketini kaçırıyordu; düzeltildi ve test eklendi.
4. **CI** — `gh run list`: 1. ve 2. commit'lerin CI koşuları yeşil (backend, PostgreSQL entegrasyon, Docker imajı, mobil analiz/test, Android debug APK). Node 20 uyarıları için `checkout@v5`, `setup-go@v6`, `setup-java@v5`'e geçildi.
5. **V1 iyileştirmeleri (güvenlik)** — refresh token (opak, 256 bit, yalnızca SHA-256 özeti saklanır, tek kullanımlık rotasyon, 30 gün), access token 7 gün → **1 saat**; `POST /auth/refresh`, `POST /auth/logout` (tek token veya `all`), şifre sıfırlama iskeleti (`/auth/password/forgot` her zaman 202 → hesap ifşası yok; `/auth/password/reset` tek kullanımlık, 1 saat, yalnızca en yeni bağlantı geçerli; başarılı sıfırlamada tüm oturumlar kapanır), `PUT /me/password`, `PATCH /me`. E-posta gönderimi `mail.Sender` arayüzünün arkasında; V1'de yalnızca maskeli log (`LogSender`, token loglanmaz).
   Güvenlik düzeltmesi: hız sınırlayıcı `X-Forwarded-For`'a koşulsuz güveniyordu (başlığı değiştirerek sınır aşılabiliyordu); artık yalnızca `TRUST_PROXY=true` iken kullanılıyor. Migration `0002_auth_tokens.sql`.
   Mobil: refresh token güvenli depoda; 401 alınınca tek bir ortak yenileme çağrısı yapılıp istek tekrarlanıyor (eşzamanlı 401'ler aynı yenilemeyi paylaşır), yenileme başarısızsa oturum kapanıyor. Çıkışta sunucudaki refresh token iptal ediliyor. Yeni ekranlar: "Şifremi unuttum" (kod iste → kod + yeni şifre), Ayarlar → "Şifreyi değiştir" ve ad düzenleme. Ortak `PasswordField`/`ErrorText` (ekran okuyucu için `liveRegion`). Mobil testler: 25/25.
6. **Giriş doğrulama** — uzunluk kontrolleri bayt yerine karakter sayıyor (Türkçe harfler 2 bayt sayılıyordu); fiyat üst sınırı (1 milyar, `numeric(12,2)` taşmasını ve 500'ü önler), para birimi yalnızca 3 büyük harf, satın alma tarihi ≥ 1990-01-01, servis tarihi en fazla 1 yıl ileri; ürün listesinde geçersiz `status` filtresi ve 100 karakterden uzun arama 422 döner.

## Kalanlar / sonraki adımlar
- [ ] CI'da mobil analiz/test sonuçlarını kontrol edip olası hataları düzeltmek
- [ ] V2: barkod/QR ve seri numarası tarama, AI arıza asistanı, servis merkezi bulma, aile hesabı, bulut yedekleme
- [x] Refresh token, şifre sıfırlama iskeleti
- [ ] E-posta doğrulama ve gerçek SMTP gönderici
- [ ] S3/MinIO depolama uygulaması, FCM push

## Alınan kararlar
- Monorepo: backend (Go), mobile (Flutter), docs.
- Go: yalnızca standart kütüphane (net/http, crypto/pbkdf2, elle HS256 JWT). PostgreSQL sürücüsü `-tags pgx` ile bağlanır.
- V1'de Redis yok; bildirimler cihazda yerel + sunucuda tablo/işçi.
- Gemini çağrısı yalnızca backend'de; anahtar yoksa/kota dolarsa kural tabanlı ayrıştırıcı.
- Varsayılan model `gemini-3.5-flash` (2026-10-16 canlı testte çalışan, görsel + structured output destekli); meşgul/kota/404 durumunda `gemini-2.5-flash` denenir. `gemini-3.8-flash` test sırasında sürekli 503 verdiği için varsayılan olmaktan çıkarıldı (`GEMINI_MODEL` ile seçilebilir).
- `android/`/`ios/` klasörleri artık repoda (2026-10-16): izinlerin herkes için aynı olması için.
- Flutter sürümü CI'da 3.47.7'ye sabitlendi.
