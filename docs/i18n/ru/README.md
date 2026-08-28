# apidoc-go — универсальный плагин API-документации на Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## О проекте

**apidoc-go** — универсальная библиотека-плагин API-документации для Go: интерфейсная документация объявляется в виде **типизированных структур** вместе с регистрацией маршрута — документация и маршрут появляются одновременно. Встроенный Web UI обеспечивает онлайн-просмотр и онлайн-отладку, а также включает аутентификацию по паролю, управление несколькими приложениями/версиями, Mock-данные и экспорт в JSON / TypeScript, а также автопарсинг аннотаций, HTTP-кэширование и автозаполнение параметров. Одна интеграция — работает с любым фреймворком, доработка существующего проекта не требуется.

## Возможности

| # | Возможность | Описание |
|---|------|------|
| 1 | Автогенерация документации | Типизированные объявления Doc, регистрируемые вместе с маршрутом — одно объявление, документация и маршрут появляются одновременно |
| 2 | Онлайн-отладка | Запросы к реальным интерфейсам прямо из браузера, без серверной пересылки — SSRF исключён |
| 3 | Mock-данные | Mock-примеры на уровне полей, интеграция API ускоряется |
| 4 | Несколько приложений / версий | Древовидное управление App / Version, один плагин покрывает документацию всего проекта |
| 5 | Аутентификация по паролю | Глобальный пароль + пароль уровня приложения, HMAC-токен · сравнение за постоянное время |
| 6 | Markdown-документация | Безопасный рендеринг через goldmark, нативный HTML автоматически удаляется |
| 7 | Поддержка фреймворков | net/http · Gin · Echo · Chi · Fiber, одна интеграция — работает везде |
| 8 | Экспорт JSON / TypeScript | Экспорт типов интерфейсов в один клик, более гладкая интеграция фронтенда и бэкенда |
| 9 | Защита | Без SSRF · CORS по белому списку · защита от XSS · защита от path traversal |
| 10 | Автопарсинг аннотаций | go/ast генерирует документацию из комментариев — достаточно маркера `@apidoc` |
| 11 | HTTP-кэширование | ETag + 304, ответы документации открываются мгновенно |
| 12 | Автозаполнение параметров | reflect выводит параметры запроса из сигнатуры обработчика |

## Обзор архитектуры

![Архитектура проекта](../../svg/architecture.svg)

![Возможности проекта](../../svg/features.svg)

![Жизненный цикл проекта](../../svg/lifecycle.svg)

## Структура проекта

```
apidoc-go/
├── apidoc.go            # Основная точка входа: New / Register / Mount / Handler
├── doc.go               # Типы Route / Doc / Param / Response
├── model/               # Модель данных
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Хранение и дедупликация
│   └── store.go         #   Дедупликация по (app, version, method, url), позднее объявление перезаписывает
├── auth/                # Аутентификация
│   └── auth.go          #   Проверка пароля · выпуск и проверка HMAC-токена
├── server/              # JSON API + встроенный Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Слой адаптеров фреймворков (типизированные интерфейсы)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Автопарсер аннотаций
│   └── parse.go         #   go/ast · маркеры комментариев @apidoc
├── export/              # Экспорт
│   └── export.go        #   Определения интерфейсов TypeScript
├── mock/                # Mock-данные
│   └── mock.go          #   Генерация примеров на уровне полей
├── example/             # Пример проекта (5 фреймворков :8081–:8085)
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
	// Откройте в браузере http://localhost:8080/apidoc
}
```

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

Для Echo / Chi / Fiber достаточно заменить конструктор адаптера: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — остальной код полностью идентичен.

### Параметры конфигурации

| Параметр | По умолчанию | Описание |
|--------|--------|------|
| `Prefix` | `/apidoc` | Путь монтирования документации |
| `Title` | `API Docs` | Заголовок документации |
| `Desc` | — | Описание документации |
| `GlobalParams` | — | Глобальные параметры, объединяются в каждое Action |
| `Auth` | — | Конфигурация аутентификации: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Конфигурация приложений: `Key` / `Title` / `Password` (независимый пароль уровня приложения) |
| `DebugOrigins` | — | Источники, допущенные к кросс-доменной отладке (по умолчанию нет) |

### Адаптеры фреймворков

| Фреймворк | Конструктор |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

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

### HTTP-кэширование (ETag)

Все эндпоинты документации автоматически отдают `ETag` + `Cache-Control: private, max-age=300`; при повторных визитах возвращается 304. Настройка не требуется.

### Экспорт

| Формат | Endpoint | Описание |
|--------|----------|----------|
| JSON | `GET /apidoc/api/export` | Полное дерево проекта |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Определения типов интерфейсов |

### Mock-данные

Страница деталей автоматически показывает Mock-пример: настройте через `Doc.Params[].Mock` или дайте сгенерировать по типу поля (string→"sample", int→0, bool→true, ...).

### Пример проекта

`example/` включает 5 серверов на разных фреймворках (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085). Запустите все командой `go run ./example`.

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
