![Apidoc 마스코트](../../svg/apidoc-pet.svg)

# apidoc-go — Go 범용 API 문서 플러그인

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## 프로젝트 소개

**현재 버전: v1.1.0** ([Releases](https://github.com/erikwang2013/apidoc-go/releases) 참고)

**apidoc-go**는 Go용 범용 API 문서 플러그인 라이브러리입니다: 인터페이스 문서는 라우트 등록 시 함께 **타입화된 구조체**로 선언되어, 문서와 라우트가 동시에 생성됩니다. 문서는 레거시 `/api/*`와 UIC-1 `/ui/v1/*` JSON API 모두로 제공되므로, 어떤 외부 UI든 렌더링하고 테스트 요청을 보낼 수 있으며, 패키징된 Vue UI는 `Config.UIPath`를 통해 동일 프로세스에서 제공할 수 있습니다. 비밀번호 인증, 다중 앱 / 다중 버전 관리, Mock 데이터, JSON / TypeScript 내보내기, 주석 및 9가지 형식의 프로토콜 정의 파싱, 응답 캐싱, 파라미터 자동 완성을 내장합니다. 서버 측 프로토콜 실행기는 빌드 태그에 따라 필요 시 컴파일되며 HTTP, RPC, 소켓, 메시징, IoT, 산업, 미디어 프로토콜 계열을 지원합니다. 한 번 연동하면 모든 프레임워크에서 사용할 수 있으며, 기존 프로젝트를 개조할 필요가 없습니다.

## 프로젝트 기능

| # | 기능 | 설명 |
|---|------|------|
| 1 | 문서 자동 생성 | 타입화된 Doc 선언을 라우트와 함께 등록, 한 번의 선언으로 문서와 라우트 동시 생성 |
| 2 | 문서 API | 레거시 `/api/*`(menus / protocols / detail / export)와 UIC-1 `/ui/v1/*`(docs, debug, shares, generators, export), 외부 UI 또는 내장 Vue UI용 |
| 3 | Mock 데이터 | 필드 수준 Mock 예시, 인터페이스 연동이 한 발 앞서 |
| 4 | 다중 앱 / 다중 버전 | App / Version 트리 관리, 플러그인 하나로 전체 프로젝트 문서 |
| 5 | 비밀번호 인증 | 전역 비밀번호 + 앱 수준 비밀번호, HMAC 토큰 · 항시 비교 |
| 6 | Markdown 문서 | goldmark 안전 렌더링, 네이티브 HTML 자동 제거 |
| 7 | 다중 프레임워크 지원 | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, 한 번 연동으로 전 프레임워크 공용 |
| 8 | JSON / TypeScript 내보내기 | 인터페이스 타입 원클릭 내보내기, 프런트·백엔드 연동이 더 원활 |
| 9 | 보안 보호 | 비밀번호 인증 · CORS 화이트리스트 · XSS / 경로 탐색 방지 · 서버 측 디버그용 SSRF 가드 및 세션/바이트 제한 |
| 10 | 어노테이션 및 정의 파싱 | go/ast가 `@apidoc` / `@protocol` / `@definition` 파싱, 9가지 내장 정의 형식: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC, XML-RPC |
| 11 | HTTP 캐싱 | 버전별 응답 캐시 + ETag/304, 문서 API가 밀리초 단위로 응답 |
| 12 | 파라미터 자동 완성 | reflect가 핸들러 시그니처에서 요청 파라미터를 추론 |
| 13 | 임의 프로토콜 어노테이션 | `@protocol`로 프로토콜을 지정, detail이 protocol/method/url/params/responses/mock 및 파싱된 프로토콜 정의 반환 |
| 14 | 내장 Vue UI (선택) | `Config.UIPath`를 UIC-1 패키징 빌드로 지정하면 동일 프로세스에서 Vue UI 제공, 빈 값이면 JSON API로 폴백 |
| 15 | 서버 측 프로토콜 실행기 | 빌드 태그로 컴파일되는 7개 프로토콜 계열, 기본값은 꺼짐이며 미컴파일 시 fail closed, `apidoc_debug_all`로 전체 활성화 |

## 아키텍처 개요

![프로젝트 아키텍처](../../svg/architecture.svg)

![프로젝트 기능](../../svg/features.svg)

![프로젝트 수명주기](../../svg/lifecycle.svg)

## 프로젝트 구조

```
apidoc-go/
├── apidoc.go            # 핵심 진입점: New / Register / Mount / Handler
├── VERSION              # 프로젝트 버전(v1.1.0), 릴리스 워크플로가 이 파일로 태그 생성
├── doc.go               # Route / Doc / Param / Response 타입
├── model/               # 데이터 모델
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   @definition 외부 프로토콜 정의
│   ├── protocol.go      #   URL 스킴 → 프로토콜
│   └── protocols.go     #   프로토콜 레지스트리 / 별칭 / 기본 포트
├── store/               # 저장 및 중복 제거
│   └── store.go         #   (app, version, method, url) 중복 제거, 나중에 등록된 것이 덮어씀
├── auth/                # 인증
│   └── auth.go          #   비밀번호 검증 · HMAC 토큰 발급 및 검증
├── server/              # HTTP API
│   ├── server.go        #   레거시 /api 라우트 · 인증 / CORS / 캐싱
│   ├── ui.go            #   UIPath 정적 Vue UI (SPA 폴백 · 경로 제한)
│   └── uiapi/           #   UIC-1 /ui/v1 API
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     문서 조회 · 정의 파싱
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators · 코드 템플릿 · shares · cache
│       ├── share.go
│       └── debug/       #     서버 측 프로토콜 실행기 (계열별 빌드 태그)
├── adapter/             # 프레임워크 어댑터 계층 (타입화 인터페이스)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # 어노테이션 자동 파서
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   9가지 프로토콜 정의 파서
├── export/              # 내보내기
│   └── export.go        #   TypeScript 인터페이스 정의
├── mock/                # Mock 데이터
│   └── mock.go          #   필드 수준 예시 생성
├── example/             # 샘플 프로젝트 (프레임워크 8종 :8081–:8088)
│   ├── main.go
│   └── handlers/        #   @apidoc 주석 예시
└── docs/                # 문서와 자료
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # 아키텍처도 / 기능도 / 수명주기도
```

## 사용 방법

### 설치

```bash
go get github.com/erikwang2013/apidoc-go
```

### 빠른 시작 (net/http)

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

	// 문서와 라우트를 함께 선언
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
	// 외부 UI는 /apidoc/api/menus 또는 UIC-1 /apidoc/ui/v1/*를 읽습니다.
	// UIPath가 설정되면 http://localhost:8080/apidoc/를 직접 엽니다
}
```

### UIC-1 내장 Vue UI 및 서버 측 실행기 활성화

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// 선택 사항: apidoc-ui의 npm run package:ui 출력
	// (index.html / config.js / assets/).
	// 비워 두면 JSON API만 제공하고 외부 UI를 계속 사용합니다.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// 서버 측 릴레이는 기본적으로 꺼져 있으며, 활성화해도
	// SSRF 화이트리스트와 바이트/세션 제한이 적용됩니다.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

실행기는 빌드 시 프로토콜 계열별로 선택됩니다: `go build -tags apidoc_debug_all`은 모두를 컴파일하며, `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial`, `apidoc_debug_media`로 필요한 것만 선택할 수 있습니다. 컴파일되지 않은 프로토콜 호출은 항상 fail closed로 실패합니다.

### 임의 프로토콜 어노테이션

비 HTTP 인터페이스는 `@method` + `@url`을 재사용하고 `@protocol`로 프로토콜을 지정합니다(URL scheme에서 추론할 수도 있음):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title 온도 발행
// @param qos int false "QoS" in=option default=1
// @param payload json true "메시지 본문" in=payload
func PublishTemp() {}
```

`@protocol`는 전체 프로토콜 목록과 별칭을 허용합니다(HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket). 전체 key, 별칭, scheme, 기본 포트 레지스트리는 `GET /api/protocols`에 있습니다. 바이너리 body는 `@param ... type=binary`(또는 `hex`/`base64`) + `in=payload`로 선언합니다.

`/api/detail`과 UIC-1 `/ui/v1/docDetail`은 `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` 및 파싱된 `definitions`를 반환합니다. 디버깅에는 두 가지 채널이 있습니다: 브라우저에서 직접(HTTP / WebSocket / SSE, 항상 사용 가능)와 서버 측 릴레이(`Debug` 활성화 + 해당 빌드 태그 필요). 서버 측 프로토콜 계열은 HTTP(http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC(gRPC / Thrift / Dubbo / Hessian / ISO 8583), 소켓(TCP / UDP / Raw / Unix), 메시징(MQTT / STOMP / AMQP / Kafka / XMPP), IoT(CoAP / LwM2M), 산업(Modbus / DNP3 / IEC 104 / BACnet / OPC UA), 미디어(RTSP / SIP / RTP / RTCP / RTMP)입니다. 기본 빌드에는 실행기가 하나도 포함되지 않으며, 활성화하면 대상은 먼저 SSRF 가드(`AllowHosts` 화이트리스트, 사설/루프백/링크로컬/클라우드 메타데이터 주소는 기본 차단, DNS 해석 후 IP별 검증과 연결 고정)를 통과해야 하고, 타임아웃, 메시지당 1 MiB 제한, 응답 `MaxBytes`, 스트림 세션 제한이 적용됩니다.

### 프로토콜 정의 파싱 (@definition)

`@definition`으로 외부 프로토콜 정의를 가져옵니다(형식은 자동 감지되거나 `format=`으로 지정). 파싱 결과는 detail과 함께 반환되어 UIC가 메시지 구조를 렌더링하고 디버그 폼을 구성할 수 있습니다:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

지원 형식: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX(OData), JSON Schema, JSON-RPC, XML-RPC. `Config.DefinitionRoot`는 읽을 수 있는 파일을 루트 디렉터리로 제한하며(기본값은 프로세스 작업 디렉터리), 파싱은 파일당 1 MiB 및 50,000줄, 중첩 참조 깊이 32로 제한됩니다. `operation=`, `message=`, `direction=`, `sample=`, `inline=` 등의 옵션은 `parse` 패키지 주석을 참고하세요.

### 빠른 시작 (Gin)

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

Echo / Chi / Fiber는 어댑터 생성자만 교체하면 됩니다: `adapter.NewEcho(e)`、`adapter.NewChi(mux)`、`adapter.NewFiber(app)`, 나머지 코드는 완전히 동일합니다. beego / Kratos / go-zero는 아래 프레임워크 어댑터 표를 참고하세요.

### 설정 항목

| 설정 항목 | 기본값 | 설명 |
|--------|--------|------|
| `Prefix` | `/apidoc` | 문서 마운트 경로 |
| `Title` | `API Docs` | 문서 제목 |
| `Desc` | — | 문서 설명 |
| `GlobalParams` | — | 전역 파라미터, 모든 Action에 병합 |
| `Auth` | — | 인증 설정: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | 앱 설정: `Key` / `Title` / `Password` (앱 수준 독립 비밀번호) |
| `DebugOrigins` | — | 문서 API를 교차 도메인으로 읽도록 허용할 출처 (외부 UI, 기본값 없음) |
| `PublicHost` | — | 외부 UI가 디버깅에 사용하는 절대 기준 URL, 비어 있으면 동일 출처 |
| `ShareDir` | 임시 디렉터리 아래 `apidoc-go-shares` | UIC-1 공유 기록 디렉터리 |
| `DefinitionRoot` | 프로세스 작업 디렉터리 | `@definition` 파일 루트, 루트 밖 경로와 심볼릭 링크는 거부 |
| `Generator` / `CodeTemplates` | — | 선언적 생성기 및 코드 템플릿 (렌더링은 부작용 없음) |
| `ShareActions` | — | UI가 해석하는 공유 액션 선언 |
| `Debug` | 0값 = 꺼짐 | 서버 측 실행기: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (내장 UI 없음) | 패키징된 UIC-1 Vue 빌드 디렉터리, 값이 있으면 `{Prefix}/` 아래에서 동일 프로세스로 정적 UI 제공 |

### UIC-1 API 및 내장 Vue UI (선택)

라이브러리는 기본적으로 두 가지 JSON 문서 API를 제공합니다: 레거시 `/api/*`와 UIC-1 `/ui/v1/*`이며, 외부 UI가 이를 소비합니다. `GET {Prefix}/ui/v1/capabilities`는 계약, 백엔드 버전, 기능 매트릭스(브라우저/서버 디버그 모드, shares, cache, generators, export, auth)를 반환하며, UI는 이를 사용해 사용 가능한 패널을 결정합니다. `/ui/v1/config`는 메뉴와 생성기·코드 템플릿 설정을 반환합니다. 패키징된 `apidoc-ui` 빌드를 `Config.UIPath`에 넘기면 동일 프로세스가 `{Prefix}/` 아래에서 Vue UI도 직접 제공합니다:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // scripts/package-ui.sh 출력 디렉터리
})
```

라이브러리 자체는 프런트엔드 자산을 내장하지 않으므로 빌드 시 의존성이 없습니다. 내장 UI 릴리스 패키지는 기본적으로 `UIPath`를 동봉된 `dist/uic1-ui/` 디렉터리로 지정해야 하며, 위 배선이 권장 설정입니다. 배포 단계:

1. UI 저장소에서 빌드 및 패키징: `npm run package:ui` (출력 `dist/uic1-ui/`).
2. 대상 디렉터리에 배포(디렉터리가 존재해야 하며 비어 있거나 이전 uic1-ui 패키지가 있어야 함): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. 시작 시 `UIPath` 설정, 비워 두고 재시작하면 문서 API 전용 모드로 롤백됩니다.

동작:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js`, `{Prefix}/assets/*`는 확장자에 따라 MIME 타입이 결정되는 정적 파일로 제공됩니다.
- 확장자가 없는 알 수 없는 경로는 `index.html`로 폴백(SPA 폴백)되고, 없는 `.js` / `.css` 등 자산은 404를 반환합니다.
- `{Prefix}`(후행 슬래시 없음)는 상대 자산 경로가 유지되도록 307로 `{Prefix}/`로 리다이렉트됩니다.
- `{Prefix}/api/*`와 `{Prefix}/ui/v1/*`가 우선하며, 알 수 없는 API 경로는 404를 반환하고 UI로 폴백하지 않습니다.
- 파일 접근은 `UIPath`로 제한되며(`..` 및 루트 밖 심볼릭 링크 거부), 디렉터리 목록을 제공하지 않습니다. 디렉터리나 `index.html`이 없으면 UI 라우트는 500을 반환하고 문서 API는 영향을 받지 않습니다.
- go-zero 어댑터는 고정 API 라우트만 등록하고 접두사 아래의 임의 경로를 전달하지 않으므로, 해당 어댑터에서는 내장 UI를 사용할 수 없습니다(다른 어댑터는 모두 지원).
- 외부 UI를 계속 사용하는 배포는 변경이 필요 없으며, `config.js`의 `HTTP.API_PREFIX`를 서버 `Prefix`와 같게 유지하면 됩니다.

