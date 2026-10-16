# Warranty Wallet — Tüm garantilerin tek uygulamada

> **"Satın aldığın ürünleri unutma, garanti haklarını kaybetme."**

Warranty Wallet; laptop, kulaklık, telefon, monitör, kahve makinesi gibi satın aldığın ürünlerin
fişlerini, faturalarını, garanti belgelerini ve servis kayıtlarını tek yerde saklayan ve garanti
sürelerini otomatik takip eden bir mobil uygulamadır.

Merkezdeki deneyim: **fişin fotoğrafını çek, gerisini uygulama halletsin.**

```
Ürün Ekle → fiş/fatura fotoğrafı → OCR → ham metin → AI (Gemini) → yapılandırılmış veri → kullanıcı onayı → Kaydet
```

Örnek çıktı: *Ürün: ASUS TUF VG27AQ · Kategori: Monitör · Satıcı: MediaMarkt · Satın alma: 12.09.2026 ·
Fiyat: 12.499 TL · Garanti: 24 ay · Bitiş: 12.09.2028*

Bu proje aynı zamanda bir üniversite ekip projesidir (Yazılım Proje Yönetimi dersi). Mimari ve
kararlar [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), ilerleme durumu [docs/PROGRESS.md](docs/PROGRESS.md)
dosyasındadır.

## Repo yapısı (monorepo)

| Klasör | İçerik |
|---|---|
| `backend/` | Go REST API (net/http + PostgreSQL), Gemini entegrasyonu, PDF üretimi |
| `mobile/` | Flutter mobil uygulama (Android / iOS) |
| `docs/` | Mimari, API, ilerleme ve ekip dokümanları |
| `.github/workflows/` | GitHub Actions CI (backend + mobile) |
| `docker-compose.yml` | Backend + PostgreSQL |

## Yol haritası

**V1 (MVP)** — Kimlik doğrulama, ürün ekleme, fatura fotoğrafı yükleme, OCR, AI ile bilgi çıkarma,
garanti bitiş tarihi hesaplama, ürün listesi, ürün detay ekranı, fatura ve garanti belgesi saklama,
garanti bitiş bildirimleri (90/30/7 gün), servis kaydı oluşturma, "Ürünüm Arızalandı" akışı,
servis belgeleri PDF paketi, dashboard.

**V2** — QR/barkod ve seri numarası tarama, servis merkezi bulma, AI arıza asistanı, aile hesabı,
bulut yedekleme, analitik, Admin paneli (kullanıcılar, kategoriler, markalar, OCR hataları).

**V3** — Mağaza ve üretici entegrasyonları, e-postadan fatura içe aktarma, ikinci el satış,
ürün değer tahmini, uzatılmış garanti teklifleri, Business (servis sağlayıcı) rolü.

## Kurulum

### Gereksinimler
- Docker + Docker Compose (en kolay yol) **veya** Go 1.24+ ve PostgreSQL 16+
- Flutter 3.x (stable) — mobil uygulama için

### 1) Ortam değişkenleri
```bash
cp .env.example .env
# .env dosyasını açıp değerleri doldurun
```
`.env` dosyası `.gitignore` içindedir; **asla commit etmeyin.**

**GEMINI_API_KEY nasıl ayarlanır?**
1. <https://aistudio.google.com/apikey> adresinden ücretsiz bir Gemini API anahtarı oluşturun.
2. `.env` içindeki `GEMINI_API_KEY=` satırının sonuna anahtarı yapıştırın (veya shell'de
   `export GEMINI_API_KEY=...`).
3. Anahtar boşsa, geçersizse ya da ücretsiz kota dolarsa backend otomatik olarak **kural tabanlı
   ayrıştırıcıya** geçer; uygulama yine çalışır, kullanıcı alanları elle düzeltebilir.

Anahtar yalnızca backend'de kullanılır, mobil uygulamaya gömülmez.

### 2) Backend'i çalıştırma
```bash
docker compose up --build          # API: http://localhost:8080, PostgreSQL: 5432
curl http://localhost:8080/healthz
```
Docker olmadan:
```bash
cd backend
export DATABASE_URL=postgres://warranty:warranty@localhost:5432/warranty?sslmode=disable
export JWT_SECRET=dev-secret
go run ./cmd/api                    # migration'lar açılışta otomatik uygulanır
```

### 3) Mobil uygulamayı çalıştırma
```bash
cd mobile
flutter pub get
# Android emülatörü için backend adresi 10.0.2.2'dir:
flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

### 4) Testler
```bash
cd backend && go vet ./... && go test ./...
cd mobile && flutter analyze && flutter test
```
Testler gerçek Gemini API'sini çağırmaz (mock kullanılır); CI'da anahtar gerekmez.

## Lisans
MIT
