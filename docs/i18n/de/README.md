![Apidoc-Maskottchen](../../svg/apidoc-pet.svg)

# apidoc-go — Allgemeines API-Dokumentations-Plugin für Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Projektbeschreibung

**Aktuelle Version: v1.1.1** (siehe [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** ist eine universelle API-Dokumentations-Plugin-Bibliothek für Go: Die Schnittstellendokumentation wird als **typisierte Strukturen** zusammen mit der Routenregistrierung deklariert — Dokumentation und Route entstehen Seite an Seite. Die Dokumentation wird sowohl über die Legacy-JSON-API `/api/*` als auch über die UIC-1-JSON-API `/ui/v1/*` bereitgestellt, sodass entweder jede externe UI sie rendern und Testanfragen ausführen kann — oder eine gepackte Vue-UI über `Config.UIPath` im selben Prozess ausgeliefert wird. Enthalten: Passwort-Authentifizierung, Verwaltung mehrerer Anwendungen/Versionen, Mock-Daten, JSON-/TypeScript-Export, Parsing von Kommentaren und Protokolldefinitionen in neun Formaten, Response-Caching und automatische Parametervervollständigung; serverseitige Protokoll-Executoren werden per Build-Tag bei Bedarf kompiliert und decken die Protokollfamilien HTTP, RPC, Sockets, Messaging, IoT, Industrie und Medien ab. Einmal integriert, mit allen Frameworks kompatibel — ohne Umbau bestehender Projekte.

## Funktionen

| # | Funktion | Beschreibung |
|---|----------|--------------|
| 1 | Automatische Dokumentationserstellung | Typisierte Doc-Deklaration zusammen mit der Routenregistrierung — einmal deklariert, entstehen Dokumentation und Route gemeinsam |
| 2 | Dokumentations-API | Legacy-`/api/*` (Menus / Protocols / Detail / Export) und UIC-1-`/ui/v1/*` (Docs, Debug, Shares, Generatoren, Export) — für externe UIs oder die mitgelieferte Vue-UI |
| 3 | Mock-Daten | Feldgenaue Mock-Beispiele, schneller bei der Schnittstellen-Integration |
| 4 | Mehrere Apps / Versionen | Baumartige Verwaltung von App / Version — ein Plugin deckt die gesamte Projektdokumentation ab |
| 5 | Passwort-Authentifizierung | Globales Passwort + app-spezifisches Passwort, HMAC-Token · zeitkonstanter Vergleich |
| 6 | Markdown-Dokumentation | Sichere goldmark-Renderung, natives HTML wird automatisch entfernt |
| 7 | Multi-Framework-Anpassung | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero — einmal integriert, mit allen Frameworks kompatibel |
| 8 | JSON-/TypeScript-Export | Schnittstellentypen mit einem Klick exportieren, reibungslosere Frontend-Backend-Integration |
| 9 | Sicherheitsschutz | Passwort-Authentifizierung · CORS-Whitelist-Einschränkung · XSS- / Pfad-Traversal-Schutz · SSRF-Schutz und Session-/Byte-Limits für serverseitiges Debugging |
| 10 | Annotation- und Definitions-Parsing | go/ast parst `@apidoc` / `@protocol` / `@definition`; neun integrierte Definitionsformate: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC und XML-RPC |
| 11 | HTTP-Caching | Versionsbezogener Response-Cache + ETag/304, Doc-Endpunkte antworten in Millisekunden |
| 12 | Automatische Parametervervollständigung | reflect leitet die Request-Parameter aus der Handler-Signatur ab |
| 13 | Annotationen für beliebige Protokolle | `@protocol` legt das Protokoll fest; Detail liefert protocol/method/url/params/responses/mock sowie geparste Protokolldefinitionen |
| 14 | Mitgelieferte Vue-UI (optional) | Richten Sie `Config.UIPath` auf einen gepackten UIC-1-Build, um die Vue-UI im selben Prozess auszuliefern; ein leerer Wert fällt auf die JSON-API zurück |
| 15 | Serverseitige Protokoll-Executoren | Sieben per Build-Tag kompilierte Protokollfamilien; standardmäßig deaktiviert und fail closed, wenn nicht kompiliert, wobei `apidoc_debug_all` alles aktiviert |

## Architekturübersicht

![Projektarchitektur](../../svg/architecture.svg)

![Projektfunktionen](../../svg/features.svg)

![Projektlebenszyklus](../../svg/lifecycle.svg)

## Projektstruktur

```
apidoc-go/
├── apidoc.go            # Zentrale Einstiegspunkte: New / Register / Mount / Handler
├── VERSION              # Projektversion (v1.1.1); der Release-Workflow taggt danach
├── doc.go               # Route / Doc / Param / Response-Typen
├── model/               # Datenmodelle
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Externe Protokolldefinitionen über @definition
│   ├── protocol.go      #   URL-Scheme → Protokoll
│   └── protocols.go     #   Protokoll-Registry / Aliase / Standardports
├── store/               # Speicherung und Deduplizierung
│   └── store.go         #   (app, version, method, url) Deduplizierung, spätere überschreiben
├── auth/                # Authentifizierung
│   └── auth.go          #   Passwortprüfung · Ausstellung und Validierung von HMAC-Tokens
├── server/              # HTTP-API
│   ├── server.go        #   Legacy-Routen /api · Auth / CORS / Caching
│   ├── ui.go            #   Statische Vue-UI aus UIPath (SPA-Fallback · auf den Pfad beschränkt)
│   └── uiapi/           #   UIC-1-API /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     Dokument-Lesen · Definitions-Parsing
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     Generatoren · Code-Templates · Shares · Cache
│       ├── share.go
│       └── debug/       #     serverseitige Protokoll-Executoren (Build-Tag pro Familie)
├── adapter/             # Framework-Adapter-Schicht (typisierte Schnittstellen)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Annotation-Auto-Parser
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   neun Parser für Protokolldefinitionen
├── export/              # Exporte
│   └── export.go        #   TypeScript-Interface-Definitionen
├── mock/                # Mock-Daten
│   └── mock.go          #   Feldgenaue Beispielgenerierung
├── example/             # Beispielprojekt (8 Frameworks :8081–:8088)
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
	// Externe UIs lesen /apidoc/api/menus oder UIC-1 /apidoc/ui/v1/*;
	// mit gesetztem UIPath direkt http://localhost:8080/apidoc/ öffnen
}
```

### Aktivieren der mitgelieferten UIC-1-Vue-UI und der serverseitigen Executoren

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Optional: die Ausgabe von npm run package:ui aus apidoc-ui
	// (index.html / config.js / assets/).
	// Leer lassen, um nur die JSON-API auszuliefern und eine externe UI weiterzuverwenden.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// Server-seitiges Relay ist standardmäßig aus; wenn aktiviert, gilt weiterhin
	// die SSRF-Whitelist sowie Byte- und Session-Limits.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Executoren werden zur Build-Zeit nach Protokollfamilie ausgewählt: `go build -tags apidoc_debug_all` kompiliert alle, oder begrenzen Sie die Auswahl mit `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` und `apidoc_debug_media`. Aufrufe von Protokollen ohne passenden Build-Tag schlagen immer fail closed fehl.

### Annotationen für beliebige Protokolle

Nicht-HTTP-Schnittstellen verwenden weiterhin `@method` + `@url` und geben das Protokoll mit `@protocol` an (es kann auch aus dem URL-Schema abgeleitet werden):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Temperatur veröffentlichen
// @param qos int false "QoS" in=option default=1
// @param payload json true "Nachrichtentext" in=payload
func PublishTemp() {}
```

`@protocol` akzeptiert die vollständige Protokollliste und ihre Aliase (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); die vollständige Registry aus Key, Alias, Scheme und Standardport finden Sie unter `GET /api/protocols`. Binäre Bodies werden mit `@param ... type=binary` (oder `hex`/`base64`) + `in=payload` deklariert.

`/api/detail` und UIC-1 `/ui/v1/docDetail` liefern `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` und die geparsten `definitions`.

Für das Debugging gibt es zwei Kanäle: direkt aus dem Browser (HTTP / WebSocket / SSE, immer verfügbar) und über ein serverseitiges Relay (mit aktiviertem `Debug` und passendem Build-Tag). Die serverseitigen Protokollfamilien sind HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), Sockets (TCP / UDP / Raw / Unix), Messaging (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), Industrie (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) und Medien (RTSP / SIP / RTP / RTCP / RTMP). Standardmäßig wird kein Executor kompiliert; wenn aktiviert, müssen Ziele zuerst den SSRF-Schutz passieren (Whitelist `AllowHosts`, private/loopback/link-local/Cloud-Metadata-Adressen standardmäßig verweigert, Validierung pro IP nach DNS-Auflösung mit gepinnter Verbindung) und sind durch Timeouts, ein Limit von 1 MiB pro Nachricht, das Response-`MaxBytes` und Stream-Session-Limits begrenzt.

### Parsing von Protokolldefinitionen (@definition)

Verwenden Sie `@definition`, um externe Protokolldefinitionen einzubinden (das Format wird automatisch erkannt oder mit `format=` gesetzt); das geparste Ergebnis wird mit dem Detail zurückgeliefert, damit die UIC Nachrichtenstrukturen rendern und Debug-Formulare aufbauen kann:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Unterstützte Formate: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC und XML-RPC. `Config.DefinitionRoot` beschränkt lesbare Dateien auf ein Wurzelverzeichnis (standardmäßig das Arbeitsverzeichnis des Prozesses); das Parsing ist auf 1 MiB und 50.000 Zeilen pro Datei sowie eine Verschachtelungstiefe von 32 beschränkt; Optionen wie `operation=`, `message=`, `direction=`, `sample=` und `inline=` sind in den Kommentaren des `parse`-Pakets beschrieben.

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

Bei Echo / Chi / Fiber muss lediglich der Adapter-Konstruktor ausgetauscht werden: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — der übrige Code bleibt vollständig identisch. Für beego / Kratos / go-zero siehe die Tabelle „Framework-Adapter“ unten.

### Konfigurationsoptionen

| Option | Standardwert | Beschreibung |
|--------|--------------|--------------|
| `Prefix` | `/apidoc` | Pfad, unter dem die Dokumentation bereitgestellt wird |
| `Title` | `API Docs` | Titel der Dokumentation |
| `Desc` | — | Beschreibung der Dokumentation |
| `GlobalParams` | — | Globale Parameter, werden in jede Action übernommen |
| `Auth` | — | Auth-Konfiguration: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | App-Konfiguration: `Key` / `Title` / `Password` (unabhängiges Passwort pro App) |
| `DebugOrigins` | — | Quellen, die die Dokumentations-API cross-origin lesen dürfen (externe UI, standardmäßig keine) |
| `PublicHost` | — | Absolute Basis-URL, die externe UIs zum Debuggen verwenden; leer bedeutet Same-Origin |
| `ShareDir` | `apidoc-go-shares` im Temp-Verzeichnis | Verzeichnis für UIC-1-Share-Einträge |
| `DefinitionRoot` | Arbeitsverzeichnis des Prozesses | Wurzel für `@definition`-Dateien; Pfade außerhalb der Wurzel und Symlinks werden abgelehnt |
| `Generator` / `CodeTemplates` | — | Deklarative Generatoren und Code-Templates (das Rendern ist seiteneffektfrei) |
| `ShareActions` | — | Share-Action-Deklarationen, die von der UI interpretiert werden |
| `Debug` | Nullwert = aus | Serverseitige Executoren: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (keine mitgelieferte UI) | Verzeichnis eines gepackten UIC-1-Vue-Builds; wenn nicht leer, wird die statische UI im selben Prozess unter `{Prefix}/` ausgeliefert |

### UIC-1-API und mitgelieferte Vue-UI (optional)

Die Bibliothek stellt standardmäßig zwei JSON-Dokumentations-APIs bereit: Legacy-`/api/*` und UIC-1-`/ui/v1/*`, die von einer externen UI genutzt werden. `GET {Prefix}/ui/v1/capabilities` liefert den Vertrag, die Backend-Version und die Feature-Matrix (Browser-/Server-Debug-Modi, Shares, Cache, Generatoren, Export, Auth); die UI entscheidet damit, welche Panels verfügbar sind. `/ui/v1/config` liefert die Menüs sowie die Generator- und Code-Template-Konfiguration. Nachdem ein gepackter `apidoc-ui`-Build an `Config.UIPath` übergeben wurde, liefert derselbe Prozess die Vue-UI zusätzlich direkt unter `{Prefix}/` aus:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // Ausgabeverzeichnis von scripts/package-ui.sh
})
```

Die Bibliothek selbst bettet keine Frontend-Assets ein, daher gibt es keine Build-Zeit-Abhängigkeit; ein Release-Paket mit mitgelieferter UI sollte `UIPath` standardmäßig auf das mitgelieferte Verzeichnis `dist/uic1-ui/` zeigen lassen — die obige Verdrahtung ist das empfohlene Setup. Bereitstellungsschritte:

1. Im UI-Repository bauen und paketieren: `npm run package:ui` (Ausgabe `dist/uic1-ui/`).
2. In das Zielverzeichnis deployen (es muss existieren und leer sein oder das vorherige uic1-ui-Paket enthalten): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. `UIPath` beim Start setzen; leer lassen und neu starten, um auf den reinen Dokumentations-API-Modus zurückzurollen.

Verhalten:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` und `{Prefix}/assets/*` werden als statische Dateien ausgeliefert, der MIME-Typ wird anhand der Erweiterung bestimmt;
- unbekannte Pfade ohne Erweiterung fallen auf `index.html` zurück (SPA-Fallback); fehlende `.js` / `.css` und ähnliche Assets liefern 404;
- `{Prefix}` (ohne abschließenden Schrägstrich) leitet mit 307 auf `{Prefix}/` um, damit relative Asset-Pfade korrekt bleiben;
- `{Prefix}/api/*` und `{Prefix}/ui/v1/*` haben Vorrang; unbekannte API-Pfade liefern 404 und fallen niemals auf die UI zurück;
- der Dateizugriff ist auf `UIPath` beschränkt (`..` und Symlinks außerhalb der Wurzel werden abgelehnt), ohne Verzeichnislisting; fehlt das Verzeichnis oder `index.html`, liefert die UI-Route 500 und die Dokumentations-API bleibt unberührt;
- der go-zero-Adapter registriert nur die festen API-Routen und leitet keine beliebigen Pfade unter dem Prefix weiter, daher ist die mitgelieferte UI mit diesem Adapter nicht verfügbar (alle anderen Adapter unterstützen sie);
- Bereitstellungen, die weiterhin eine externe UI verwenden, benötigen keine Änderungen; halten Sie einfach `HTTP.API_PREFIX` in `config.js` gleich dem Server-`Prefix`.

### Serverseitige Protokoll-Executoren (Build-Tags)

Standardmäßig wird im serverseitigen Relay kein Executor kompiliert, und der Aufruf eines Protokolls ohne passenden Build-Tag liefert einen fail-closed-Fehler. Wählen Sie Build-Tags nach Protokollfamilie; `apidoc_debug_all` aggregiert alle:

| Build-Tag | Protokollfamilie | Protokolle |
|-----------|------------------|------------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, nur https) |
| `apidoc_debug_sockets` | Sockets | tcp, udp, raw, unix (Unary + Streaming) |
| `apidoc_debug_rpc` | RPC / Finanzen | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (Unary; Deskriptor oder Server-Reflection) |
| `apidoc_debug_messaging` | Messaging | mqtt, stomp, amqp, kafka, xmpp (Unary + Streaming; auch einzeln wählbar mit `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industrie | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Medien | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Alle | alle oben genannten Familien |

Sicherheitsgrenze: `Debug.EnableServerSide` ist standardmäßig aus; wenn aktiviert, müssen Ziele weiterhin die `AllowHosts`-Whitelist passieren (Suffix-Matching wie `*.example.com`), private/loopback/link-local/CGNAT- und Cloud-Metadata-Adressen sind standardmäßig verweigert (nur `AllowPrivateNetworks` erlaubt sie), die Validierung pro IP erfolgt nach der DNS-Auflösung mit gepinnter Verbindung, und `CONNECT`, Redirects und Proxies sind verboten; Anfragen und Antworten sind durch `TimeoutMS`, `MaxBytes` (standardmäßig 1 MiB), Stream-Sessions (4 pro Scope), Frame-Anzahlen und Idle-Timeouts begrenzt. Das Definitions-Parsing liest nur reguläre Dateien innerhalb von `DefinitionRoot`, begrenzt durch Dateigröße, Zeilenanzahl und Referenztiefe.

### Framework-Adapter

| Framework | Konstruktor |
|-----------|-------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

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

### HTTP-Caching (Response-Cache + ETag)
Antworten für `/api/menus`, `/api/protocols` und `/api/export` werden pro Dokumentversion einmal serialisiert und im Prozess zwischengespeichert. Der Cache wird beim Erstellen des `Handler` vorgewärmt und durch `Register` / `SetAppMeta` automatisch invalidiert. Antworten tragen weiterhin `ETag` + `Cache-Control: private, max-age=300`; wiederholte Aufrufe erhalten 304. Keine Konfiguration erforderlich.

### Export
| Format | Endpunkt | Beschreibung |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Vollständiger Projektbaum |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Interface-Typdefinitionen |

### Mock-Daten
Die Detailseite zeigt automatisch ein Mock-Beispiel an: Passen Sie es mit `Doc.Params[].Mock` an oder lassen Sie es aus dem Feldtyp generieren (string→"sample", int→0, bool→true, ...).

### Beispielprojekt
`example/` enthält 8 Framework-Server (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Starten Sie alle mit `go run ./example`; die Legacy-Dokumentations-API liegt auf jedem Port unter `/apidoc/api/...`, die UIC-1-API unter `/apidoc/ui/v1/...`, und konsumiert werden sie entweder von einer externen UI oder von der mitgelieferten Vue-UI über `UIPath`.

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