### 서버 측 프로토콜 실행기 (빌드 태그)

서버 측 릴레이는 기본적으로 실행기를 컴파일하지 않으며, 컴파일되지 않은 프로토콜 호출은 fail-closed 오류를 반환합니다. 프로토콜 계열별로 빌드 태그를 선택하며, `apidoc_debug_all`은 모두를 집계합니다:

| 빌드 태그 | 프로토콜 계열 | 프로토콜 |
|-----------|-----------------|-----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, https 전용) |
| `apidoc_debug_sockets` | 소켓 | tcp, udp, raw, unix (단항 + 스트리밍) |
| `apidoc_debug_rpc` | RPC / 금융 | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (단항, 디스크립터 또는 서버 리플렉션) |
| `apidoc_debug_messaging` | 메시징 | mqtt, stomp, amqp, kafka, xmpp (단항 + 스트리밍, `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`로 개별 선택 가능) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | 산업 | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | 미디어 | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | 전체 | 위의 모든 계열 |

보안 경계: `Debug.EnableServerSide`는 기본적으로 꺼져 있으며, 활성화해도 대상은 `AllowHosts` 화이트리스트(접미사 일치, 예: `*.example.com`)를 통과해야 합니다. 사설/루프백/링크로컬/CGNAT 및 클라우드 메타데이터 주소는 기본 차단되며(`AllowPrivateNetworks`만 허용), DNS 해석 후 IP별 검증이 연결 고정과 함께 수행되고 `CONNECT`, 리다이렉트, 프록시는 금지됩니다. 요청과 응답은 `TimeoutMS`, `MaxBytes`(기본 1 MiB), 스트림 세션(스코프당 4개), 프레임 수, 유휴 타임아웃으로 제한됩니다. 정의 파싱은 `DefinitionRoot` 내부의 일반 파일만 읽으며, 파일 크기, 줄 수, 참조 깊이로 제한됩니다.

