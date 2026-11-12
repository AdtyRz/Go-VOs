# Go-Voting

Aplikasi pemungutan suara elektronik OSIS berbasis Go dan MySQL. Antarmuka siswa dibuat mobile-first, panel panitia dibuat untuk pekerjaan operasional, dan empat tema visual memiliki tata letak, tipografi, permukaan, serta ornamen yang benar-benar berbeda.

## Fitur

- Halaman beranda terpisah dari halaman `/token` yang khusus untuk memasukkan token.
- Validasi token sekali pakai, sesi memilih, bilik suara, konfirmasi, dan halaman selesai.
- Suara anonim: tabel `votes` hanya menyimpan pemilihan, calon, dan waktu.
- Token dibuat massal, dicabut, dipantau, diekspor, dan dicetak sebagai kartu A4 oleh panitia.
- Panel admin untuk sekolah, tema, periode pemilihan, pasangan calon, token, hasil, dan jejak audit.
- Hasil publik di beranda: live saat pemilihan berlangsung atau final setelah ditutup, dengan tombol nyala/mati realtime dari panel panitia.
- Hasil langsung melalui WebSocket serta ekspor CSV dan laporan PDF.
- Empat tema: Batik, Indonesia, Wayang, dan Modern Glass.
- Kata sandi Argon2id, hash token HMAC, cookie HttpOnly, CSRF, pembatasan laju, dan header keamanan.
- Migrasi otomatis, seed demo opsional, dan unggahan logo yang divalidasi.
- Aset statis ditanam ke dalam binary sehingga deployment hanya membutuhkan satu file executable dan MySQL.

## Tumpukan

- Go 1.26 dengan `net/http` dan `html/template`
- MySQL 8
- HTML, CSS, dan JavaScript tanpa framework
- WebSocket untuk pembaruan hasil
- Argon2id dan HMAC-SHA256
- `go-sql-driver/mysql`, `gorilla/websocket`, `go-pdf/fpdf`

## Mulai Cepat dengan Docker Compose

Prasyarat: Docker Engine dan Docker Compose v2.

1. Salin konfigurasi contoh.

   ```bash
   cp .env.example .env
   ```

2. Buat `TOKEN_SECRET`.

   ```bash
   openssl rand -hex 32
   ```

   Masukkan hasilnya ke `TOKEN_SECRET` di `.env`.

3. Isi `SCHOOL_NAME`, `SCHOOL_SLUG`, `SCHOOL_EMAIL`, `SCHOOL_ADDRESS`, `SCHOOL_PHONE`, `SCHOOL_WEBSITE`, `ADMIN_USERNAME`, dan `ADMIN_PASSWORD` di `.env`. Gunakan kata sandi admin yang panjang dan unik.

4. Jalankan aplikasi.

   ```bash
   docker compose up --build -d
   ```

5. Buka `http://localhost:8080`. Panel admin berada di `http://localhost:8080/admin/login`.

6. Login dengan akun admin dari `.env`, lalu buat token dari menu Token Pemilih.
7. Bagikan alamat `http://localhost:8080/token` kepada siswa. Halaman beranda menampilkan informasi pemilihan dan hasil publik sesuai pengaturan visibilitas.

Tidak ada token demo yang dibuat otomatis. Token asli hanya ditampilkan sekali setelah dibuat, jadi simpan, ekspor, atau cetak kartu token sebelum menutup jendela. Tombol `Unduh PDF kartu token` hanya tersedia selama dialog hasil generate masih terbuka; batch lama tidak dapat dicetak ulang karena nilai token disimpan sebagai hash. Halaman lama `/vote` tetap tersedia sebagai pengalihan ke `/token`.

Untuk menghentikan layanan:

```bash
docker compose down
```

Data MySQL dan unggahan disimpan di volume Docker. Menjalankan `docker compose down -v` akan menghapus keduanya.

## Menjalankan Tanpa Docker

Prasyarat: Go 1.26 dan MySQL 8.

1. Buat database dan pengguna MySQL.

   ```sql
   CREATE DATABASE go_voting CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   CREATE USER 'osivote'@'%' IDENTIFIED BY 'ganti-password';
   GRANT ALL PRIVILEGES ON go_voting.* TO 'osivote'@'%';
   FLUSH PRIVILEGES;
   ```

