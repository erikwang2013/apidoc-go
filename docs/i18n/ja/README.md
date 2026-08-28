# apidoc-go — Go 汎用 API ドキュメントプラグイン

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## プロジェクト紹介

**apidoc-go** は、汎用の Go API ドキュメントプラグインライブラリです。インターフェースのドキュメントは**型付き構造体**としてルート登録と同時に宣言され、ドキュメントはルートと共に生まれます。組み込みの Web UI によりオンライン閲覧とオンライン実行（デバッグ）が可能で、パスワード認証、複数アプリ / 複数バージョン管理、Mock データ、JSON / TypeScript エクスポート機能を内蔵しています。一度組み込めば全フレームワークで利用でき、既存プロジェクトを改造する必要はありません。

## プロジェクト機能

| # | 機能 | 説明 |
|---|------|------|
| 1 | ドキュメント自動生成 | 型付き Doc 宣言をルート登録と同時に行う。一度宣言すれば、ドキュメントはルートと共に生まれる |
| 2 | オンライン実行 | ブラウザから直接実 API をリクエスト。サーバー経由の転送なしで、SSRF の心配が根本的にない |
| 3 | Mock データ | フィールド単位の Mock サンプル。API 連携を一歩先取り |
| 4 | 複数アプリ / 複数バージョン | App / Version のツリー管理。1 つのプラグインでプロジェクト全体のドキュメントをカバー |
| 5 | パスワード認証 | グローバルパスワード + アプリ単位パスワード。HMAC トークン・定数時間比較 |
| 6 | Markdown ドキュメント | goldmark による安全なレンダリング。生の HTML は自動的に除去 |
| 7 | 複数フレームワーク対応 | net/http · Gin · Echo · Chi · Fiber。一度組み込めば全フレームワークで利用可能 |
| 8 | JSON / TypeScript エクスポート | インターフェース型をワンクリックでエクスポート。フロントエンドとの連携がよりスムーズに |
| 9 | セキュリティ対策 | SSRF なし・CORS ホワイトリスト制限・XSS 対策・パストラバーサル対策 |

## アーキテクチャ概要

![プロジェクトアーキテクチャ](../../svg/architecture.svg)

![プロジェクト機能](../../svg/features.svg)

![プロジェクトライフサイクル](../../svg/lifecycle.svg)

## プロジェクト構成

```
apidoc-go/
├── apidoc.go            # 中核エントリポイント: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response 型
├── model/               # データモデル
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # 保存と重複排除
│   └── store.go         #   (app, version, method, url) の重複は後勝ちで上書き
├── auth/                # 認証
│   └── auth.go          #   パスワード検証・HMAC トークンの発行と検証
├── server/              # JSON API + 組み込み Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # フレームワーク適合層（型付きインターフェース）
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
└── docs/                # ドキュメントと素材
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # アーキテクチャ図 / 機能図 / ライフサイクル図
```

## 利用方法

### インストール

```bash
go get github.com/erikwang2013/apidoc-go
```

### クイックスタート（net/http）

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "マイ API ドキュメント"})

	// ドキュメントをルートと一緒に宣言
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
			Title:   "挨拶",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "あなたの名前"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// ブラウザで http://localhost:8080/apidoc にアクセス
}
```

### クイックスタート（Gin）

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "マイ API ドキュメント"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "挨拶"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Echo / Chi / Fiber はアダプターのコンストラクターを差し替えるだけです。`adapter.NewEcho(e)`、`adapter.NewChi(mux)`、`adapter.NewFiber(app)` の順で、残りのコードは完全に同じです。

### 設定項目

| 設定項目 | デフォルト値 | 説明 |
|----------|--------------|------|
| `Prefix` | `/apidoc` | ドキュメントのマウントパス |
| `Title` | `API Docs` | ドキュメントのタイトル |
| `Desc` | — | ドキュメントの説明 |
| `GlobalParams` | — | グローバルパラメータ。すべての Action にマージされる |
| `Auth` | — | 認証設定: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | アプリ設定: `Key` / `Title` / `Password`（アプリ単位の独立パスワード） |
| `DebugOrigins` | — | クロスオリジン実行を許可する送信元（デフォルトではなし） |

### フレームワークアダプター

| フレームワーク | コンストラクター |
|----------------|------------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

## 多言語ドキュメント

| 言語 | リンク |
|------|--------|
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

## 応援のお願い

このプロジェクトが役に立ったなら、QR コードをスキャンして投げ銭で応援していただけると、継続的なメンテナンスとアップデートの励みになります！

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat Pay 投げ銭" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay 投げ銭" />
  <p>WeChat Pay（左）・Alipay（右）</p>
</div>

### 国際送金での投げ銭

**【受取人情報】**
受取人氏名：WANG KEXUN
受取口座番号：881015918251

**【受取銀行】**
ZA Bank
SWIFT コード：AABLHKHHXXX
銀行名：ZA Bank Limited
銀行コード：387
銀行住所：Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【国際送金の代理銀行（必要な場合）】**

これは国際送金の代理銀行（中継銀行）の情報であり、受取銀行の情報ではないことにご注意ください。代理銀行の情報が必要かどうかは送金銀行にご確認ください。

香港ドル・人民元・米ドルでの送金時の代理銀行は Citibank です。

銀行名：Citibank N.A. Hong Kong
SWIFT コード：CITIHKHXXXX
銀行コード：006
支店名：Hong Kong Branch
支店コード：391
銀行住所：Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

その他の通貨での送金時の代理銀行は BNY Mellon です。

銀行名：THE BANK OF NEW YORK MELLON
SWIFT コード：IRVTUS3NXXX
銀行住所：THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