### 프레임워크 어댑터

| 프레임워크 | 생성자 |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### 어노테이션 자동 파싱 (go/ast)

핸들러 위에 `@apidoc` 주석을 작성한 후, 파싱 결과를 등록합니다:

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

### 파라미터 자동 완성 (reflect)

`Doc.Params`가 비어 있으면 Register가 핸들러 시그니처에서 reflect로 파라미터를 추론합니다. struct 인자는 본문 필드로 확장되고(json 태그를 따름), 프레임워크 컨텍스트(gin.Context / echo.Context / fiber.Ctx)는 자동으로 건너뜁니다.

### HTTP 캐싱 (응답 캐시 + ETag)

`/api/menus`, `/api/protocols`, `/api/export` 응답은 문서 버전마다 한 번 직렬화되어 프로세스 내에 캐시됩니다. `Handler` 생성 시 예열되고 `Register` / `SetAppMeta` 후 자동으로 무효화됩니다. 응답에는 여전히 `ETag` + `Cache-Control: private, max-age=300`가 적용되어 재방문 시 304를 반환합니다. 설정이 필요 없습니다.

### 내보내기

| 형식 | 엔드포인트 | 설명 |
|------|----------|-------------|
| JSON | `GET /apidoc/api/export` | 전체 프로젝트 트리 |
| TypeScript | `GET /apidoc/api/export?format=typescript` | 인터페이스 타입 정의 |