2. Salin `.env.example` menjadi `.env`, lalu sesuaikan `DB_DSN`, `TOKEN_SECRET`, identitas sekolah, dan akun admin.

3. Jalankan server.

   ```bash
   go run ./cmd/server
   ```

Migrasi dijalankan otomatis saat `AUTO_MIGRATE=true`. Seed sekolah dan admin pertama hanya berjalan ketika database masih kosong. Atur `SEED_DEMO=true` bila ingin membuat pemilihan contoh beserta tiga pasangan calon.

## Alur Siswa dan Hasil Publik

1. Siswa membuka beranda `/` untuk membaca informasi pemilihan. Bila hasil publik diaktifkan, beranda juga menampilkan perolehan suara.
2. Siswa membuka `/token`, memasukkan token lima karakter dari panitia, lalu memilih di bilik suara.
3. Setelah suara tersimpan, siswa kembali ke beranda dengan konfirmasi bahwa suara sudah tercatat. Token salah menampilkan pesan `Token yang digunakan salah, coba lagi.` dan tetap berada di halaman token.

Pengaturan `results_visibility` pada pemilihan menentukan tampilan publik:

- `LIVE`: hasil tampil di beranda sejak pemilihan aktif dan diperbarui melalui WebSocket `/ws/public/results`.
- `FINAL`: hasil disembunyikan dari publik sampai pemilihan ditutup, lalu ditampilkan sebagai hasil akhir.

Panitia dapat menyalakan atau mematikan hasil publik realtime lewat tombol `Hasil publik realtime` di halaman Periode pemilihan atau Hasil pemilihan. Tombol ini tersedia selama status pemilihan `ACTIVE` atau `PAUSED`, jadi periode tidak perlu diedit ulang. Saat dimatikan, beranda siswa langsung menyembunyikan hasil dan koneksi WebSocket publik menerima event `results_hidden`; saat dinyalakan, hasil kembali tampil dan diperbarui otomatis.

Hasil publik hanya memuat nomor urut, nama pasangan, perolehan suara, peringkat, persentase, dan total suara sah. Calon tanpa suara ditampilkan dengan tanda `-`, bukan peringkat `#1`. Token, sesi, dan identitas pemilih tidak pernah ditampilkan. Panitia dapat mengunduh kartu token A4 dari menu Token Pemilih; setiap halaman memuat 28 kartu.

## Konfigurasi

| Variabel | Wajib | Keterangan |
| --- | --- | --- |
| `APP_ENV` | Tidak | `development` atau `production`. Default `development`. |
| `APP_ADDR` | Tidak | Alamat listen, default `:8080`. |
| `APP_BASE_URL` | Tidak | URL publik aplikasi, dipakai untuk metadata dan pesan. |
| `APP_TIMEZONE` | Tidak | Zona waktu tampilan, default `Asia/Jakarta`. |
| `DB_DSN` | Ya | DSN MySQL, harus memakai `parseTime=true`. |
| `AUTO_MIGRATE` | Tidak | Jalankan migrasi saat startup, default `true`. |
| `TOKEN_SECRET` | Ya | Minimal 64 karakter hex. Gunakan `openssl rand -hex 32`. |
| `COOKIE_SECURE` | Tidak | Wajib `true` saat `APP_ENV=production`. |
| `TRUST_PROXY` | Tidak | Set `true` hanya di belakang reverse proxy tepercaya. |
| `SESSION_TTL` | Tidak | Masa berlaku sesi admin, default `12h`. |
| `SCHOOL_NAME` | Ya saat inisialisasi | Nama sekolah. |
| `SCHOOL_SLUG` | Tidak | Slug sekolah, dibuat otomatis dari nama bila kosong. |
| `SCHOOL_EMAIL` | Tidak | Email sekolah. |
| `SCHOOL_ADDRESS` | Tidak | Alamat sekolah. |
| `SCHOOL_PHONE` | Tidak | Telepon sekolah. |
| `SCHOOL_WEBSITE` | Tidak | Situs sekolah. |
| `ADMIN_NAME` | Tidak | Nama panitia, default `Administrator`. |
| `ADMIN_USERNAME` | Ya saat inisialisasi | Username admin pertama. |
| `ADMIN_EMAIL` | Tidak | Email admin pertama. |
| `ADMIN_PASSWORD` | Ya saat inisialisasi | Minimal 8 karakter. Gunakan kata sandi panjang. |
| `SEED_DEMO` | Tidak | Buat pemilihan dan calon contoh, default `false`. |
| `UPLOAD_DIR` | Tidak | Folder unggahan, default `./storage/uploads`. |

