# Warranty Wallet — REST API (v1)

Tüm uç noktalar `/api/v1` altındadır, gövde ve yanıtlar JSON'dur (belge yükleme ve `/extract` hariç: `multipart/form-data`).
Örnekler 2026-10-16 tarihinde yerel PostgreSQL'e bağlı API'ye `curl` ile yapılan gerçek çağrılardan alınmıştır (kısaltıldı).

```bash
B=http://localhost:8080/api/v1
H="Authorization: Bearer $TOKEN"
```

## Genel kurallar

| Konu | Kural |
|---|---|
| Kimlik doğrulama | `Authorization: Bearer <access token>`. Access token 1 saat geçerlidir (`TOKEN_TTL`). Süresi dolunca `POST /auth/refresh` ile yenilenir. |
| Hata biçimi | `{"error": {"code": "validation_failed", "message": "...", "fields": {"alan": "neden"}}}` |
| Bilinmeyen alan | JSON gövdesinde tanımsız alan → `400 bad_request` (istemci yazım hatalarını yakalamak için). |
| Sahiplik | Başka kullanıcının kaynağı her zaman `404 not_found` döner (varlığı sızdırılmaz). |
| Tarih | `YYYY-MM-DD` (gün bazında, Türkiye saati). Zaman damgaları RFC 3339 UTC. |
| Hız sınırı | `/auth/*`: IP başına dakikada 10, `/extract` ve `/products/{id}/diagnose`: dakikada 20 → `429 rate_limited` + `Retry-After`. `X-Forwarded-For` yalnızca `TRUST_PROXY=true` ise dikkate alınır. |
| Yükleme | En fazla 10 MB; JPEG, PNG, WEBP, HEIC, PDF (içerik koklanır, uzantıya güvenilmez). |

### Hata kodları

| HTTP | `code` | Ne zaman |
|---|---|---|
| 400 | `bad_request` | Bozuk JSON / multipart, bilinmeyen alan |
| 400 | `invalid_reset_token` | Şifre sıfırlama kodu geçersiz, kullanılmış veya süresi dolmuş |
| 401 | `unauthorized` | Token yok, geçersiz veya süresi dolmuş |
| 401 | `invalid_credentials` | E-posta veya şifre hatalı (ikisi için aynı yanıt) |
| 401 | `invalid_refresh_token` | Refresh token geçersiz / kullanılmış / süresi dolmuş |
| 403 | `forbidden` | Admin rolü gerekiyor |
| 404 | `not_found` | Kaynak yok veya size ait değil |
| 409 | `email_taken` | E-posta zaten kayıtlı |
| 413 | `too_large` | Dosya 10 MB'tan büyük |
| 415 | `unsupported_type` | Desteklenmeyen dosya türü |
| 422 | `validation_failed` | Alan doğrulaması; `fields` hangi alanın neden reddedildiğini söyler |
| 429 | `rate_limited` | Hız sınırı |
| 502 | `extraction_failed` | Fiş hiçbir yöntemle okunamadı |
| 500 | `internal` | Sunucu hatası (ayrıntı yalnızca sunucu logunda) |

---

## Sağlık

### `GET /healthz`
```json
{"status":"ok","version":"dev","extractor":"gemini+rules"}
```
Veritabanına ulaşılamıyorsa `503` ve `"status":"degraded"`. `extractor`: `gemini+rules` (anahtar var) veya `rules`.

---

## Kimlik doğrulama ve hesap