### Mock 데이터

상세 페이지에 Mock 예시가 자동으로 표시됩니다. `Doc.Params[].Mock`으로 커스터마이즈하거나 필드 타입에서 자동 생성됩니다(string→"sample", int→0, bool→true, ...).

### 샘플 프로젝트

`example/`에는 8개의 프레임워크 서버(net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088)가 포함되어 있습니다. `go run ./example`로 모두 실행할 수 있으며, 레거시 문서 API는 각 포트의 `/apidoc/api/...`, UIC-1 API는 `/apidoc/ui/v1/...`에 있고, 외부 UI 또는 `UIPath`를 통한 내장 Vue UI가 이를 소비할 수 있습니다.

## 다국어 문서

| 언어 | 링크 |
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

## 후원하기

이 프로젝트가 도움이 되셨다면 QR코드로 후원해 주세요. 지속적인 유지보수와 업데이트에 힘이 됩니다!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat Pay 후원" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay 후원" />
  <p>WeChat Pay(왼쪽) · Alipay(오른쪽)</p>
</div>

### 해외 송금 후원

**【수취인 정보】**
수취인 이름: WANG KEXUN
수취인 계좌번호: 881015918251

**【수취 은행】**
ZA Bank
SWIFT Code: AABLHKHHXXX
은행 이름: ZA Bank Limited
은행 코드: 387
은행 주소: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【국제 송금 대리은행(필요 시)】**

참고: 아래 정보는 국제 송금 대리은행(중계은행) 정보이며, 수취 은행 정보가 아닙니다. 송금 은행에 이 정보를 제공해야 하는지 문의하시기 바랍니다.

홍콩 달러(HKD), 위안화(CNY) 및 미국 달러(USD) 송금 시 대리은행은 Citibank입니다 —

은행 이름: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
은행 코드: 006
지점 이름: Hong Kong Branch
지점 코드: 391
은행 주소: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

기타 통화 송금 시 대리은행은 BNY Mellon입니다 —

은행 이름: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
은행 주소: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
