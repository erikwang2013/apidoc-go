![Maskot Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — Plugin Dokumentasi API Umum untuk Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Pendahuluan

**Versi saat ini: v1.1.1** (lihat [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** adalah pustaka plugin dokumentasi API umum untuk Go: dokumentasi antarmuka dideklarasikan bersama rute sebagai **struct bertipe (typed struct)** saat rute didaftarkan, sehingga dokumentasi lahir bersama rute. Dokumentasi disajikan melalui API JSON lama `/api/*` dan UIC-1 `/ui/v1/*`, sehingga UI eksternal mana pun dapat merendernya dan mengirim permintaan uji — atau UI Vue yang sudah dipaketkan dapat disajikan dalam proses yang sama melalui `Config.UIPath`. Dilengkapi autentikasi kata sandi, manajemen multi-aplikasi/multi-versi, data Mock, ekspor JSON / TypeScript, parsing komentar dan definisi protokol sembilan format, cache respons, dan pelengkapan parameter otomatis; eksekutor protokol sisi server dikompilasi sesuai permintaan berdasarkan build tag dan mencakup famili protokol HTTP, RPC, socket, messaging, IoT, industrial, dan media. Sekali integrasi, berlaku untuk semua kerangka kerja — tanpa perlu mengubah proyek yang sudah ada.

## Fitur Proyek

| # | Fitur | Deskripsi |
|---|------|------|
| 1 | Pembuatan dokumentasi otomatis | Deklarasi Doc bertipe, didaftarkan bersama rute; satu deklarasi, dokumentasi lahir bersama rute |
| 2 | API dokumentasi | `/api/*` lama (menus / protocols / detail / export) dan UIC-1 `/ui/v1/*` (docs, debug, share, generator, export), untuk UI eksternal atau UI Vue bawaan |
| 3 | Data Mock | Contoh Mock tingkat bidang, mempercepat integrasi antarmuka |
| 4 | Multi-aplikasi / multi-versi | Manajemen pohon App / Version, satu plugin mencakup dokumentasi seluruh proyek |
| 5 | Autentikasi kata sandi | Kata sandi global + kata sandi tingkat aplikasi, HMAC Token · perbandingan waktu konstan |
| 6 | Dokumentasi Markdown | Rendering aman dengan goldmark, HTML asli otomatis dihilangkan |
| 7 | Adaptasi multi-kerangka | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, sekali integrasi berlaku untuk semua kerangka |
| 8 | Ekspor JSON / TypeScript | Ekspor tipe antarmuka sekali klik, integrasi frontend-backend lebih lancar |
| 9 | Perlindungan keamanan | Autentikasi kata sandi · CORS dibatasi daftar putih · cegah XSS · cegah path traversal · proteksi SSRF dan batas sesi/byte untuk debug sisi server |
| 10 | Parsing anotasi dan definisi | go/ast mem-parsing `@apidoc` / `@protocol` / `@definition`; sembilan format definisi bawaan: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC, dan XML-RPC |
| 11 | Cache HTTP | Cache respons berversi + ETag/304, endpoint dokumentasi merespons dalam milidetik |
| 12 | Pelengkapan parameter otomatis | reflect menyimpulkan parameter permintaan dari tanda tangan handler |
| 13 | Anotasi protokol apa pun | `@protocol` menandai protokol; detail mengembalikan protocol/method/url/params/responses/mock beserta definisi protokol yang telah di-parse |
| 14 | UI Vue bawaan (opsional) | Arahkan `Config.UIPath` ke build paket UIC-1 untuk menyajikan UI Vue dalam proses yang sama; nilai kosong kembali ke API JSON |
| 15 | Eksekutor protokol sisi server | Tujuh famili protokol dikompilasi berdasarkan build tag; nonaktif secara default dan fail closed bila tidak dikompilasi, dengan `apidoc_debug_all` mengaktifkan semuanya |

## Ikhtisar Arsitektur

![Arsitektur Proyek](../../svg/architecture.svg)

![Fitur Proyek](../../svg/features.svg)

![Siklus Hidup Proyek](../../svg/lifecycle.svg)

## Struktur Proyek

```
apidoc-go/
├── apidoc.go            # Pintu masuk inti: New / Register / Mount / Handler
├── VERSION              # Versi proyek (v1.1.1); alur kerja rilis membuat tag darinya
├── doc.go               # Tipe Route / Doc / Param / Response
├── model/               # Model data
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Definisi protokol eksternal @definition
│   ├── protocol.go      #   skema URL → protokol
│   └── protocols.go     #   registri protokol / alias / port default
├── store/               # Penyimpanan dan deduplikasi
│   └── store.go         #   Deduplikasi berdasarkan (app, version, method, url), yang terakhir menimpa
├── auth/                # Autentikasi
│   └── auth.go          #   Verifikasi kata sandi · Penerbitan dan verifikasi HMAC Token
├── server/              # API HTTP
│   ├── server.go        #   rute /api lama · auth / CORS / cache
│   ├── ui.go            #   UI Vue statis via UIPath (fallback SPA · akses dibatasi ke jalur)
│   └── uiapi/           #   API UIC-1 /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     baca dokumen · parsing definisi
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generator · template kode · share · cache
│       ├── share.go
│       └── debug/       #     eksekutor protokol sisi server (build tag per famili)
├── adapter/             # Lapisan adapter kerangka (antarmuka bertipe)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Parser anotasi otomatis
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   sembilan parser definisi protokol
├── export/              # Ekspor
│   └── export.go        #   Definisi antarmuka TypeScript
├── mock/                # Data Mock
│   └── mock.go          #   Pembuatan contoh tingkat bidang
├── example/             # Proyek contoh (8 kerangka :8081–:8088)
│   ├── main.go
│   └── handlers/        #   Contoh komentar @apidoc
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
	// UI eksternal membaca /apidoc/api/menus atau UIC-1 /apidoc/ui/v1/*;
	// dengan UIPath disetel, buka http://localhost:8080/apidoc/ secara langsung
}
```

### Mengaktifkan UI Vue UIC-1 Bawaan dan Eksekutor Sisi Server

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Opsional: keluaran `npm run package:ui` dari apidoc-ui
	// (index.html / config.js / assets/).
	// Biarkan kosong untuk hanya menyajikan API JSON dan tetap memakai UI eksternal.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// Relay sisi server nonaktif secara default; saat diaktifkan tetap tunduk
	// pada daftar putih SSRF dan batas byte/sesi.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Eksekutor dipilih saat build berdasarkan famili protokol: `go build -tags apidoc_debug_all` mengompilasi semuanya, atau batasi dengan `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial`, dan `apidoc_debug_media`. Panggilan ke protokol yang tidak dikompilasi selalu gagal secara tertutup (fail closed).

### Anotasi Protokol Apa Pun

Antarmuka non-HTTP menggunakan kembali `@method` + `@url` dan menentukan protokol dengan `@protocol` (juga dapat disimpulkan dari skema URL):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Publikasikan suhu
// @param qos int false "QoS" in=option default=1
// @param payload json true "Bodi pesan" in=payload
func PublishTemp() {}
```

`@protocol` menerima daftar protokol lengkap dan aliasnya (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); registri lengkap key, alias, skema, dan port default tersedia di `GET /api/protocols`. Body biner dideklarasikan dengan `@param ... type=binary` (atau `hex`/`base64`) + `in=payload`.

`/api/detail` dan UIC-1 `/ui/v1/docDetail` mengembalikan `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock`, dan `definitions` yang telah di-parse. Debug memiliki dua saluran: langsung dari browser (HTTP / WebSocket / SSE, selalu tersedia) dan relay sisi server (dengan `Debug` diaktifkan plus build tag yang sesuai). Famili protokol sisi server adalah HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), socket (TCP / UDP / Raw / Unix), messaging (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industrial (Modbus / DNP3 / IEC 104 / BACnet / OPC UA), dan media (RTSP / SIP / RTP / RTCP / RTMP). Build default tidak mengompilasi eksekutor apa pun; saat diaktifkan, target harus lebih dulu lolos penjaga SSRF (daftar putih `AllowHosts`, alamat privat/loopback/link-local/metadata cloud ditolak secara default, validasi per-IP setelah resolusi DNS dengan koneksi di-pin), dan dibatasi oleh timeout, batas 1 MiB per pesan, `MaxBytes` respons, serta batas sesi streaming.

### Parsing Definisi Protokol (@definition)

Gunakan `@definition` untuk menarik definisi protokol eksternal (format dideteksi otomatis, atau atur dengan `format=`); hasil parsing dikembalikan bersama detail agar UIC dapat merender struktur pesan dan menyusun formulir debug:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Format yang didukung: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC, dan XML-RPC. `Config.DefinitionRoot` membatasi file yang dapat dibaca ke direktori akar (default: direktori kerja proses); parsing dibatasi 1 MiB dan 50.000 baris per file serta kedalaman referensi bersarang 32; lihat komentar paket `parse` untuk opsi seperti `operation=`, `message=`, `direction=`, `sample=`, dan `inline=`.

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

Untuk Echo / Chi / Fiber cukup ganti konstruktor adapter: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)`, kode lainnya sepenuhnya sama. Untuk beego / Kratos / go-zero, lihat tabel Adapter Kerangka di bawah.

### Opsi Konfigurasi

| Opsi | Nilai default | Deskripsi |
|--------|--------|------|
| `Prefix` | `/apidoc` | Jalur pemasangan dokumentasi |
| `Title` | `API Docs` | Judul dokumentasi |
| `Desc` | — | Deskripsi dokumentasi |
| `GlobalParams` | — | Parameter global, digabungkan ke setiap Action |
| `Auth` | — | Konfigurasi autentikasi: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Konfigurasi aplikasi: `Key` / `Title` / `Password` (kata sandi independen per aplikasi) |
| `DebugOrigins` | — | Sumber yang diizinkan membaca API dokumentasi lintas domain (UI eksternal, default: tidak ada) |
| `PublicHost` | — | URL basis absolut yang dipakai UI eksternal untuk debug; kosong berarti asal yang sama |
| `ShareDir` | `apidoc-go-shares` di direktori temp | Direktori catatan share UIC-1 |
| `DefinitionRoot` | direktori kerja proses | Akar file `@definition`; jalur di luar akar dan symlink ditolak |
| `Generator` / `CodeTemplates` | — | Generator deklaratif dan template kode (rendering tanpa efek samping) |
| `ShareActions` | — | Deklarasi aksi share yang diinterpretasikan oleh UI |
| `Debug` | nilai nol = nonaktif | Eksekutor sisi server: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (tanpa UI bawaan) | Direktori build Vue UIC-1 yang dipaketkan; bila tidak kosong, menyajikan UI statis dalam proses yang sama di `{Prefix}/` |

### API UIC-1 dan UI Vue Bawaan (Opsional)

Pustaka menyajikan dua API JSON dokumentasi secara default: `/api/*` lama dan UIC-1 `/ui/v1/*`, yang dikonsumsi UI eksternal. `GET {Prefix}/ui/v1/capabilities` mengembalikan kontrak, versi backend, dan matriks fitur (mode debug browser/server, share, cache, generator, ekspor, autentikasi); UI memakainya untuk menentukan panel mana yang tersedia. `/ui/v1/config` mengembalikan menu beserta konfigurasi generator dan template kode. Setelah menyerahkan build `apidoc-ui` yang dipaketkan ke `Config.UIPath`, proses yang sama juga menyajikan UI Vue langsung di `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // direktori keluaran scripts/package-ui.sh
})
```

Pustaka itu sendiri tidak menyematkan aset frontend, jadi tidak ada dependensi saat build; paket rilis dengan UI bawaan sebaiknya mengarahkan `UIPath` ke direktori `dist/uic1-ui/` yang disertakan secara default — pemasangan di atas adalah konfigurasi yang direkomendasikan. Langkah deployment:

1. Build dan paketkan di repositori UI: `npm run package:ui` (keluaran `dist/uic1-ui/`).
2. Deploy ke direktori target (direktori harus ada dan kosong, atau berisi paket uic1-ui sebelumnya): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Setel `UIPath` saat startup; biarkan kosong dan mulai ulang untuk kembali ke mode hanya API dokumentasi.

Perilaku:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js`, dan `{Prefix}/assets/*` disajikan sebagai file statis, dengan tipe MIME ditentukan oleh ekstensi;
- jalur tanpa ekstensi yang tidak dikenal jatuh ke `index.html` (fallback SPA); aset yang hilang seperti `.js` / `.css` dan sejenisnya mengembalikan 404;
- `{Prefix}` (tanpa garis miring di akhir) mengalihkan dengan 307 ke `{Prefix}/` agar jalur aset relatif tetap benar;
- `{Prefix}/api/*` dan `{Prefix}/ui/v1/*` diutamakan; jalur API yang tidak dikenal mengembalikan 404 dan tidak pernah jatuh ke UI;
- akses file dibatasi ke `UIPath` (`..` dan symlink di luar akar ditolak), tanpa daftar direktori; jika direktori atau `index.html` tidak ada, rute UI mengembalikan 500 dan API dokumentasi tidak terpengaruh;
- adapter go-zero hanya mendaftarkan rute API tetap dan tidak meneruskan jalur arbitrer di bawah prefix, sehingga UI bawaan tidak tersedia dengan adapter tersebut (semua adapter lain mendukungnya);
- deployment yang masih memakai UI eksternal tidak perlu perubahan; cukup pertahankan `HTTP.API_PREFIX` di `config.js` sama dengan `Prefix` server.

### Eksekutor Protokol Sisi Server (Build Tag)

Relay sisi server tidak mengompilasi eksekutor apa pun secara default, dan memanggil protokol yang tidak dikompilasi mengembalikan error fail closed. Pilih build tag berdasarkan famili protokol; `apidoc_debug_all` menggabungkan semuanya:

| Build tag | Famili protokol | Protokol |
|-----------|-----------------|----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, hanya https) |
| `apidoc_debug_sockets` | Socket | tcp, udp, raw, unix (unary + streaming) |
| `apidoc_debug_rpc` | RPC / finansial | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unary; descriptor atau refleksi server) |
| `apidoc_debug_messaging` | Messaging | mqtt, stomp, amqp, kafka, xmpp (unary + streaming; juga dapat dipilih satu per satu dengan `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industrial | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Media | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Semua | semua famili di atas |

Batas keamanan: `Debug.EnableServerSide` nonaktif secara default; saat diaktifkan, target tetap harus lolos daftar putih `AllowHosts` (pencocokan sufiks seperti `*.example.com`), alamat privat/loopback/link-local/CGNAT dan metadata cloud ditolak secara default (hanya `AllowPrivateNetworks` yang mengizinkannya), validasi per-IP dilakukan setelah resolusi DNS dengan koneksi di-pin, dan `CONNECT`, redirect, serta proxy dilarang; permintaan dan respons dibatasi oleh `TimeoutMS`, `MaxBytes` (1 MiB secara default), sesi streaming (4 per scope), jumlah frame, dan timeout idle. Parsing definisi hanya membaca file reguler di dalam `DefinitionRoot`, dibatasi oleh ukuran file, jumlah baris, dan kedalaman referensi.

### Adapter Kerangka

| Kerangka | Konstruktor |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### Parsing Anotasi Otomatis (go/ast)

Tulis komentar `@apidoc` di atas handler, lalu daftarkan hasil parsing:

```go
// @apidoc
// @method POST
// @url /api/v1/users
// @title Create user
// @param name string true "Username"
// @success ok User "Success"
func CreateUser(c *gin.Context, req *CreateUserReq) { ... }
```

```go
results, err := parse.ParseDir("./handlers")
if err != nil { log.Fatal(err) }
for _, r := range results {
    s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: CreateUser, Doc: r.Doc})
}
```

### Pelengkapan Parameter Otomatis (reflect)

Saat `Doc.Params` kosong, Register menyimpulkannya dengan reflect dari tanda tangan handler: argumen struct diperluas menjadi kolom body (mengikuti tag json), konteks kerangka (gin.Context / echo.Context / fiber.Ctx) dilewati secara otomatis.

### Cache HTTP (cache respons + ETag)

Respons `/api/menus`, `/api/protocols`, dan `/api/export` diserialisasi sekali per versi dokumen dan di-cache di dalam proses. Cache dipanaskan saat `Handler` dibuat dan otomatis dibatalkan oleh `Register` / `SetAppMeta`. Respons tetap membawa `ETag` + `Cache-Control: private, max-age=300`, sehingga kunjungan berulang mendapat 304. Tidak perlu konfigurasi apa pun.

### Ekspor

| Format | Endpoint | Deskripsi |
|--------|----------|-----------|
| JSON | `GET /apidoc/api/export` | Pohon proyek lengkap |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Definisi tipe antarmuka |

### Data Mock

Halaman detail menampilkan contoh Mock secara otomatis: sesuaikan dengan `Doc.Params[].Mock`, atau biarkan dibuat dari tipe kolom (string→"sample", int→0, bool→true, ...).

### Proyek Contoh

`example/` menyediakan 8 server kerangka (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Jalankan semuanya dengan `go run ./example`; API dokumentasi lama tersedia di `/apidoc/api/...` pada setiap port, API UIC-1 di `/apidoc/ui/v1/...`, dan baik UI eksternal maupun UI Vue bawaan via `UIPath` dapat mengonsumsinya.

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
