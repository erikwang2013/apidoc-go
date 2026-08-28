# apidoc-go — Allgemeines API-Dokumentations-Plugin für Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Projektbeschreibung

**apidoc-go** ist eine universelle API-Dokumentations-Plugin-Bibliothek für Go: Die Schnittstellendokumentation wird als **typisierte Strukturen** zusammen mit der Routenregistrierung deklariert — Dokumentation und Route entstehen Seite an Seite. Die eingebettete Web-UI bietet Online-Anzeige und Online-Debugging und umfasst Passwort-Authentifizierung, Verwaltung mehrerer Anwendungen/Versionen, Mock-Daten sowie JSON-/TypeScript-Export, Annotation-Auto-Parsing, HTTP-Caching und automatische Parametervervollständigung. Einmal integriert, mit allen Frameworks kompatibel — ohne Umbau bestehender Projekte.

## Funktionen

| # | Funktion | Beschreibung |
|---|----------|--------------|
| 1 | Automatische Dokumentationserstellung | Typisierte Doc-Deklaration zusammen mit der Routenregistrierung — einmal deklariert, entstehen Dokumentation und Route gemeinsam |
| 2 | Online-Debugging | Echte Endpunkte direkt aus dem Browser aufrufen, keine serverseitige Weiterleitung, von Natur aus SSRF-frei |
| 3 | Mock-Daten | Feldgenaue Mock-Beispiele, schneller bei der Schnittstellen-Integration |
| 4 | Mehrere Apps / Versionen | Baumartige Verwaltung von App / Version — ein Plugin deckt die gesamte Projektdokumentation ab |
| 5 | Passwort-Authentifizierung | Globales Passwort + app-spezifisches Passwort, HMAC-Token · zeitkonstanter Vergleich |
| 6 | Markdown-Dokumentation | Sichere goldmark-Renderung, natives HTML wird automatisch entfernt |
| 7 | Multi-Framework-Anpassung | net/http · Gin · Echo · Chi · Fiber — einmal integriert, mit allen Frameworks kompatibel |
| 8 | JSON-/TypeScript-Export | Schnittstellentypen mit einem Klick exportieren, reibungslosere Frontend-Backend-Integration |
| 9 | Sicherheitsschutz | Kein SSRF · CORS-Whitelist-Einschränkung · XSS-Schutz · Schutz vor Pfad-Traversal |
| 10 | Annotation-Auto-Parsing | go/ast erzeugt die Dokumentation aus Kommentaren; der Marker `@apidoc` genügt |
| 11 | HTTP-Caching | ETag + 304, Doc-Antworten öffnen sofort |
| 12 | Automatische Parametervervollständigung | reflect leitet die Request-Parameter aus der Handler-Signatur ab |

## Architekturübersicht

![Projektarchitektur](../../svg/architecture.svg)

![Projektfunktionen](../../svg/features.svg)

![Projektlebenszyklus](../../svg/lifecycle.svg)

## Projektstruktur

```
apidoc-go/
├── apidoc.go            # Zentrale Einstiegspunkte: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response-Typen
├── model/               # Datenmodelle
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Speicherung und Deduplizierung
│   └── store.go         #   (app, version, method, url) Deduplizierung, spätere überschreiben
├── auth/                # Authentifizierung
│   └── auth.go          #   Passwortprüfung · Ausstellung und Validierung von HMAC-Tokens
├── server/              # JSON-API + eingebettete Web-UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Framework-Adapter-Schicht (typisierte Schnittstellen)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Annotation-Auto-Parser
│   └── parse.go         #   go/ast · @apidoc-Kommentarmarker
├── export/              # Exporte
│   └── export.go        #   TypeScript-Interface-Definitionen
├── mock/                # Mock-Daten
│   └── mock.go          #   Feldgenaue Beispielgenerierung
├── example/             # Beispielprojekt (5 Frameworks :8081–:8085)
│   ├── main.go
│   └── handlers/        #   @apidoc-Kommentarbeispiele
└── docs/                # Dokumentation und Ressourcen
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Architekturdiagramm / Funktionsdiagramm / Lebenszyklusdiagramm
```

## Verwendung

### Installation

```bash
go get github.com/erikwang2013/apidoc-go
```

