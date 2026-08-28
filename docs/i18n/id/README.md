# apidoc-go — Plugin Dokumentasi API Umum untuk Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Pendahuluan

**apidoc-go** adalah pustaka plugin dokumentasi API umum untuk Go: dokumentasi antarmuka dideklarasikan bersama rute sebagai **struct bertipe (typed struct)** saat rute didaftarkan, sehingga dokumentasi lahir bersama rute; Web UI tertanam menyediakan penelusuran dan pengujian daring, serta dilengkapi autentikasi kata sandi, manajemen multi-aplikasi/multi-versi, data Mock, dan kemampuan ekspor JSON / TypeScript. Sekali integrasi, berlaku untuk semua kerangka kerja, tanpa perlu mengubah proyek yang sudah ada.

## Fitur Proyek

| # | Fitur | Deskripsi |
|---|------|------|
| 1 | Pembuatan dokumentasi otomatis | Deklarasi Doc bertipe, didaftarkan bersama rute; satu deklarasi, dokumentasi lahir bersama rute |
| 2 | Pengujian daring | Permintaan langsung ke antarmuka asli dari peramban, tanpa perantara server, bebas SSRF secara alami |
| 3 | Data Mock | Contoh Mock tingkat bidang, mempercepat integrasi antarmuka |
| 4 | Multi-aplikasi / multi-versi | Manajemen pohon App / Version, satu plugin mencakup dokumentasi seluruh proyek |
| 5 | Autentikasi kata sandi | Kata sandi global + kata sandi tingkat aplikasi, HMAC Token · perbandingan waktu konstan |
| 6 | Dokumentasi Markdown | Rendering aman dengan goldmark, HTML asli otomatis dihilangkan |
| 7 | Adaptasi multi-kerangka | net/http · Gin · Echo · Chi · Fiber, sekali integrasi berlaku untuk semua kerangka |
| 8 | Ekspor JSON / TypeScript | Ekspor tipe antarmuka sekali klik, integrasi frontend-backend lebih lancar |
| 9 | Perlindungan keamanan | Tanpa SSRF · CORS dibatasi daftar putih · cegah XSS · cegah path traversal |

## Ikhtisar Arsitektur

![Arsitektur Proyek](../../docs/svg/architecture.svg)

![Fitur Proyek](../../docs/svg/features.svg)

![Siklus Hidup Proyek](../../docs/svg/lifecycle.svg)

## Struktur Proyek

```
apidoc-go/
├── apidoc.go            # Pintu masuk inti: New / Register / Mount / Handler
├── doc.go               # Tipe Route / Doc / Param / Response
├── model/               # Model data
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Penyimpanan dan deduplikasi
│   └── store.go         #   Deduplikasi berdasarkan (app, version, method, url), yang terakhir menimpa
├── auth/                # Autentikasi
│   └── auth.go          #   Verifikasi kata sandi · Penerbitan dan verifikasi HMAC Token
├── server/              # JSON API + Web UI tertanam
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Lapisan adapter kerangka (antarmuka bertipe)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
└── docs/                # Dokumentasi dan aset
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Diagram arsitektur / fitur / siklus hidup
```

## Panduan Penggunaan

### Instalasi

```bash
go get github.com/erikwang2013/apidoc-go
```

### Memulai Cepat (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "我的 API 文档"})

	// Dokumentasi dideklarasikan bersama rute
	if err := s.Register(apidoc.Route{
		Method: "GET",
		URL:    "/hello",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello"))
		}),
		Doc: apidoc.Doc{
			App:     "demo",
			Version: "v1",
			Action:  "hello",
			Title:   "打招呼",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "你的名字"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Buka http://localhost:8080/apidoc di peramban
}
```

### Memulai Cepat (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "我的 API 文档"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "打招呼"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Untuk Echo / Chi / Fiber cukup ganti konstruktor adapter: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)`, kode lainnya sepenuhnya sama.

### Opsi Konfigurasi

| Opsi | Nilai default | Deskripsi |
|--------|--------|------|
| `Prefix` | `/apidoc` | Jalur pemasangan dokumentasi |
| `Title` | `API Docs` | Judul dokumentasi |
| `Desc` | — | Deskripsi dokumentasi |
| `GlobalParams` | — | Parameter global, digabungkan ke setiap Action |
| `Auth` | — | Konfigurasi autentikasi: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Konfigurasi aplikasi: `Key` / `Title` / `Password` (kata sandi independen per aplikasi) |
| `DebugOrigins` | — | Sumber yang diizinkan untuk pengujian lintas domain (default: tidak ada) |

### Adapter Kerangka

| Kerangka | Konstruktor |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

## Dokumentasi Multibahasa

| Bahasa | Tautan |
|------|------|
| English | [English](../en/README.md) |
| 한국어 | [한국어](../ko/README.md) |
| Русский | [Русский](../ru/README.md) |
| Deutsch | [Deutsch](../de/README.md) |
| Français | [Français](../fr/README.md) |
| Español | [Español](../es/README.md) |
| Português | [Português](../pt/README.md) |
| हिन्दी | [हिन्दी](../hi/README.md) |
| العربية | [العربية](../ar/README.md) |
| বাংলা | [বাংলা](../bn/README.md) |
| Bahasa Indonesia | [Bahasa Indonesia](../id/README.md) |
| 日本語 | [日本語](../ja/README.md) |

## Dukung Kami

Jika proyek ini bermanfaat bagi Anda, silakan pindai kode QR untuk memberikan donasi dukungan, agar kami tetap termotivasi memelihara dan memperbarui proyek ini!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="Donasi WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="Donasi Alipay" />
  <p>WeChat Pay (kiri) · Alipay (kanan)</p>
</div>

### Donasi Transfer Bank Global

**【Informasi Penerima】**
Beneficiary Name: WANG KEXUN
Account Number: 881015918251

**【Bank Penerima】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Bank Name: ZA Bank Limited
Bank Code: 387
Bank Address: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Bank Perantara Transfer Lintas Negara (jika diperlukan)】**

Perlu diperhatikan bahwa ini adalah informasi bank perantara (bank koresponden) untuk transfer lintas negara, bukan informasi bank penerima. Silakan tanyakan kepada bank pengirim apakah informasi bank perantara diperlukan.

Bank perantara untuk transfer Dolar Hong Kong, Yuan Tiongkok, dan Dolar AS adalah Citibank ——

Bank Name: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Bank Code: 006
Branch Name: Hong Kong Branch
Branch Code: 391
Bank Address: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Bank perantara untuk mata uang lainnya adalah BNY Mellon ——

Bank Name: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Bank Address: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Dibuat dengan ❤️ oleh <a href="https://erik.xyz">erik.xyz</a></div>
