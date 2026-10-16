# İlerleme (PROGRESS)

Son güncelleme: 2026-10-16

## Tamamlananlar
- [x] Repo iskeleti, README, ARCHITECTURE, .gitignore, .env.example

## Devam eden / sıradaki
- [ ] Backend iskeleti (Go, config, healthz, migration'lar)
- [ ] Auth, ürün CRUD, garanti hesabı
- [ ] Belge yükleme, Gemini çıkarım + kural tabanlı yedek
- [ ] Servis kaydı, PDF paketi, dashboard, bildirim planlama
- [ ] Flutter uygulaması
- [ ] Docker Compose, CI

## Alınan kararlar
- Monorepo: backend (Go), mobile (Flutter), docs.
- Go standart net/http (framework yok), pgx, gömülü SQL migration.
- V1'de Redis yok; bildirimler cihazda yerel + sunucuda tablo/işçi.
- Gemini çağrısı yalnızca backend'de; anahtar yoksa kural tabanlı ayrıştırıcı.
