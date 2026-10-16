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
| Hız sınırı | `/auth/*`: IP başına dakikada 10, `/extract`: dakikada 20 → `429 rate_limited` + `Retry-After`. `X-Forwarded-For` yalnızca `TRUST_PROXY=true` ise dikkate alınır. |
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
  "user": {"id":"d72a0af5-...","email":"berat@example.com","name":"Berat","role":"user","created_at":"2026-10-16T20:02:18Z"}
}
```
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
Hesap varsa 1 saat geçerli, tek kullanımlık bir kod üretilir; önceki kodlar geçersiz olur. V1'de e-posta gönderici
yalnızca maskeli log yazar (`b***@example.com`); SMTP `mail.Sender` arayüzüne eklenecek.

### `POST /auth/password/reset`
```json
{"token":"<e-postadaki kod>","password":"YeniSifre123"}
```
`204`. Başarılı sıfırlama tüm refresh token'ları iptal eder (diğer cihazlarda oturum kapanır).
Hatalar: `400 invalid_reset_token`, `422` (`fields.password`).

### `GET /me` (auth)
```json
{"id":"d72a0af5-...","email":"berat@example.com","name":"Berat","role":"user","created_at":"2026-10-16T20:02:18Z"}
```

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

---

## Ürünler

### `POST /products` (auth)
```json
{
  "name":"ASUS TUF VG27AQ","brand":"ASUS","model":"VG27AQ","category":"monitor",
  "seller":"MediaMarkt","purchase_date":"2026-09-12","price":12499,"currency":"TRY",
  "warranty_months":24,"serial_number":"R9LMQS123456","notes":""
}
```
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

### `GET /products?category=&status=&q=` (auth)
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

### `PATCH /service-records/{id}` (auth)
```json
{"status":"in_service","notes":"Servise teslim edildi"}
```
`status`: `open|in_service|resolved|cancelled`. Gönderilmeyen alanlar değişmez.

### `GET /service-records` · `GET /products/{id}/service-records` · `GET /service-records/{id}` (auth)

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
  "upcoming":[]
}
```

---

## Admin (`role = admin`)

| Metot | Yol | Açıklama |
|---|---|---|
| GET | `/admin/stats` | `{"users":2,"products":1,"extractions":{"ok":7,"failed":1,"sample_size":8,"by_provider":{"gemini":6,"rules":1,"none":1}}}` |
| GET | `/admin/users` | `{"items":[user...]}` |
| PATCH | `/admin/users/{id}` | `{"role":"admin"}` → güncel kullanıcı |
| DELETE | `/admin/users/{id}` | `204` (kendini silemez) |
| GET | `/admin/extraction-logs?failures=1` | Fiş okuma kayıtları (OCR hataları görünümü) |
| POST | `/admin/categories` | `{"slug":"drone","name_tr":"Drone"}` → upsert |
| DELETE | `/admin/categories/{slug}` | `204` |

Admin olmayan kullanıcı `403 forbidden`, token yoksa `401` alır.