### `POST /auth/register`
```json
{"email":"berat@example.com","password":"Sifre1234!","name":"Berat"}
```
`201`:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2026-10-16T21:02:18Z",
  "refresh_token": "kq3V1m...43 karakter",
  "refresh_expires_at": "2026-11-15T20:02:18Z",
  "user": {"id":"d72a0af5-...","email":"berat@example.com","name":"Berat","role":"user","email_verified_at":null,"created_at":"2026-10-16T20:02:18Z"}
}
```
Kayıttan sonra e-posta adresine 24 saat geçerli bir doğrulama bağlantısı gönderilir (aşağıya bakın).
Doğrulama: e-posta geçerli ve ≤254 karakter, şifre 8–128 karakter, ad ≤100 karakter. `ADMIN_EMAILS` içindeki e-postalar `admin` rolü alır.
Hatalar: `409 email_taken`, `422 validation_failed` (`fields.email`, `fields.password`).

### `POST /auth/login`
```json
{"email":"berat@example.com","password":"Sifre1234!"}
```
`200` — kayıtla aynı gövde. Hatalı e-posta ve hatalı şifre aynı `401 invalid_credentials` yanıtını alır.

### `POST /auth/refresh`
```json
{"refresh_token":"kq3V1m..."}
```
`200` — yeni `token` **ve yeni** `refresh_token`. Refresh token tek kullanımlıktır (rotasyon); aynı token ikinci kez `401 invalid_refresh_token` alır.
Sunucu yalnızca token'ın SHA-256 özetini saklar.

### `POST /auth/logout` (auth)
```json
{"refresh_token":"kq3V1m..."}      // yalnızca bu cihaz
{"all":true}                        // tüm cihazlar
```
`204`. İdempotenttir.

### `POST /auth/password/forgot`
```json
{"email":"berat@example.com"}
```
`202` — hesap olsun olmasın **aynı** yanıt (hesap ifşası yok):
```json
{"status":"accepted","message":"if an account exists, a reset link has been sent"}
```
Hesap varsa 1 saat geçerli, tek kullanımlık bir kod üretilir; önceki kodlar geçersiz olur. `SMTP_HOST` tanımlıysa kod
e-postayla gönderilir; tanımlı değilse gönderici yalnızca maskeli log yazar (`b***@example.com`).

### `POST /auth/password/reset`
```json
{"token":"<e-postadaki kod>","password":"YeniSifre123"}
```
`204`. Başarılı sıfırlama tüm refresh token'ları iptal eder (diğer cihazlarda oturum kapanır).
Hatalar: `400 invalid_reset_token`, `422` (`fields.password`).

### `POST /auth/email/resend` (auth)
Yeni bir doğrulama bağlantısı gönderir; önceki bağlantılar geçersiz olur.
`202 {"status":"sent"}` veya adres zaten doğrulanmışsa `200 {"status":"already_verified"}`.

### `POST /auth/email/verify`
```json
{"token":"<bağlantıdaki token>"}
```
`200` — güncel kullanıcı (`email_verified_at` dolu). Token tek kullanımlıktır, 24 saat geçerlidir.
Hata: `400 invalid_verification_token`.

### `GET /auth/email/verify?token=...`
E-postadaki bağlantı buraya gelir (`PUBLIC_BASE_URL` + bu yol). Tarayıcıda Türkçe bir sonuç sayfası gösterir
(`200` başarılı, `400` geçersiz veya süresi dolmuş bağlantı).

### `GET /me` (auth)
```json
{"id":"d72a0af5-...","email":"berat@example.com","name":"Berat","role":"user","email_verified_at":"2026-10-17T11:20:00Z","created_at":"2026-10-16T20:02:18Z"}
```
`email_verified_at` doğrulanmamış hesaplarda `null`; mobil uygulama bu durumda ana ekranda doğrulama uyarısı gösterir.

### `PATCH /me` (auth)
```json
{"name":"Berat Kaan"}
```
`200` — güncel kullanıcı.

### `PUT /me/password` (auth)
```json
{"current_password":"Sifre1234!","new_password":"DahaIyi5678"}
```
`204`; tüm refresh token'lar iptal edilir. `422`: `fields.current_password = "incorrect password"`,
`fields.new_password = "must differ from the current password"`.

### `DELETE /me` (auth)
`204` — hesap, ürünler, belgeler (dosyalar dahil), servis kayıtları ve bildirimler silinir (KVKK silme hakkı).

### `GET /me/export` (auth) → `application/zip`
Hesabın yedeği (KVKK veri taşınabilirliği): `backup.json` + `files/` altında tüm belgeler.
`Content-Disposition: attachment; filename="warranty-wallet-yedek-2026-10-17.zip"`.
```json
{"format":"warranty-wallet-backup","version":1,"exported_at":"...","account":{"email":"...","name":"..."},
 "products":[{"ref":"<eski id>","name":"...","purchase_date":"2026-09-12","warranty_months":24,"...":"...",
   "service_records":[{"ref":"...","problem":"screen_lines","status":"resolved","cost":350,"events":[...]}],
   "documents":[{"ref":"...","type":"invoice","original_name":"fatura.jpg","file":"files/<id>.jpg"}]}]}
