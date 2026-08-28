# apidoc-go — Go 범용 API 문서 플러그인

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## 프로젝트 소개

**apidoc-go**는 Go용 범용 API 문서 플러그인 라이브러리입니다. 인터페이스 문서는 **타입화된 구조체**로 라우트 등록 시 함께 선언되어, 문서와 라우트가 동시에 생성됩니다. 내장 Web UI는 온라인 열람과 온라인 디버깅을 제공하며, 비밀번호 인증, 다중 앱/다중 버전 관리, Mock 데이터, JSON / TypeScript 내보내기 기능을 내장하고 있습니다. 한 번 연동하면 모든 프레임워크에서 사용할 수 있으며, 기존 프로젝트를 개조할 필요가 없습니다.

## 프로젝트 기능

| # | 기능 | 설명 |
|---|------|------|
| 1 | 문서 자동 생성 | 타입화된 Doc 선언을 라우트와 함께 등록, 한 번의 선언으로 문서와 라우트 동시 생성 |
| 2 | 온라인 디버깅 | 브라우저에서 실제 인터페이스에 직접 요청, 서버 전달이 없어 SSRF 원천 차단 |
| 3 | Mock 데이터 | 필드 수준 Mock 예시, 인터페이스 연동이 한 발 앞서 |
| 4 | 다중 앱 / 다중 버전 | App / Version 트리 관리, 플러그인 하나로 전체 프로젝트 문서 |
| 5 | 비밀번호 인증 | 전역 비밀번호 + 앱 수준 비밀번호, HMAC 토큰 · 항시 비교 |
| 6 | Markdown 문서 | goldmark 안전 렌더링, 네이티브 HTML 자동 제거 |
| 7 | 다중 프레임워크 지원 | net/http · Gin · Echo · Chi · Fiber, 한 번 연동으로 전 프레임워크 공용 |
| 8 | JSON / TypeScript 내보내기 | 인터페이스 타입 원클릭 내보내기, 프런트·백엔드 연동이 더 원활 |
| 9 | 보안 | SSRF 없음 · CORS 화이트리스트 제한 · XSS 방지 · 경로 탐색 방지 |

## 아키텍처 개요

![프로젝트 아키텍처](../../svg/architecture.svg)

![프로젝트 기능](../../svg/features.svg)

![프로젝트 수명주기](../../svg/lifecycle.svg)

## 프로젝트 구조

```
apidoc-go/
├── apidoc.go            # 핵심 진입점: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response 타입
├── model/               # 데이터 모델
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # 저장 및 중복 제거
│   └── store.go         #   (app, version, method, url) 중복 제거, 나중에 등록된 것이 덮어씀
├── auth/                # 인증
│   └── auth.go          #   비밀번호 검증 · HMAC 토큰 발급 및 검증
├── server/              # JSON API + 내장 Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # 프레임워크 어댑터 계층 (타입화 인터페이스)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
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
	// 브라우저에서 http://localhost:8080/apidoc 접속
}
```

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

Echo / Chi / Fiber는 어댑터 생성자만 교체하면 됩니다: `adapter.NewEcho(e)`、`adapter.NewChi(mux)`、`adapter.NewFiber(app)`, 나머지 코드는 완전히 동일합니다.

### 설정 항목

| 설정 항목 | 기본값 | 설명 |
|--------|--------|------|
| `Prefix` | `/apidoc` | 문서 마운트 경로 |
| `Title` | `API Docs` | 문서 제목 |
| `Desc` | — | 문서 설명 |
| `GlobalParams` | — | 전역 파라미터, 모든 Action에 병합 |
| `Auth` | — | 인증 설정: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | 앱 설정: `Key` / `Title` / `Password` (앱 수준 독립 비밀번호) |
| `DebugOrigins` | — | 교차 도메인 디버깅 허용 출처 (기본값 없음) |

### 프레임워크 어댑터

| 프레임워크 | 생성자 |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

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
