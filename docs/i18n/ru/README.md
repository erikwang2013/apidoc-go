![Талисман Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — универсальный плагин API-документации на Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## О проекте

**Текущая версия: v1.1.0** (см. [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** — универсальная библиотека-плагин API-документации для Go: интерфейсная документация объявляется в виде **типизированных структур** вместе с регистрацией маршрута — документация и маршрут появляются одновременно. Документация публикуется через прежний JSON API `/api/*` и UIC-1 JSON API `/ui/v1/*`, поэтому любой внешний UI может отображать её и отправлять тестовые запросы — или упакованный Vue UI может обслуживаться в том же процессе через `Config.UIPath`. Включены: аутентификация по паролю, управление несколькими приложениями/версиями, Mock-данные, экспорт в JSON / TypeScript, автопарсинг комментариев и протокольных определений в девяти форматах, кэширование ответов и автозаполнение параметров; серверные исполнители протоколов компилируются по требованию через build-теги и покрывают семейства протоколов HTTP, RPC, сокетов, обмена сообщениями, IoT, промышленных и медиапротоколов. Одна интеграция — работает с любым фреймворком, доработка существующего проекта не требуется.

## Возможности

| # | Возможность | Описание |
|---|------|------|
| 1 | Автогенерация документации | Типизированные объявления Doc, регистрируемые вместе с маршрутом — одно объявление, документация и маршрут появляются одновременно |
| 2 | API документации | Прежний `/api/*` (menus / protocols / detail / export) и UIC-1 `/ui/v1/*` (docs, debug, shares, generators, export) — для внешних UI или встроенного Vue UI |
| 3 | Mock-данные | Mock-примеры на уровне полей, интеграция API ускоряется |
| 4 | Несколько приложений / версий | Древовидное управление App / Version, один плагин покрывает документацию всего проекта |
| 5 | Аутентификация по паролю | Глобальный пароль + пароль уровня приложения, HMAC-токен · сравнение за постоянное время |
| 6 | Markdown-документация | Безопасный рендеринг через goldmark, нативный HTML автоматически удаляется |
| 7 | Поддержка фреймворков | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, одна интеграция — работает везде |
| 8 | Экспорт JSON / TypeScript | Экспорт типов интерфейсов в один клик, более гладкая интеграция фронтенда и бэкенда |
| 9 | Защита | Аутентификация по паролю · CORS по белому списку · защита от XSS / path traversal · SSRF-защита и лимиты сессий/байтов для серверной отладки |
| 10 | Автопарсинг аннотаций и определений | go/ast разбирает `@apidoc` / `@protocol` / `@definition`; девять встроенных форматов определений: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC и XML-RPC |
| 11 | HTTP-кэширование | Версионированный кэш ответов + ETag/304, эндпоинты документации отвечают за миллисекунды |
| 12 | Автозаполнение параметров | reflect выводит параметры запроса из сигнатуры обработчика |
| 13 | Аннотации для любых протоколов | `@protocol` задаёт протокол; detail возвращает protocol/method/url/params/responses/mock, а также разобранные протокольные определения |
| 14 | Встроенный Vue UI (опционально) | Укажите `Config.UIPath` на упакованную сборку UIC-1, чтобы обслуживать Vue UI в том же процессе; пустое значение означает работу только через JSON API |
| 15 | Серверные исполнители протоколов | Семь семейств протоколов, компилируемых через build-теги; по умолчанию выключены и fail closed, если не скомпилированы, а `apidoc_debug_all` включает всё |

## Обзор архитектуры

![Архитектура проекта](../../svg/architecture.svg)

![Возможности проекта](../../svg/features.svg)

![Жизненный цикл проекта](../../svg/lifecycle.svg)

## Структура проекта

```
apidoc-go/
├── apidoc.go            # Основная точка входа: New / Register / Mount / Handler
├── VERSION              # Версия проекта (v1.1.0); release-процесс ставит по ней тег
├── doc.go               # Типы Route / Doc / Param / Response
├── model/               # Модель данных
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Внешние протокольные определения @definition
│   ├── protocol.go      #   схема URL → протокол
│   └── protocols.go     #   реестр протоколов / псевдонимы / порты по умолчанию
├── store/               # Хранение и дедупликация
│   └── store.go         #   Дедупликация по (app, version, method, url), позднее объявление перезаписывает
├── auth/                # Аутентификация
│   └── auth.go          #   Проверка пароля · выпуск и проверка HMAC-токена
├── server/              # HTTP API
│   ├── server.go        #   прежние маршруты /api · auth / CORS / кэширование
│   ├── ui.go            #   Статический Vue UI из UIPath (SPA fallback · доступ в пределах пути)
│   └── uiapi/           #   UIC-1 API /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     чтение документа · разбор определений
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators · code templates · shares · cache
│       ├── share.go
│       └── debug/       #     серверные исполнители протоколов (build-тег на семейство)
├── adapter/             # Слой адаптеров фреймворков (типизированные интерфейсы)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Автопарсер аннотаций
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   девять парсеров протокольных определений
├── export/              # Экспорт
│   └── export.go        #   Определения интерфейсов TypeScript
├── mock/                # Mock-данные
│   └── mock.go          #   Генерация примеров на уровне полей
├── example/             # Пример проекта (8 фреймворков :8081–:8088)
│   ├── main.go
│   └── handlers/        #   Примеры комментариев @apidoc
└── docs/                # Документация и материалы
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Диаграммы: архитектура / возможности / жизненный цикл
```

## Использование

### Установка

```bash
go get github.com/erikwang2013/apidoc-go
```

### Быстрый старт (net/http)

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

	// Документация объявляется вместе с маршрутом
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
	// Внешние UI читают /apidoc/api/menus или UIC-1 /apidoc/ui/v1/*;
	// при заданном UIPath откройте http://localhost:8080/apidoc/ напрямую
}
```

### Включение встроенного Vue UI UIC-1 и серверных исполнителей

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Опционально: вывод npm run package:ui из apidoc-ui
	// (index.html / config.js / assets/).
	// Оставьте пустым, чтобы обслуживать только JSON API и использовать внешний UI.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// Серверный relay по умолчанию выключен; при включении он по-прежнему
	// подчиняется SSRF-белому списку и лимитам байтов/сессий.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Исполнители выбираются на этапе сборки по семейству протоколов: `go build -tags apidoc_debug_all` компилирует все, либо ограничьте набор тегами `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` и `apidoc_debug_media`. Вызовы протоколов, не скомпилированных в сборку, всегда завершаются fail closed.

### Аннотации для любых протоколов

Для интерфейсов, отличных от HTTP, используются те же `@method` + `@url`, а протокол задаётся через `@protocol` (его также можно вывести из схемы URL):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Публикация температуры
// @param qos int false "QoS" in=option default=1
// @param payload json true "Тело сообщения" in=payload
func PublishTemp() {}
```

`@protocol` принимает полный список протоколов и их алиасы (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); полный реестр ключей, алиасов, схем и портов по умолчанию доступен по `GET /api/protocols`. Двоичные тела объявляются через `@param ... type=binary` (или `hex`/`base64`) + `in=payload`.

`/api/detail` и UIC-1 `/ui/v1/docDetail` возвращают `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` и разобранные `definitions`.

Отладка имеет два канала: напрямую из браузера (HTTP / WebSocket / SSE, доступно всегда) и серверный relay (при включённом `Debug` и соответствующем build-теге). Серверные семейства протоколов: HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), сокеты (TCP / UDP / Raw / Unix), обмен сообщениями (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), промышленные (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) и медиа (RTSP / SIP / RTP / RTCP / RTMP). Сборки по умолчанию не компилируют ни одного исполнителя; при включении цели должны сначала пройти SSRF-защиту (белый список `AllowHosts`, адреса private/loopback/link-local/cloud-metadata по умолчанию запрещены, проверка каждого IP после DNS-разрешения с закреплением соединения), а также ограничены тайм-аутами, лимитом 1 MiB на сообщение, `MaxBytes` ответа и лимитами потоковых сессий.

### Разбор протокольных определений (@definition)

Используйте `@definition`, чтобы подключить внешние протокольные определения (формат определяется автоматически или задаётся через `format=`); разобранный результат возвращается вместе с detail, чтобы UIC мог отрисовать структуры сообщений и построить формы отладки:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Поддерживаемые форматы: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC и XML-RPC. `Config.DefinitionRoot` ограничивает читаемые файлы корневой директорией (по умолчанию — рабочая директория процесса); разбор ограничен 1 MiB и 50 000 строк на файл и глубиной вложенных ссылок 32; параметры вроде `operation=`, `message=`, `direction=`, `sample=` и `inline=` описаны в комментариях пакета `parse`.

### Быстрый старт (Gin)

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

Для Echo / Chi / Fiber достаточно заменить конструктор адаптера: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — остальной код полностью идентичен. Для beego / Kratos / go-zero см. таблицу «Адаптеры фреймворков» ниже.

### Параметры конфигурации

| Параметр | По умолчанию | Описание |
|--------|--------|------|
| `Prefix` | `/apidoc` | Путь монтирования документации |
| `Title` | `API Docs` | Заголовок документации |
| `Desc` | — | Описание документации |
| `GlobalParams` | — | Глобальные параметры, объединяются в каждое Action |
| `Auth` | — | Конфигурация аутентификации: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Конфигурация приложений: `Key` / `Title` / `Password` (независимый пароль уровня приложения) |
| `DebugOrigins` | — | Источники, которым разрешено кросс-доменное чтение API документации (внешний UI, по умолчанию нет) |
| `PublicHost` | — | Абсолютный базовый URL, используемый внешними UI для отладки; пусто означает same-origin |
| `ShareDir` | `apidoc-go-shares` во временной директории | Директория записей share для UIC-1 |
| `DefinitionRoot` | рабочая директория процесса | Корень файлов `@definition`; пути вне корня и символические ссылки отклоняются |
| `Generator` / `CodeTemplates` | — | Декларативные генераторы и шаблоны кода (рендеринг без побочных эффектов) |
| `ShareActions` | — | Объявления share-действий, интерпретируемые UI |
| `Debug` | нулевое значение = выключено | Серверные исполнители: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (без встроенного UI) | Директория упакованной Vue-сборки UIC-1; если не пуста, статический UI обслуживается в том же процессе по `{Prefix}/` |

### API UIC-1 и встроенный Vue UI (опционально)

Библиотека по умолчанию предоставляет два JSON API документации: прежний `/api/*` и UIC-1 `/ui/v1/*`, которые потребляет внешний UI. `GET {Prefix}/ui/v1/capabilities` возвращает контракт, версию бэкенда и матрицу возможностей (режимы отладки в браузере/на сервере, shares, кэш, генераторы, экспорт, auth); UI использует её, чтобы определить доступные панели. `/ui/v1/config` возвращает меню, а также конфигурацию генераторов и шаблонов кода. После передачи упакованной сборки `apidoc-ui` в `Config.UIPath` тот же процесс также обслуживает Vue UI напрямую по `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // выходная директория scripts/package-ui.sh
})
```

Библиотека сама не встраивает фронтенд-ресурсы, поэтому зависимости на этапе сборки нет; release-пакет со встроенным UI должен по умолчанию указывать `UIPath` на поставляемую директорию `dist/uic1-ui/` — приведённая выше настройка является рекомендуемой. Шаги развёртывания:

1. Соберите и упакуйте в репозитории UI: `npm run package:ui` (выход `dist/uic1-ui/`).
2. Разверните в целевую директорию (она должна существовать и быть пустой либо содержать предыдущий пакет uic1-ui): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Задайте `UIPath` при старте; оставьте пустым и перезапустите, чтобы вернуться в режим только API документации.

Поведение:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` и `{Prefix}/assets/*` отдаются как статические файлы, MIME-тип определяется по расширению;
- пути без расширения, которых нет на диске, откатываются к `index.html` (SPA fallback); отсутствующие `.js` / `.css` и подобные ресурсы возвращают 404;
- `{Prefix}` (без завершающего слэша) перенаправляется с 307 на `{Prefix}/`, чтобы относительные пути ресурсов оставались корректными;
- `{Prefix}/api/*` и `{Prefix}/ui/v1/*` имеют приоритет; неизвестные API-пути возвращают 404 и никогда не откатываются к UI;
- доступ к файлам ограничен `UIPath` (пути `..` и символические ссылки вне корня отклоняются), листинг директорий отсутствует; если директория или `index.html` отсутствуют, UI-маршрут возвращает 500, а API документации не затрагивается;
- адаптер go-zero регистрирует только фиксированные API-маршруты и не перенаправляет произвольные пути под префиксом, поэтому встроенный UI с этим адаптером недоступен (все остальные адаптеры его поддерживают);
- развёртывания, которые по-прежнему используют внешний UI, изменений не требуют; просто держите `HTTP.API_PREFIX` в `config.js` равным серверному `Prefix`.

### Серверные исполнители протоколов (build-теги)

Серверный relay по умолчанию не компилирует ни одного исполнителя, а вызов несомпилированного протокола возвращает ошибку fail closed. Выбирайте build-теги по семейству протоколов; `apidoc_debug_all` объединяет их все:

| Build-тег | Семейство протоколов | Протоколы |
|-----------|----------------------|-----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, только https) |
| `apidoc_debug_sockets` | Сокеты | tcp, udp, raw, unix (unary + streaming) |
| `apidoc_debug_rpc` | RPC / финансы | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unary; дескриптор или серверная рефлексия) |
| `apidoc_debug_messaging` | Обмен сообщениями | mqtt, stomp, amqp, kafka, xmpp (unary + streaming; также выбираются отдельно через `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Промышленные | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Медиа | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Все | все перечисленные семейства |

Граница безопасности: `Debug.EnableServerSide` по умолчанию выключен; при включении цели всё равно должны пройти белый список `AllowHosts` (сопоставление по суффиксу, например `*.example.com`), адреса private/loopback/link-local/CGNAT и cloud-metadata по умолчанию запрещены (их разрешает только `AllowPrivateNetworks`), проверка каждого IP выполняется после DNS-разрешения с закреплением соединения, а `CONNECT`, редиректы и прокси запрещены; запросы и ответы ограничены `TimeoutMS`, `MaxBytes` (1 MiB по умолчанию), потоковыми сессиями (4 на scope), количеством кадров и тайм-аутами простоя. Разбор определений читает только обычные файлы внутри `DefinitionRoot`, с ограничениями по размеру файла, числу строк и глубине ссылок.

### Адаптеры фреймворков

| Фреймворк | Конструктор |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### Автопарсинг аннотаций (go/ast)

Напишите комментарии `@apidoc` над обработчиком, затем зарегистрируйте результаты парсинга:

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

### Автозаполнение параметров (reflect)

Если `Doc.Params` пуст, Register выводит параметры с помощью reflect из сигнатуры обработчика: аргументы-структуры раскрываются в поля тела (следуя json-тегам), контексты фреймворка (gin.Context / echo.Context / fiber.Ctx) пропускаются автоматически.

### HTTP-кэширование (кэш ответов + ETag)

Ответы `/api/menus`, `/api/protocols` и `/api/export` сериализуются один раз на версию документа и кэшируются в процессе. Кэш прогревается при создании `Handler` и автоматически сбрасывается после `Register` / `SetAppMeta`. Ответы по-прежнему содержат `ETag` + `Cache-Control: private, max-age=300`, поэтому повторные визиты получают 304. Настройка не требуется.

### Экспорт

| Формат | Endpoint | Описание |
|--------|----------|----------|
| JSON | `GET /apidoc/api/export` | Полное дерево проекта |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Определения типов интерфейсов |

### Mock-данные

Страница деталей автоматически показывает Mock-пример: настройте через `Doc.Params[].Mock` или дайте сгенерировать по типу поля (string→"sample", int→0, bool→true, ...).

### Пример проекта

`example/` включает 8 серверов на разных фреймворках (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Запустите все командой `go run ./example`; прежний API документации доступен на каждом порту по пути `/apidoc/api/...`, UIC-1 API — по `/apidoc/ui/v1/...`, и потреблять их может как внешний UI, так и встроенный Vue UI через `UIPath`.

## Документация на других языках

| Язык | Ссылка |
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

## Поддержка проекта

Если этот проект вам полезен, поддержите нас пожертвованием по QR-коду — это даст нам силы продолжать развитие и обновления!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="Пожертвование через WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="Пожертвование через Alipay" />
  <p>WeChat Pay (слева) · Alipay (справа)</p>
</div>

### Банковское пожертвование (международный перевод)

**【Информация о получателе】**
Имя получателя: WANG KEXUN
Номер счёта получателя: 881015918251

**【Банк получателя】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Название банка: ZA Bank Limited
Код банка: 387
Адрес банка: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Банк-посредник для международных переводов (при необходимости)】**

Обратите внимание: это информация о банке-посреднике (корреспондентском банке) для международных переводов, а не о банке получателя. Уточните в вашем банке, требуется ли её предоставлять.

Банк-посредник для переводов в гонконгских долларах, китайских юанях и долларах США — Citibank:

Название банка: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Код банка: 006
Название отделения: Hong Kong Branch
Код отделения: 391
Адрес банка: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Банк-посредник для переводов в других валютах — BNY Mellon:

Название банка: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Адрес банка: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