```
Parolalar, token'lar ve hatırlatıcı durumları yedeğe girmez.

### `POST /me/import` (auth, gövde: yedek .zip, `Content-Type: application/zip`, en fazla 200 MB, 20 istek/dk)
Yedeği mevcut hesaba **ekler** (hiçbir şey silinmez). Aynı ad + alış tarihi + seri numarasına sahip ürünler atlanır,
bu yüzden aynı yedeği iki kez yüklemek kopya oluşturmaz. Garanti bitişi ve hatırlatıcılar yeniden hesaplanır,
belgelerin türü yine içerikten (magic bytes) belirlenir.
`200`:
```json
{"products":3,"service_records":1,"documents":4,"skipped_duplicates":0,"failed":1,
 "errors":["\"Kettle\" belgesi atlandı: dosya türü desteklenmiyor"]}
```
Hatalar: `422 invalid_backup` (zip değil, `backup.json` yok, biçim/sürüm uyumsuz), `413 too_large`.

---

## Ürünler

### `POST /products` (auth)
```json
{
  "name":"ASUS TUF VG27AQ","brand":"ASUS","model":"VG27AQ","category":"monitor",
  "seller":"MediaMarkt","purchase_date":"2026-09-12","price":12499,"currency":"TRY",
  "warranty_months":24,"serial_number":"R9LMQS123456","barcode":"4711081234567","notes":""
}
```
`barcode` (V2, isteğe bağlı, ≤128 karakter): kutudaki EAN/UPC/QR kodu.

`201`:
```json
{
  "id":"35ccbfd5-0c06-4975-86b1-ae6123de0cab","name":"ASUS TUF VG27AQ","brand":"ASUS","model":"VG27AQ",
  "category":"monitor","seller":"MediaMarkt","purchase_date":"2026-09-12","price":12499,"currency":"TRY",
  "warranty_months":24,"warranty_end":"2028-09-12","serial_number":"R9LMQS123456","notes":"",
  "created_at":"2026-10-16T20:02:18Z","updated_at":"2026-10-16T20:02:18Z","status":"active","days_left":697
}
```
- `warranty_end` sunucuda hesaplanır (ay sonu kuralı: 31.01 + 1 ay → 28/29.02); istemciden alınmaz.
- `status`: `active` | `expiring_soon` (≤30 gün) | `expired` | `unknown` — okuma anında hesaplanır.
- Doğrulama: `name` zorunlu ≤200 karakter; `purchase_date` zorunlu, 1990-01-01 ile yarın arası; `warranty_months` 0–240;
  `price` 0–1.000.000.000; `currency` 3 büyük harf (varsayılan `TRY`); diğer metinler ≤120, `notes` ≤2000 karakter
  (karakter sayılır, bayt değil).
- Ürün yazılınca 90/30/7 gün hatırlatıcıları planlanır.

### `GET /products?category=&status=&q=&code=` (auth)
- `q`: ad, marka, model, satıcı, seri numarası veya barkodda geçen metin (büyük/küçük harf duyarsız).
- `code` (V2 tarama): barkod **veya** seri numarasıyla birebir eşleşme (büyük/küçük harf duyarsız), ≤128 karakter.
  Mobil uygulama tarayıcıdan gelen kodu buna gönderir: `GET /products?code=5023417` → `{"count":1,"items":[...]}`.
```json
{"count":1,"items":[{ "id":"35ccbfd5-...", "name":"ASUS TUF VG27AQ", "status":"active", "days_left":697, "...":"..." }]}
```
`status` yalnızca `active|expiring_soon|expired|unknown`, `q` ≤100 karakter; aksi `422`.

### `GET /products/expiring?days=90` (auth)
```json
{"count":1,"days":90,"items":[ ... ]}
```
`days` 1–3650 (varsayılan 90). Kalan güne göre artan sırada.

### `GET /products/{id}` · `PUT /products/{id}` · `DELETE /products/{id}` (auth)
`PUT` gövdesi `POST` ile aynıdır (tam güncelleme). `DELETE` → `204`, belgeleri ve dosyaları da siler.

### `GET /categories` (auth)
```json
{"items":[{"slug":"phone","name_tr":"Telefon"},{"slug":"monitor","name_tr":"Monitör"}, "..."]}
```

### `GET /notifications` (auth)
```json
{"items":[{"id":"83060c3a-...","product_id":"35ccbfd5-...","type":"warranty_90",
  "message":"ASUS monitörünüzün garantisinin bitmesine 90 gün kaldı.","scheduled_at":"2028-06-14T07:00:00Z","sent":false}]}