Compose memakai `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_ROOT_PASSWORD`, `MYSQL_PORT`, dan `APP_PORT` bila tersedia. Nilai defaultnya cocok untuk pengembangan lokal.

Untuk produksi, set `APP_ENV=production`, `COOKIE_SECURE=true`, `SEED_DEMO=false`, dan letakkan aplikasi di belakang reverse proxy HTTPS.

## Perintah

| Perintah | Kegunaan |
| --- | --- |
| `make run` | Jalankan server dari source. |
| `make build` | Build binary ke `bin/govoting`. |
| `make test` | Jalankan seluruh unit test dan uji privasi statis. |
| `make test-integration` | Jalankan uji integrasi MySQL memakai `TEST_DB_DSN`. |
| `make test-all` | Jalankan semua pengujian. |
| `make vet` | Jalankan `go vet`. |
| `make fmt` | Format seluruh file Go. |
| `make docker-up` | Build dan jalankan Docker Compose. |
| `make docker-down` | Hentikan Docker Compose. |
| `make docker-logs` | Ikuti log aplikasi. |
| `make clean` | Hapus hasil build dan coverage. |

Uji integrasi membutuhkan database terpisah. Contoh:

```bash
CREATE_DATABASE_SQL="CREATE DATABASE go_voting_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
docker exec -it <container-mysql> mysql -uroot -p -e "$CREATE_DATABASE_SQL"
TEST_DB_DSN='osivote:osivote_dev@tcp(127.0.0.1:3307)/go_voting_test?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27' make test-integration
```

Tanpa `TEST_DB_DSN`, uji integrasi dilewati dan hanya uji statis yang berjalan.

## Struktur Proyek

```text
cmd/server/          entrypoint aplikasi
internal/bootstrap/  seed sekolah, admin, dan data demo
internal/config/     pembacaan konfigurasi dan .env
internal/database/   koneksi, migrasi, dan pemecah SQL
internal/httpapi/    route, handler, middleware, render, ekspor
internal/models/     model domain
internal/security/   Argon2id, hash token, hash sesi, rate limiter
internal/store/      query MySQL dan transaksi
internal/upload/     validasi dan pengolahan gambar
internal/ws/         hub WebSocket
migrations/          skema MySQL
tests/               uji privasi dan integrasi
web/                 template, CSS, JavaScript, font, gambar
```

## Tema

- Batik: kertas hangat, motif parang dan kawung, tipografi serif, bingkai berlapis.
- Indonesia: merah putih, tipografi poster, garis tegas, ritme editorial.
- Wayang: latar malam, emas, lengkung gunungan, tipografi klasik.
- Modern Glass: permukaan translusen, sudut besar, navigasi pil, tipografi geometris.

Warna utama dan sekunder dapat diubah dari panel Sekolah. Palet aksen tema tetap dijaga agar kontras dan identitas visual tidak rusak.

## Anonimitas dan Keamanan

Tabel `votes` sengaja tidak memiliki kolom token, sesi, siswa, perangkat, atau alamat IP. Token berubah menjadi `USED` dan sesi memilih ditutup dalam transaksi yang sama dengan penyimpanan suara. Aplikasi tetap dapat mencatat metadata operasional di log dan jejak audit, tetapi metadata tersebut tidak dihubungkan ke pilihan suara.

Jangan menambahkan kolom identitas pemilih ke tabel `votes`. Jangan mencatat token asli, cookie, atau isi pilihan siswa di log. Untuk deployment produksi, gunakan HTTPS, batasi akses database, aktifkan pencadangan, dan tinjau bagian keamanan sebelum pemilihan dibuka.

## Lisensi Font

Font Fraunces, DM Sans, Anton, Cormorant Garamond, dan Manrope disertakan di `web/static/fonts` dengan lisensi SIL Open Font License 1.1. Teks lisensi berada di `web/static/fonts/OFL.txt`.

## Dokumentasi

Panduan instalasi rinci, deployment, keamanan, privasi, pengujian keamanan, dan administrasi disimpan di luar repositori ini agar repositori tetap berisi aplikasi dan konfigurasi yang diperlukan untuk berjalan.
