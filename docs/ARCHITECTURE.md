# Warranty Wallet — Mimari

Bu doküman sistemin yapısını, veri modelini, REST API'yi, OCR/AI (Gemini) entegrasyonunu ve
bildirim stratejisini **gerekçeleriyle** açıklar. Ekip üyeleri yeni bir özellik eklemeden önce
bu dokümanı okumalıdır.

## 1. Genel bakış

```
┌──────────────────────┐   HTTPS/JSON (JWT)   ┌───────────────────────────────┐
│  Flutter mobil app   │ ───────────────────▶ │  Go REST API (net/http)        │
│  - Riverpod state    │                      │  handlers → services → repos   │
│  - on-device OCR     │ ◀─────────────────── │  ReceiptExtractor (Gemini/kural)│
│  - yerel bildirimler │                      │  Storage (yerel disk / S3)     │
└──────────────────────┘                      │  PDF üretici, bildirim planlayıcı│
                                              └───────┬───────────────┬───────┘
                                                      │               │
                                               PostgreSQL 16     Google Gemini API
                                                                 (generateContent)
```

### Neden bu teknolojiler?
| Karar | Gerekçe |
|---|---|
| **Monorepo** (`backend/`, `mobile/`, `docs/`) | Ders projesi; tek repo, tek CI, tek issue takibi. API değişikliği ve mobil değişikliği aynı commit'te olabilir. |
| **Go + standart `net/http`** (Go 1.22+ desen yönlendirme) | Fikir sahibinin önerisi. Ek framework yok → öğrenme eğrisi düşük, bağımlılık az. Tek statik binary, küçük Docker imajı. |
| **PostgreSQL + pgx** | İlişkisel veri (kullanıcı → ürün → belge/servis kaydı). `pgx` en yaygın ve hızlı sürücü. |
| **Gömülü SQL migration'lar** (`embed`) | Ek araç gerektirmez; API açılışta `schema_migrations` tablosuna bakarak sırayla uygular. |
| **JWT (HS256) + bcrypt** | Durumsuz kimlik doğrulama, mobil için basit. Access token 7 gün (MVP); V2'de refresh token. |
| **Storage arayüzü** — V1'de yerel disk (Docker volume) | MinIO/S3 opsiyoneldir; arayüz sayesinde `S3Storage` sonradan eklenebilir. MVP'de ek servis çalıştırmamak için. |
| **Redis şimdilik YOK** | V1 bildirimleri cihazda yerel olarak planlanır (internet gerekmez, güvenilir). Sunucu tarafında `notifications` tablosu + Go içinde periyodik işçi (`FOR UPDATE SKIP LOCKED`) yeterli. Redis, V2'de FCM push + kuyruk ihtiyacı doğunca eklenecek. |
| **Flutter + Riverpod** | Tek kod tabanı ile Android/iOS. Riverpod test edilebilir, BuildContext'e bağlı olmayan durum yönetimi sunar. |
| **Gemini (backend'de)** | Anahtar mobil uygulamaya gömülmez. Çok modlu model fiş görüntüsünü doğrudan okuyabilir (OCR + çıkarım tek adım). |
| **PDF üretimi backend'de** (`go-pdf/fpdf`) | Belgeler zaten sunucuda; tek uç noktadan "servis paketi" PDF'i indirilir. Test edilebilir. |

## 2. Backend katmanları

```
backend/
  cmd/api/main.go          → yapılandırma, bağımlılıkların kurulumu (composition root)
  internal/config          → ortam değişkenlerinden Config (gizli bilgiler SADECE env'den)
  internal/domain          → saf modeller + iş kuralları (garanti hesabı, durum) — I/O yok
  internal/store           → Repository arayüzleri; postgres/ ve memory/ uygulamaları
  internal/auth            → parola hash, JWT üretme/doğrulama, middleware
  internal/extract         → ReceiptExtractor arayüzü: GeminiExtractor, RuleExtractor, Fallback
  internal/storage         → dosya saklama arayüzü (LocalStorage)
  internal/pdf             → servis belge paketi PDF üretimi
  internal/notify          → bildirim planlama (90/30/7 gün) + işçi
  internal/httpapi         → HTTP handler'lar, routing, JSON yardımcıları, hata formatı
  migrations/              → 0001_init.sql, ...
```

Kurallar:
- `domain` hiçbir şeye bağımlı değildir; diğer paketler `domain`'e bağımlıdır.
- Handler'lar repository **arayüzlerine** bağımlıdır → testlerde `memory` uygulaması kullanılır
  (veritabanı gerekmeden handler testleri). Postgres testleri `DATABASE_URL` tanımlıysa çalışır (CI'da servis konteyneri ile).
- Tüm hata yanıtları: `{"error": {"code": "validation_failed", "message": "...", "fields": {...}}}`

## 3. Veri modeli

```sql
users(id uuid pk, email citext unique, password_hash, name, role ('user'|'admin'), created_at)
categories(id serial pk, slug unique, name_tr)                    -- admin yönetir
products(id uuid pk, user_id fk, name, brand, model, category, seller,
         purchase_date date, purchase_price numeric(12,2), currency char(3),
         warranty_months int, warranty_end date, serial_number, notes,
         created_at, updated_at)
documents(id uuid pk, user_id fk, product_id fk null, type ('invoice'|'warranty'|'service'|'photo'),
          file_key, content_type, size_bytes, original_name, created_at)
service_records(id uuid pk, user_id fk, product_id fk, problem ('wont_turn_on'|'screen_lines'|
          'physical_damage'|'other'), description, service_name, service_date date,
          status ('open'|'in_service'|'resolved'|'cancelled'), notes, created_at, updated_at)
notifications(id uuid pk, user_id fk, product_id fk, type ('warranty_90'|'warranty_30'|'warranty_7'),
          scheduled_at timestamptz, sent bool, sent_at, unique(product_id,type))
extraction_logs(id uuid pk, user_id fk, provider ('gemini'|'rules'), success bool, error text,
          raw_text text, created_at)                               -- admin "OCR hataları" görünümü
```

Kararlar:
- **`warranty_end` saklanır** (türetilmiş olsa da) → "yakında bitiyor" sorgusu indeksle hızlı olur.
  Her yazmada `domain.WarrantyEnd(purchase_date, months)` ile yeniden hesaplanır, istemciden alınmaz.
- **Ay sonu kuralı:** 31.01 + 1 ay → 28/29.02 (ayın son günü), Go'nun `AddDate` taşmasını engeller.
- Fiyat `numeric(12,2)`; API'de `price` string değil sayı (float64, 2 hane) + `currency` (ISO 4217, varsayılan TRY).
- Durum (`active`/`expiring_soon`/`expired`) saklanmaz, okuma anında hesaplanır (`expiring_soon` = ≤30 gün).

## 4. REST API (v1)

Tümü `/api/v1` altında, JSON. Auth gerektirenler `Authorization: Bearer <jwt>`.

| Metot | Yol | Açıklama |
|---|---|---|
| GET | `/healthz` | Sağlık kontrolü |
| POST | `/auth/register` | `{email,password,name}` → `{token,user}` |
| POST | `/auth/login` | `{email,password}` → `{token,user}` |
| GET | `/me` | Oturumdaki kullanıcı |
| GET | `/products?category=&status=&q=` | Ürün listesi |
| POST | `/products` | Ürün oluştur (bitiş tarihi sunucuda hesaplanır) |
| GET/PUT/DELETE | `/products/{id}` | Detay / güncelle / sil |
| GET | `/products/expiring?days=90` | Garantisi yakında bitenler (kalan gün ile) |
| POST | `/extract` | multipart `image` ve/veya `text` → `{fields, provider, raw_text}` |
| POST | `/products/{id}/documents` | multipart `file`, `type` |
| GET | `/products/{id}/documents` | Belge listesi |
| GET | `/documents/{id}/file` | Dosyayı indir (yalnız sahibi) |
| DELETE | `/documents/{id}` | Belge sil |
| GET/POST | `/products/{id}/service-records` | Servis kayıtları |
| GET | `/service-records` | Tüm servis geçmişi |
| PATCH | `/service-records/{id}` | Durum/not güncelle |
| GET | `/products/{id}/service-pack.pdf` | Servise götürülecek belgeler PDF paketi |
| GET | `/products/{id}/checklist` | Servis belgesi kontrol listesi |
| GET | `/dashboard` | Toplam ürün, aktif, garanti dışı, toplam harcama, kategori dağılımı |
| GET | `/notifications` | Planlanan bildirimler |
| GET | `/categories` | Kategori listesi |

Ayrıntılı istek/yanıt örnekleri: [API.md](API.md).

## 5. OCR + AI (Gemini) entegrasyonu

```go
type ReceiptExtractor interface {
    Extract(ctx context.Context, in Input) (Result, error) // Input{Text, Image, MIME}
}
```
- **GeminiExtractor**: `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`,
  başlık `x-goog-api-key` (anahtar asla URL'de/logda değil). `generationConfig.responseMimeType =
  application/json` + `responseSchema` (structured output) ile şu alanlar istenir: `product_name, brand,
  model, category, seller, purchase_date (YYYY-MM-DD), price, currency, warranty_months, serial_number`.
  Görsel varsa `inline_data` olarak gönderilir (OCR'ı model yapar), OCR metni de varsa ek bağlam olur.
  Zaman aşımı (20 sn), 429/5xx için üstel geri çekilmeli en fazla 2 yeniden deneme.
- **RuleExtractor**: Türkçe fiş metni için regex/sezgisel ayrıştırıcı (tarih biçimleri `12.09.2026`,
  `TOPLAM 12.499,00 TL`, bilinen satıcı/marka sözlüğü, "24 AY GARANTİ" vb.).
- **FallbackExtractor**: önce Gemini; anahtar yoksa / 401-403 / kota (429) / zaman aşımı / geçersiz JSON
  durumunda kural tabanlıya düşer. Sonuçta `provider` alanı hangi yöntemin kullanıldığını söyler.
- Mobil uygulama cihazda OCR (Google ML Kit) ile ham metni de çıkarıp gönderir → Gemini yokken bile
  kural tabanlı ayrıştırıcının metni olur. Tüm alanlar kullanıcıya düzenlenebilir form olarak sunulur.
- Garanti süresi çıkarılamazsa form açılır menüde varsayılan **24 ay** (TR tüketici mevzuatındaki asgari süre) önerir.

## 6. Bildirim stratejisi

- **V1 (birincil): cihazda yerel bildirim** — `flutter_local_notifications` ile her ürün için bitişe
  90/30/7 gün kala saat 10:00'da bildirim planlanır. Ürün eklendiğinde/güncellendiğinde/silindiğinde
  yeniden planlanır; uygulama açılışında tüm liste senkronize edilir. Sunucuya ve internete bağımlı değildir.
- **Sunucu tarafı:** ürün yazıldığında `notifications` tablosuna aynı 3 kayıt (geçmişte kalanlar
  hariç) upsert edilir. Periyodik işçi zamanı gelenleri `sent=true` yapar; V2'de FCM push buraya bağlanır.
- Mesaj örneği: "ASUS monitörünüzün garantisinin bitmesine 30 gün kaldı."

## 7. Güvenlik
- Gizli bilgiler (`GEMINI_API_KEY`, `JWT_SECRET`, `DATABASE_URL`) yalnızca ortam değişkenlerinden okunur.
  Repoda sadece boş değerli `.env.example` bulunur.
- Her kaynak sorgusu `user_id` ile filtrelenir (başka kullanıcının ürününe erişim → 404).
- Yükleme sınırı 10 MB; izinli türler: JPEG, PNG, WEBP, HEIC, PDF.
- Parola en az 8 karakter, bcrypt.
- Basit IP başına hız sınırlama `/auth/*` ve `/extract` için.

## 8. Test stratejisi
- `domain`: tablo güdümlü birim testleri (garanti hesabı, ay sonu, durum).
- `extract`: Gemini için `httptest.Server` ile mock (başarı, 429→yedek, geçersiz JSON), kural tabanlı için örnek fişler.
- `httpapi`: bellek içi repository ile uçtan uca handler testleri (auth, ürün CRUD, yetki izolasyonu).
- `store/postgres`: `DATABASE_URL` varsa entegrasyon testleri (CI'da PostgreSQL servisi).
- Mobil: model/garanti hesabı birim testleri + widget testleri (sahte API istemcisi).

## 9. Roller
- **User**: kendi ürünleri.
- **Admin**: `/api/v1/admin/*` — kullanıcılar, kategoriler, çıkarım (OCR) hata kayıtları.
- **Business** (ileride): servis sağlayıcı; servis kayıtlarını güncelleyebilir.