```

### `POST /devices` (auth)
```json
{"token":"<FCM kayıt token'ı>","platform":"android"}
```
`204`. Uygulama girişten sonra çağırır; aynı token tekrar gönderilirse güncellenir, başka hesapla giriş yapılırsa
token o hesaba geçer. `422`: `fields.token` (20–4096 ASCII karakter, boşluksuz), `fields.platform` (`android`|`ios`).
Sunucuda `FCM_CREDENTIALS_FILE` tanımlıysa hatırlatıcı zamanı gelince kullanıcının tüm cihazlarına push gönderilir;
FCM'in artık geçersiz dediği token'lar silinir.

### `DELETE /devices/{token}` (auth) → `204`
Yalnızca kendi cihazınızı siler (çıkışta çağrılır). Başka kullanıcının token'ı için de `204` döner ama hiçbir şey silinmez.

---

## Belgeler

### `POST /products/{id}/documents` (auth, multipart)
Alanlar: `file` (zorunlu), `type` = `invoice|warranty|service|photo`, isteğe bağlı `service_record_id`.
```bash
curl -X POST $B/products/$PID/documents -H "$H" -F type=invoice -F "file=@fis.jpg;type=image/jpeg"
```
`201`:
```json
{"id":"fa071ee4-...","product_id":"35ccbfd5-...","type":"invoice","content_type":"image/jpeg",
 "size_bytes":55493,"original_name":"fis.jpg","created_at":"2026-10-16T20:02:18Z"}