### Schnellstart (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "Meine API-Dokumentation"})

	// Dokumentation und Route gemeinsam deklarieren
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
			Title:   "Begrüßung",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "Dein Name"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Im Browser http://localhost:8080/apidoc aufrufen
}
```

### Schnellstart (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "Meine API-Dokumentation"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "Begrüßung"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Bei Echo / Chi / Fiber muss lediglich der Adapter-Konstruktor ausgetauscht werden: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — der übrige Code bleibt vollständig identisch.

### Konfigurationsoptionen

| Option | Standardwert | Beschreibung |
|--------|--------------|--------------|
| `Prefix` | `/apidoc` | Pfad, unter dem die Dokumentation bereitgestellt wird |
| `Title` | `API Docs` | Titel der Dokumentation |
| `Desc` | — | Beschreibung der Dokumentation |
| `GlobalParams` | — | Globale Parameter, werden in jede Action übernommen |
| `Auth` | — | Auth-Konfiguration: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | App-Konfiguration: `Key` / `Title` / `Password` (unabhängiges Passwort pro App) |
| `DebugOrigins` | — | Für Debugging erlaubte Cross-Origin-Quellen (standardmäßig keine) |

### Framework-Adapter

| Framework | Konstruktor |
|-----------|-------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

### Annotation-Auto-Parsing (go/ast)
Schreiben Sie `@apidoc`-Kommentare über einen Handler und registrieren Sie anschließend die Parsing-Ergebnisse:

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

### Automatische Parametervervollständigung (reflect)
Wenn `Doc.Params` leer ist, leitet Register die Parameter per reflect aus der Handler-Signatur ab: Struktur-Argumente werden zu Body-Feldern (gemäß json-Tags), Framework-Kontexte (gin.Context / echo.Context / fiber.Ctx) werden automatisch übersprungen.

### HTTP-Caching (ETag)
Alle Doc-Endpunkte senden automatisch `ETag` + `Cache-Control: private, max-age=300`; wiederholte Aufrufe erhalten 304. Keine Konfiguration erforderlich.

### Export
| Format | Endpunkt | Beschreibung |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Vollständiger Projektbaum |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Interface-Typdefinitionen |

### Mock-Daten
Die Detailseite zeigt automatisch ein Mock-Beispiel an: Passen Sie es mit `Doc.Params[].Mock` an oder lassen Sie es aus dem Feldtyp generieren (string→"sample", int→0, bool→true, ...).

### Beispielprojekt
`example/` enthält 5 Framework-Server (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085). Starten Sie alle mit `go run ./example`.

## Mehrsprachige Dokumentation

| Sprache | Link |
|---------|------|
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

## Unterstützen Sie uns

Wenn Ihnen dieses Projekt geholfen hat, können Sie uns gerne per QR-Code-Scan eine kleine Spende zukommen lassen — das motiviert uns, das Projekt weiter zu pflegen und zu aktualisieren!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat-Pay-Spende" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay-Spende" />
  <p>WeChat Pay (links) · Alipay (rechts)</p>
</div>

### Spende per internationaler Überweisung

**【Empfängerinformationen】**
Empfängername: WANG KEXUN
Kontonummer: 881015918251

**【Empfängerbank】**
ZA Bank
SWIFT-Code: AABLHKHHXXX
Bankname: ZA Bank Limited
Bankleitzahl: 387
Bankadresse: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Korrespondenzbank für grenzüberschreitende Überweisungen (falls erforderlich)】**

Bitte beachten Sie: Hierbei handelt es sich um die Informationen der Korrespondenzbank (Zwischenbank) für grenzüberschreitende Überweisungen, nicht um die der Empfängerbank. Fragen Sie Ihre überweisende Bank, ob die Angaben zur Korrespondenzbank benötigt werden.

Die Korrespondenzbank für Überweisungen in Hongkong-Dollar, Chinesischen Yuan (CNY) und US-Dollar ist Citibank —

Bankname: Citibank N.A. Hong Kong
SWIFT-Code: CITIHKHXXXX
Bankleitzahl: 006
Filialname: Hong Kong Branch
Filialcode: 391
Bankadresse: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Die Korrespondenzbank für Überweisungen in anderen Währungen ist BNY Mellon —

Bankname: THE BANK OF NEW YORK MELLON
SWIFT-Code: IRVTUS3NXXX
Bankadresse: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Mit ❤️ erstellt von <a href="https://erik.xyz">erik.xyz</a></div>
