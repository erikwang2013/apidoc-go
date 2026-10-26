![Mascota de Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — Plugin universal de documentación de API para Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Descripción del proyecto

**Versión actual: v1.1.1** (consulta [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** es una librería de plugin de documentación de API universal para Go: la documentación de las interfaces se declara como **structs tipados** junto con el registro de las rutas, de modo que la documentación nace al mismo tiempo que la ruta. La documentación se sirve tanto por la API JSON heredada `/api/*` como por la API UIC-1 `/ui/v1/*`, de modo que cualquier interfaz externa puede renderizarla y enviar peticiones de prueba — o se puede servir una interfaz Vue empaquetada en el mismo proceso mediante `Config.UIPath`. Incluye: autenticación por contraseña, gestión de múltiples aplicaciones/versiones, datos Mock, exportación a JSON / TypeScript, análisis de comentarios y de definiciones de protocolo en nueve formatos, caché de respuestas y autocompletado automático de parámetros; los ejecutores de protocolo del lado del servidor se compilan bajo demanda por build tag y cubren las familias de protocolos HTTP, RPC, socket, mensajería, IoT, industrial y medios. Integra una sola vez y funcionará con todos los frameworks, sin necesidad de modificar tu proyecto existente.

## Funcionalidades

| # | Función | Descripción |
|---|---------|-------------|
| 1 | Generación automática de documentación | Declaración tipada de Doc junto con el registro de la ruta; declara una sola vez y la documentación nace junto con la ruta |
| 2 | API de documentación | API heredada `/api/*` (menús / protocolos / detail / export) y API UIC-1 `/ui/v1/*` (docs, debug, recursos compartidos, generadores, export), para interfaces externas o la interfaz Vue integrada |
| 3 | Datos Mock | Ejemplos Mock a nivel de campo, adelántate en la integración de interfaces |
| 4 | Múltiples aplicaciones / versiones | Gestión jerárquica de App / Version, un solo plugin cubre la documentación de todo el proyecto |
| 5 | Autenticación por contraseña | Contraseña global + contraseña por aplicación, HMAC Token · comparación en tiempo constante |
| 6 | Documentos Markdown | Renderizado seguro con goldmark, el HTML nativo se elimina automáticamente |
| 7 | Adaptación a múltiples frameworks | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, integra una vez y funcionará con todos los frameworks |
| 8 | Exportación JSON / TypeScript | Exporta los tipos de las interfaces con un clic, integración frontend-backend más fluida |
| 9 | Protección de seguridad | Autenticación por contraseña · lista blanca CORS · protección XSS / contra path traversal · guarda SSRF y límites de sesión/bytes para la depuración del lado del servidor |
| 10 | Análisis de anotaciones y definiciones | go/ast analiza `@apidoc` / `@protocol` / `@definition`; nueve formatos de definición integrados: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC y XML-RPC |
| 11 | Caché HTTP | Caché de respuestas versionado + ETag/304, los endpoints de documentación responden en milisegundos |
| 12 | Autocompletado automático de parámetros | reflect deduce los parámetros de la petición a partir de la firma del handler |
| 13 | Anotaciones para cualquier protocolo | `@protocol` indica el protocolo; detail devuelve protocol/method/url/params/responses/mock además de las definiciones de protocolo analizadas |
| 14 | Interfaz Vue integrada (opcional) | Apunta `Config.UIPath` a un build empaquetado de UIC-1 para servir la interfaz Vue en el mismo proceso; un valor vacío recurre a la API JSON |
| 15 | Ejecutores de protocolo del lado del servidor | Siete familias de protocolos compiladas por build tag; los ejecutores están desactivados por defecto y fallan de forma cerrada cuando no se compilan, y `apidoc_debug_all` lo habilita todo |

## Vista general de la arquitectura

![Arquitectura del proyecto](../../svg/architecture.svg)

![Funcionalidades del proyecto](../../svg/features.svg)

![Ciclo de vida del proyecto](../../svg/lifecycle.svg)

## Estructura del proyecto

```
apidoc-go/
├── apidoc.go            # Punto de entrada principal: New / Register / Mount / Handler
├── VERSION              # Versión del proyecto (v1.1.1); el workflow de release crea el tag a partir de ella
├── doc.go               # Tipos Route / Doc / Param / Response
├── model/               # Modelo de datos
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Definiciones de protocolo externas @definition
│   ├── protocol.go      #   esquema de URL → protocolo
│   └── protocols.go     #   registro de protocolos / alias / puertos predeterminados
├── store/               # Almacenamiento y deduplicación
│   └── store.go         #   Deduplicación por (app, version, method, url); la última sobrescribe
├── auth/                # Autenticación
│   └── auth.go          #   Validación de contraseña · emisión y verificación de HMAC Token
├── server/              # API HTTP
│   ├── server.go        #   rutas heredadas /api · auth / CORS / caché
│   ├── ui.go            #   interfaz Vue estática UIPath (fallback SPA · acceso confinado a la ruta)
│   └── uiapi/           #   API UIC-1 /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     lecturas de documentos · análisis de definiciones
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generadores · plantillas de código · recursos compartidos · caché
│       ├── share.go
│       └── debug/       #     ejecutores de protocolo del lado del servidor (build tag por familia)
├── adapter/             # Capa de adaptación de frameworks (interfaz tipada)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Analizador automático de anotaciones
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   nueve analizadores de definiciones de protocolo
├── export/              # Exportaciones
│   └── export.go        #   Definiciones de interfaces TypeScript
├── mock/                # Datos Mock
│   └── mock.go          #   Generación de ejemplos a nivel de campo
├── example/             # Proyecto de ejemplo (8 frameworks :8081–:8088)
│   ├── main.go
│   └── handlers/        #   Ejemplos de comentarios @apidoc
└── docs/                # Documentación y recursos
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Diagramas: arquitectura / funcionalidades / ciclo de vida
```

## Guía de uso

### Instalación

```bash
go get github.com/erikwang2013/apidoc-go
```

### Inicio rápido (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "Mi documentación de API"})

	// La documentación se declara junto con la ruta
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
			Title:   "Saludo",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "Tu nombre"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Las interfaces externas leen /apidoc/api/menus o la API UIC-1 /apidoc/ui/v1/*;
	// con UIPath definido, abre directamente http://localhost:8080/apidoc/
}
```

### Activación de la interfaz Vue integrada UIC-1 y de los ejecutores del lado del servidor

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Opcional: la salida de npm run package:ui de apidoc-ui
	// (index.html / config.js / assets/).
	// Déjalo vacío para servir solo la API JSON y seguir usando una interfaz externa.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// El relay del lado del servidor está desactivado por defecto; al activarlo sigue sujeto
	// a la lista blanca SSRF y a los límites de bytes/sesión.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Los ejecutores se seleccionan en tiempo de compilación por familia de protocolos: `go build -tags apidoc_debug_all` los compila todos, o recórtalos con `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` y `apidoc_debug_media`. Las llamadas a protocolos que no se compilaron siempre fallan de forma cerrada.

### Anotaciones para cualquier protocolo

Las interfaces que no son HTTP reutilizan `@method` + `@url` e indican el protocolo con `@protocol` (también puede inferirse del esquema de la URL):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Publicar temperatura
// @param qos int false "QoS" in=option default=1
// @param payload json true "Cuerpo del mensaje" in=payload
func PublishTemp() {}
```

`@protocol` acepta la lista completa de protocolos y sus alias (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); el registro completo de clave, alias, esquema y puerto predeterminado está en `GET /api/protocols`. Los cuerpos binarios se declaran con `@param ... type=binary` (o `hex`/`base64`) + `in=payload`.

`/api/detail` y la API UIC-1 `/ui/v1/docDetail` devuelven `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` y las `definitions` analizadas. La depuración tiene dos canales: directamente desde el navegador (HTTP / WebSocket / SSE, siempre disponible) y el relay del lado del servidor (con `Debug` activado y el build tag correspondiente). Las familias de protocolos del lado del servidor son HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), sockets (TCP / UDP / Raw / Unix), mensajería (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industrial (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) y medios (RTSP / SIP / RTP / RTCP / RTMP). Las compilaciones por defecto no compilan ningún ejecutor; cuando se activan, los destinos deben pasar primero la guarda SSRF (lista blanca `AllowHosts`, direcciones privadas/loopback/link-local/metadatos de nube denegadas por defecto, validación por IP tras la resolución DNS con la conexión fijada), y están limitados por timeouts, un límite de 1 MiB por mensaje, el `MaxBytes` de la respuesta y límites de sesiones de flujo.

### Análisis de definiciones de protocolo (@definition)

Usa `@definition` para importar definiciones de protocolo externas (el formato se detecta automáticamente, o fíjalo con `format=`); el resultado analizado se devuelve con detail para que la UIC pueda renderizar las estructuras de mensajes y construir los formularios de depuración:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Formatos compatibles: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC y XML-RPC. `Config.DefinitionRoot` confina los archivos legibles a un directorio raíz (el directorio de trabajo del proceso por defecto); el análisis se limita a 1 MiB y 50.000 líneas por archivo, y a una profundidad de referencias anidadas de 32; consulta los comentarios del paquete `parse` para opciones como `operation=`, `message=`, `direction=`, `sample=` e `inline=`.

### Inicio rápido (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "Mi documentación de API"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "Saludo"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Para Echo / Chi / Fiber solo hay que cambiar el constructor del adaptador: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)`; el resto del código es idéntico. Para beego / Kratos / go-zero, consulta la tabla «Adaptadores de frameworks» más abajo.

### Opciones de configuración

| Opción | Valor por defecto | Descripción |
|--------|-------------------|-------------|
| `Prefix` | `/apidoc` | Ruta de montaje de la documentación |
| `Title` | `API Docs` | Título de la documentación |
| `Desc` | — | Descripción de la documentación |
| `GlobalParams` | — | Parámetros globales, fusionados en cada Action |
| `Auth` | — | Configuración de autenticación: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Configuración de aplicaciones: `Key` / `Title` / `Password` (contraseña independiente por aplicación) |
| `DebugOrigins` | — | Orígenes permitidos para leer la API de documentación entre dominios (interfaz externa, ninguno por defecto) |
| `PublicHost` | — | URL base absoluta que usan las interfaces externas para depurar; vacío significa mismo origen |
| `ShareDir` | `apidoc-go-shares` en el directorio temporal | Directorio de registros de recursos compartidos UIC-1 |
| `DefinitionRoot` | directorio de trabajo del proceso | Raíz de archivos `@definition`; las rutas fuera de la raíz y los enlaces simbólicos se rechazan |
| `Generator` / `CodeTemplates` | — | Generadores declarativos y plantillas de código (el renderizado no tiene efectos secundarios) |
| `ShareActions` | — | Declaraciones de acciones de compartir interpretadas por la interfaz |
| `Debug` | valor cero = desactivado | Ejecutores del lado del servidor: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (sin interfaz integrada) | Directorio de un build Vue UIC-1 empaquetado; si no está vacío, sirve la interfaz estática en el mismo proceso bajo `{Prefix}/` |

### API UIC-1 e interfaz Vue integrada (opcional)

La librería sirve por defecto dos API JSON de documentación: la heredada `/api/*` y la UIC-1 `/ui/v1/*`, consumidas por una interfaz externa. `GET {Prefix}/ui/v1/capabilities` devuelve el contrato, la versión del backend y la matriz de funcionalidades (modos de depuración navegador/servidor, recursos compartidos, caché, generadores, export, auth); la interfaz lo usa para decidir qué paneles están disponibles. `/ui/v1/config` devuelve los menús junto con la configuración de generadores y plantillas de código. Tras entregar un build empaquetado de `apidoc-ui` a `Config.UIPath`, el mismo proceso también sirve directamente la interfaz Vue bajo `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // directorio de salida de scripts/package-ui.sh
})
```

La librería no incluye assets de frontend, por lo que no hay dependencia en tiempo de compilación; un paquete de release con interfaz integrada debe apuntar `UIPath` al directorio `dist/uic1-ui/` incluido por defecto; la configuración anterior es la recomendada. Pasos de despliegue:

1. Compila y empaqueta en el repositorio de la interfaz: `npm run package:ui` (salida `dist/uic1-ui/`).
2. Despliega al directorio de destino (debe existir y estar vacío, o contener el paquete uic1-ui anterior): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Define `UIPath` al arrancar; déjalo vacío y reinicia para volver al modo de solo API de documentación.

Comportamiento:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` y `{Prefix}/assets/*` se sirven como archivos estáticos, con el tipo MIME determinado por la extensión;
- las rutas desconocidas sin extensión recurren a `index.html` (fallback SPA); los assets `.js` / `.css` y similares que falten devuelven 404;
- `{Prefix}` (sin barra final) redirige con 307 a `{Prefix}/` para que las rutas relativas de los assets sigan siendo correctas;
- `{Prefix}/api/*` y `{Prefix}/ui/v1/*` tienen prioridad; las rutas de API desconocidas devuelven 404 y nunca recurren a la interfaz;
- el acceso a archivos está confinado a `UIPath` (`..` y los enlaces simbólicos fuera de la raíz se rechazan), sin listado de directorios; si faltan el directorio o `index.html`, la ruta de la interfaz devuelve 500 y la API de documentación no se ve afectada;
- el adaptador go-zero solo registra las rutas fijas de API y no reenvía rutas arbitrarias bajo el prefijo, por lo que la interfaz integrada no está disponible con ese adaptador (todos los demás adaptadores la admiten);
- los despliegues que aún usan una interfaz externa no necesitan cambios; solo mantén `HTTP.API_PREFIX` en `config.js` igual al `Prefix` del servidor.

### Ejecutores de protocolo del lado del servidor (build tags)

El relay del lado del servidor no compila ningún ejecutor por defecto, y llamar a un protocolo no compilado devuelve un error de fallo cerrado. Selecciona los build tags por familia de protocolos; `apidoc_debug_all` los agrega todos:

| Build tag | Familia de protocolos | Protocolos |
|-----------|-----------------------|------------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, solo https) |
| `apidoc_debug_sockets` | Sockets | tcp, udp, raw, unix (unarios + streaming) |
| `apidoc_debug_rpc` | RPC / finanzas | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unario; descriptor o reflexión del servidor) |
| `apidoc_debug_messaging` | Mensajería | mqtt, stomp, amqp, kafka, xmpp (unarios + streaming; también seleccionables individualmente con `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industrial | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Medios | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Todo | todas las familias anteriores |

Límite de seguridad: `Debug.EnableServerSide` está desactivado por defecto; cuando se activa, los destinos deben pasar además la lista blanca `AllowHosts` (coincidencia por sufijo como `*.example.com`), las direcciones privadas/loopback/link-local/CGNAT y de metadatos de nube se deniegan por defecto (solo `AllowPrivateNetworks` las permite), la validación por IP ocurre tras la resolución DNS con la conexión fijada, y `CONNECT`, las redirecciones y los proxies están prohibidos; las peticiones y respuestas están limitadas por `TimeoutMS`, `MaxBytes` (1 MiB por defecto), las sesiones de flujo (4 por ámbito), el número de tramas y los timeouts de inactividad. El análisis de definiciones solo lee archivos regulares dentro de `DefinitionRoot`, con límites de tamaño de archivo, número de líneas y profundidad de referencias.

### Adaptadores de frameworks

| Framework | Constructor |
|-----------|-------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### Análisis automático de anotaciones (go/ast)
Escribe comentarios `@apidoc` encima de un handler y registra los resultados del análisis:

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

### Autocompletado automático de parámetros (reflect)
Cuando `Doc.Params` está vacío, Register los deduce con reflect a partir de la firma del handler: los argumentos de tipo struct se expanden como campos del body (según las json tags), y los contextos de los frameworks (gin.Context / echo.Context / fiber.Ctx) se omiten automáticamente.

### Caché HTTP (caché de respuestas + ETag)
Las respuestas de `/api/menus`, `/api/protocols` y `/api/export` se serializan una vez por versión del documento y se cachean en el proceso. La caché se precalienta al crear `Handler` y se invalida automáticamente con `Register` / `SetAppMeta`. Las respuestas siguen llevando `ETag` + `Cache-Control: private, max-age=300`, por lo que las visitas repetidas reciben 304. No se necesita configuración.

### Exportación
| Formato | Endpoint | Descripción |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Árbol completo del proyecto |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Definiciones de tipos de interfaz |

### Datos Mock
La página de detalle muestra un ejemplo Mock automáticamente: personalízalo con `Doc.Params[].Mock` o déjalo generar a partir del tipo del campo (string→"sample", int→0, bool→true, ...).

### Proyecto de ejemplo
`example/` incluye 8 servidores de frameworks (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Inícialos todos con `go run ./example`; la API de documentación heredada se sirve en `/apidoc/api/...` de cada puerto, la API UIC-1 en `/apidoc/ui/v1/...`, y pueden consumirlas una interfaz externa o la interfaz Vue integrada mediante `UIPath`.

## Documentación multilingüe

| Idioma | Enlace |
|--------|--------|
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

## Apóyanos

Si este proyecto te ha resultado útil, ¡no dudes en escanear los códigos QR para hacernos una propina y darnos energía para seguir manteniéndolo y actualizándolo!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="Propina con WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="Propina con Alipay" />
  <p>WeChat Pay (izquierda) · Alipay (derecha)</p>
</div>

### Donaciones por transferencia bancaria internacional

**【Datos del beneficiario】**
Nombre del beneficiario: WANG KEXUN
Número de cuenta: 881015918251

**【Banco beneficiario】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Nombre del banco: ZA Bank Limited
Código bancario: 387
Dirección del banco: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Banco corresponsal para remesas transfronterizas (si es necesario)】**

Ten en cuenta que esta es la información del banco corresponsal (intermediario) para remesas transfronterizas, no la del banco beneficiario. Consulta con tu banco emisor si necesita que le proporciones los datos del banco corresponsal.

Para remesas en dólares de Hong Kong, renminbi y dólares estadounidenses, el banco corresponsal es Citibank —

Nombre del banco: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Código bancario: 006
Nombre de la sucursal: Hong Kong Branch
Código de sucursal: 391
Dirección del banco: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Para remesas en otras divisas, el banco corresponsal es BNY Mellon —

Nombre del banco: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Dirección del banco: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