```

### `GET /products/{id}/documents?type=` (auth) → `{"items":[...]}`
### `GET /documents/{id}/file` (auth) → dosyanın kendisi (`Content-Type` saklanan türdür)
### `DELETE /documents/{id}` (auth) → `204`

---

## Fiş okuma (OCR + AI)

### `POST /extract` (auth)
`multipart/form-data` ile `image` (fiş fotoğrafı) ve/veya `text` (cihazdaki OCR metni) ya da JSON `{"text": "..."}`.
```bash
curl -X POST $B/extract -H "$H" -F "image=@fis.jpg;type=image/jpeg"
```
`200` (gemini-3.5-flash, gerçek fiş görseli):
```json
{
  "fields": {"product_name":"ASUS TUF GAMING VG27AQ 27\" MONITOR","brand":"ASUS","model":"VG27AQ","category":"monitor",
             "seller":"MediaMarkt","purchase_date":"2026-09-12","price":12499,"currency":"TRY",
             "warranty_months":24,"serial_number":"R9LMQS123456"},
  "missing": [],
  "provider": "gemini",
  "model": "gemini-3.5-flash",
  "warnings": null,
  "raw_text": "",
  "suggested_warranty_months": 24,
  "warranty_end": "2028-09-12"
}
```
- `provider`: `gemini` | `rules` (Türkçe kural tabanlı ayrıştırıcı) | `none` (yalnız görsel geldi ve AI yok → formu kullanıcı doldurur).
- Gemini meşgul (503), kotası dolmuş (429) veya model yoksa (404) `GEMINI_FALLBACK_MODELS` sırasıyla denenir; hepsi başarısızsa kural
  tabanlıya düşülür ve `warnings` nedenini söyler, örn. `"gemini unavailable: ... HTTP 503 UNAVAILABLE ..."`.
- `missing`: kullanıcının doldurması gereken alanlar. `suggested_warranty_months` çıkarılamadıysa 24 (TR asgari süresi).
- Hatalar: görsel/metin yok → `422`; hiçbir yöntem okuyamadı → `502 extraction_failed`.

---

## AI arıza asistanı

### `POST /products/{id}/diagnose` (auth, fiş okuma ile aynı hız sınırı: 20/dk)
İstek: `{"problem":"wont_turn_on|screen_lines|physical_damage|other","description":"..."}`. `problem` boşsa `other` sayılır; `other` için açıklama en az 5 karakter olmalı. Açıklama en fazla 2000 karakter.
Gemini yapılandırılmışsa modelden yapılandırılmış JSON istenir; hata/kota/geçersiz yanıtta Türkçe kural tabanı yanıt verir (`provider: "rules"`, `warnings: ["ai_unavailable"]`). Garanti tavsiyesi ve uyarı metni her zaman sunucudaki verilerden üretilir, modelden alınmaz. Açıklamada duman, yanık kokusu, şişmiş pil, sıvı teması gibi ifadeler varsa `safety_warning` her durumda doldurulur.
```json
{
  "summary": "Telefonunuzun şarjdayken kapanması ... pil, şarj aksesuarları veya yazılımsal bir soruna işaret edebilir.",
  "likely_causes": [{"cause": "Pil tamamen boşalmış veya şarj sorunu", "likelihood": "high", "explanation": "..."}],
  "steps": [{"title": "Farklı bir şarj cihazı ve kablo deneyin", "detail": "..."}],
  "safety_warning": "",
  "recommend_service": true,
  "warranty_advice": "Ürününüz garanti kapsamında (bitişe 502 gün var). Cihazı açmayın ...",
  "provider": "gemini",
  "model": "gemini-2.5-flash",
  "disclaimer": "Bu öneriler genel bilgilendirme amaçlıdır; ..."
}
```
En fazla 5 neden ve 6 adım döner. Hatalar: `422 validation_failed` (`problem`, `description`), `404 not_found` (başka kullanıcının ürünü), `429 rate_limited`.

---

## Servis kayıtları ("Ürünüm Arızalandı")

### `POST /products/{id}/service-records` (auth)
```json
{"problem":"screen_lines","description":"Ekranda dikey çizgiler var","service_name":"ASUS Yetkili Servis","service_date":"2026-10-16"}
```
`201` — garanti kapsamındaysa kontrol listesi de döner:
```json
{
  "id":"4c9090b5-...","product_id":"35ccbfd5-...","problem":"screen_lines","description":"Ekranda dikey çizgiler var",
  "service_name":"ASUS Yetkili Servis","service_date":"2026-10-16","status":"open","notes":"","under_warranty":true,
  "created_at":"2026-10-16T20:04:13Z","updated_at":"2026-10-16T20:04:13Z",
  "checklist":[{"key":"invoice","label":"Fatura / fiş","ok":true},{"key":"warranty","label":"Garanti belgesi","ok":false},
               {"key":"serial_number","label":"Seri numarası","ok":true},{"key":"model","label":"Model bilgisi","ok":true}]
}
```
`problem`: `wont_turn_on|screen_lines|physical_damage|other`. `service_date` 1990-01-01 ile bugünden 1 yıl sonrası arası.

Kayıt alanları ayrıca `cost` (kullanıcının ödediği onarım ücreti, 0–1 milyar) ve `resolved_at` (durum `resolved` veya `cancelled` olunca sunucu doldurur) içerir. İsteğe bağlı `event_note` (≤500) zaman çizelgesindeki ilk kaydın notu olur.

### `PATCH /service-records/{id}` (auth)
```json
{"status":"in_service","service_name":"Yetkili Servis","event_note":"Servise teslim edildi","cost":0}
```
`status`: `open|in_service|resolved|cancelled`. Gönderilmeyen alanlar değişmez. Durum değişirse zaman çizelgesine bir olay eklenir (`event_note` onun notudur); kapanınca `resolved_at` dolar, yeniden açılınca silinir.

### `GET /service-records/{id}` (auth)
Kayıt + `product_name` + zaman çizelgesi (eskiden yeniye):
```json
{"id":"...","status":"resolved","cost":450.75,"resolved_at":"2026-10-16T09:00:00Z","product_name":"Arçelik Buzdolabı",
 "events":[{"id":"...","service_record_id":"...","status":"open","note":"Kayıt açıldı","created_at":"..."},
           {"id":"...","service_record_id":"...","status":"in_service","note":"Servise teslim edildi","created_at":"..."}]}
```

### `GET /service-records?status=` (auth)
`{"items":[... product_name ile ...],"summary":{"total":2,"active":1,"by_status":{"open":1,"resolved":1},"total_cost":450.75,"avg_resolution_days":3}}`. `summary` filtreden bağımsız tüm kayıtları kapsar; `avg_resolution_days` çözülmüş kayıt yoksa `null`. Geçersiz `status` → `422`.

### `GET /products/{id}/service-records` (auth) → `{"items":[...]}`

### `DELETE /service-records/{id}` (auth) → `204`
Zaman çizelgesi de silinir; kayda bağlı fotoğraflar ürünün belgeleri olarak kalır.

### `GET /products/{id}/checklist` (auth)
```json
{"days_left":697,"status":"active","under_warranty":true,"items":[{"key":"invoice","label":"Fatura / fiş","ok":true}, "..."]}
```

### `GET /products/{id}/service-pack.pdf?service_record_id=` (auth)
`200 application/pdf` — ürün bilgileri, garanti durumu, arıza açıklaması ve görsel belgeleri içeren servis paketi (örnekte 58 KB).

---

## Özet ekranı

### `GET /dashboard` (auth)
```json
{
  "greeting":"İyi akşamlar, Berat","total_products":1,"active_warranties":1,"expiring_soon":0,"expired":0,"unknown":0,
  "total_spent":{"TRY":12499},"service_records":1,"open_service_records":1,
  "by_category":[{"category":"monitor","name":"Monitör","count":1,"total":12499}],
  "upcoming":[],
  "analytics":{
    "primary_currency":"TRY",
    "protected_value":{"TRY":12499},
    "coverage_rate":1,
    "expiry_timeline":[{"month":"2026-10","count":0},"... 12 ay ..."],
    "monthly_purchases":[{"month":"2025-11","count":0},"...",{"month":"2026-09","count":1,"total":12499},{"month":"2026-10","count":0}],
    "top_brands":[{"brand":"ASUS","count":1}],
    "service":{"total":1,"active":1,"resolved":0,"under_warranty":1,"total_cost":0,"avg_resolution_days":null}
  }
}
```
`analytics` (V2): `protected_value` hâlâ garanti kapsamındaki ürünlerin fiyat toplamı (para birimi başına); `coverage_rate` garantisi süren ürünlerin oranı (0–1); `expiry_timeline` bu ay dahil önümüzdeki 12 ayda biten garanti sayısı; `monthly_purchases` son 12 ayın alımları (`total` yalnızca `primary_currency`, yani en çok harcanan para birimi için toplanır); `top_brands` en çok ürünü olan 5 marka (büyük/küçük harf duyarsız); `service` servis kayıtlarının özeti (ortalama çözüm süresi gün olarak, yoksa `null`).

---

## Admin (`role = admin`)

| Metot | Yol | Açıklama |
|---|---|---|
| GET | `/admin/stats` | `{"users":2,"products":1,"extractions":{"ok":7,"failed":1,"sample_size":8,"by_provider":{"gemini":6,"rules":1,"none":1},"by_model":{"gemini-3.5-flash":6}},"diagnoses":{"total":3,"ai":1,"rules":2,"ai_failed":1,"by_model":{"gemini-2.5-flash":1}}}` |
| GET | `/admin/users` | `{"items":[user...]}` |
| PATCH | `/admin/users/{id}` | `{"role":"admin"}` → güncel kullanıcı |
| DELETE | `/admin/users/{id}` | `204` (kendini silemez) |
| GET | `/admin/extraction-logs?failures=1&kind=receipt\|diagnosis` | Yapay zekâ kayıtları: fiş okuma ve arıza asistanı (`kind`, `provider`, `model`, `success`, `error`). Arıza açıklamasının kendisi saklanmaz. |
| POST | `/admin/categories` | `{"slug":"drone","name_tr":"Drone"}` → upsert |
| DELETE | `/admin/categories/{slug}` | `204` |

Admin olmayan kullanıcı `403 forbidden`, token yoksa `401` alır.

### Web yönetim paneli (V2): `GET /admin/`
Backend ikili dosyasına gömülü, bağımlılıksız HTML/JS paneli: özet, kullanıcılar (arama, rol değiştirme, silme), fiş okuma kayıtları (yalnızca hatalar filtresi) ve kategoriler (ekle/güncelle/sil). Yalnızca yukarıdaki admin API'sini kullanır; yetki kontrolü API'dedir. Giriş `/auth/login` ile yapılır, admin olmayan hesap reddedilir. Token'lar yalnızca sekme ömrü boyunca `sessionStorage`'da tutulur; süresi dolan access token refresh token ile bir kez yenilenir, çıkışta refresh token sunucuda da iptal edilir. Sıkı CSP (`script-src 'self'`, satır içi betik yok, `frame-ancestors 'none'`) uygulanır ve veriler yalnızca `textContent` ile yazılır. İlk yöneticiyi atamak için `ADMIN_EMAILS` ortam değişkeni kullanılır.
