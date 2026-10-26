![Apidoc のマスコット](../../svg/apidoc-pet.svg)

# apidoc-go — Go 汎用 API ドキュメントプラグイン

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## プロジェクト紹介

**現在のバージョン: v1.1.1**（[Releases](https://github.com/erikwang2013/apidoc-go/releases) を参照）

**apidoc-go** は、汎用の Go API ドキュメントプラグインライブラリです。インターフェースのドキュメントは**型付き構造体**としてルート登録と同時に宣言され、ドキュメントはルートと共に生まれます。ドキュメントはレガシーの `/api/*` と UIC-1 の `/ui/v1/*` の両方の JSON API で提供されるため、任意の外部 UI がレンダリングしてテストリクエストを送信でき、パッケージ化された Vue UI は `Config.UIPath` を通じて同一プロセスで提供することもできます。パスワード認証、複数アプリ / 複数バージョン管理、Mock データ、JSON / TypeScript エクスポート、コメントおよび 9 形式のプロトコル定義パース、レスポンスキャッシュ、パラメータ自動補完機能を内蔵しています。サーバー側プロトコル実行エンジンはビルドタグに応じてオンデマンドでコンパイルされ、HTTP、RPC、ソケット、メッセージング、IoT、産業、メディアのプロトコルファミリーをカバーします。一度組み込めば全フレームワークで利用でき、既存プロジェクトを改造する必要はありません。

## プロジェクト機能

| # | 機能 | 説明 |
|---|------|------|
| 1 | ドキュメント自動生成 | 型付き Doc 宣言をルート登録と同時に行う。一度宣言すれば、ドキュメントはルートと共に生まれる |
| 2 | ドキュメント API | レガシーの `/api/*`（menus / protocols / detail / export）と UIC-1 の `/ui/v1/*`（docs、debug、shares、generators、export）。外部 UI またはバンドルされた Vue UI 向け |
| 3 | Mock データ | フィールド単位の Mock サンプル。API 連携を一歩先取り |
| 4 | 複数アプリ / 複数バージョン | App / Version のツリー管理。1 つのプラグインでプロジェクト全体のドキュメントをカバー |
| 5 | パスワード認証 | グローバルパスワード + アプリ単位パスワード。HMAC トークン・定数時間比較 |
| 6 | Markdown ドキュメント | goldmark による安全なレンダリング。生の HTML は自動的に除去 |
| 7 | 複数フレームワーク対応 | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero。一度組み込めば全フレームワークで利用可能 |
| 8 | JSON / TypeScript エクスポート | インターフェース型をワンクリックでエクスポート。フロントエンドとの連携がよりスムーズに |
| 9 | セキュリティ対策 | パスワード認証・CORS ホワイトリスト・XSS / パストラバーサル対策・サーバー側デバッグ用の SSRF ガードとセッション/バイト制限 |
| 10 | アノテーションと定義のパース | go/ast が `@apidoc` / `@protocol` / `@definition` をパース。9 つの内蔵定義形式：OpenAPI、AsyncAPI、GraphQL、proto、WSDL、EDMX、JSON Schema、JSON-RPC、XML-RPC |
| 11 | HTTP キャッシュ | バージョン管理されたレスポンスキャッシュ + ETag/304、ドキュメント API はミリ秒で応答 |
| 12 | パラメータ自動補完 | reflect がハンドラーシグネチャからリクエストパラメータを推論 |
| 13 | 任意プロトコルのアノテーション | `@protocol` でプロトコルを指定。detail が protocol/method/url/params/responses/mock およびパース済みプロトコル定義を返す |
| 14 | バンドル Vue UI（任意） | `Config.UIPath` に UIC-1 のパッケージ済みビルドを指定すると同一プロセスで Vue UI を提供。空の場合は JSON API にフォールバック |
| 15 | サーバー側プロトコル実行エンジン | ビルドタグでコンパイルされる 7 つのプロトコルファミリー。デフォルトは無効で、未コンパイル時は fail closed。`apidoc_debug_all` で全て有効化 |

## アーキテクチャ概要

![プロジェクトアーキテクチャ](../../svg/architecture.svg)

![プロジェクト機能](../../svg/features.svg)

![プロジェクトライフサイクル](../../svg/lifecycle.svg)

## プロジェクト構成

```
apidoc-go/
├── apidoc.go            # 中核エントリポイント: New / Register / Mount / Handler
├── VERSION              # プロジェクトバージョン（v1.1.1）。リリースワークフローがこれを基にタグを作成
├── doc.go               # Route / Doc / Param / Response 型
├── model/               # データモデル
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   @definition 外部プロトコル定義
│   ├── protocol.go      #   URL スキーム → プロトコル
│   └── protocols.go     #   プロトコルレジストリ / エイリアス / デフォルトポート
├── store/               # 保存と重複排除
│   └── store.go         #   (app, version, method, url) の重複は後勝ちで上書き
├── auth/                # 認証
│   └── auth.go          #   パスワード検証・HMAC トークンの発行と検証
├── server/              # HTTP API
│   ├── server.go        #   レガシー /api ルート・認証 / CORS / キャッシュ
│   ├── ui.go            #   UIPath 静的 Vue UI（SPA フォールバック・パス制限）
│   └── uiapi/           #   UIC-1 /ui/v1 API
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     ドキュメント読み取り・定義パース
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators・コードテンプレート・shares・cache
│       ├── share.go
│       └── debug/       #     サーバー側プロトコル実行エンジン（ファミリー別ビルドタグ）
├── adapter/             # フレームワーク適合層（型付きインターフェース）
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # アノテーション自動パーサー
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   9 つのプロトコル定義パーサー
├── export/              # エクスポート
│   └── export.go        #   TypeScript インターフェース定義
├── mock/                # Mock データ
│   └── mock.go          #   フィールド単位のサンプル生成
├── example/             # サンプルプロジェクト（8 フレームワーク :8081–:8088）
│   ├── main.go
│   └── handlers/        #   @apidoc コメントの例
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
	// 外部 UI は /apidoc/api/menus または UIC-1 の /apidoc/ui/v1/* を読み取ります。
	// UIPath を設定すると http://localhost:8080/apidoc/ を直接開けます
}
```

### UIC-1 バンドル Vue UI とサーバー側実行エンジンの有効化

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// 任意: apidoc-ui の npm run package:ui の出力
	// （index.html / config.js / assets/）。
	// 空の場合は JSON API のみを提供し、外部 UI を使い続けます。
	UIPath: "/srv/myapp/dist/uic1-ui",
	// サーバー側リレーはデフォルトで無効です。有効にしても
	// SSRF ホワイトリストとバイト/セッション制限が適用されます。
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

実行エンジンはビルド時にプロトコルファミリー単位で選択されます。`go build -tags apidoc_debug_all` は全てをコンパイルし、`apidoc_debug_http`、`apidoc_debug_sockets`、`apidoc_debug_rpc`、`apidoc_debug_grpc`、`apidoc_debug_http3`、`apidoc_debug_messaging`、`apidoc_debug_iot`、`apidoc_debug_industrial`、`apidoc_debug_media` で必要なものだけに絞れます。コンパイルされていないプロトコルへの呼び出しは常に fail closed で失敗します。

### 任意プロトコルのアノテーション

HTTP 以外のインターフェースは `@method` + `@url` を再利用し、`@protocol` でプロトコルを指定します（URL スキームから推論することもできます）:

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title 温度を発行
// @param qos int false "QoS" in=option default=1
// @param payload json true "メッセージ本文" in=payload
func PublishTemp() {}
```

`@protocol` は完全なプロトコル一覧とエイリアスを受け付けます（HTTP/2、HTTP/3、SOAP、GraphQL、JSON-RPC、XML-RPC、OData、Webhooks、gRPC、Dubbo、Thrift、Hessian、MCP、A2A、ISO 8583、WebSocket、SSE、Socket.IO、WebRTC DataChannel、RTSP、RTMP、HLS、SIP、XMPP、AMQP、MQTT、STOMP、Kafka、CoAP、LwM2M、OPC UA、Modbus TCP、DNP3、IEC 60870-5-104、BACnet/IP、RTP、RTCP、TCP、UDP、Raw Socket、Unix Domain Socket）。key、エイリアス、スキーム、デフォルトポートの完全なレジストリは `GET /api/protocols` にあります。バイナリ body は `@param ... type=binary`（または `hex`/`base64`）+ `in=payload` で宣言します。

`/api/detail` と UIC-1 の `/ui/v1/docDetail` は `protocol`、`protocol_info`、`method`、`url`、`params`、`responses`、`mock` およびパース済みの `definitions` を返します。デバッグには 2 つのチャネルがあります。ブラウザから直接（HTTP / WebSocket / SSE、常に利用可能）と、サーバー側リレー（`Debug` の有効化と対応するビルドタグが必要）です。サーバー側のプロトコルファミリーは HTTP（http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook）、RPC（gRPC / Thrift / Dubbo / Hessian / ISO 8583）、ソケット（TCP / UDP / Raw / Unix）、メッセージング（MQTT / STOMP / AMQP / Kafka / XMPP）、IoT（CoAP / LwM2M）、産業（Modbus / DNP3 / IEC 104 / BACnet / OPC UA）、メディア（RTSP / SIP / RTP / RTCP / RTMP）です。デフォルトのビルドには実行エンジンが一切含まれず、有効化しても対象はまず SSRF ガード（`AllowHosts` ホワイトリスト、プライベート/ループバック/リンクローカル/クラウドメタデータアドレスはデフォルトで拒否、DNS 解決後の IP 単位検証と接続の固定）を通過する必要があり、タイムアウト、メッセージあたり 1 MiB の制限、レスポンスの `MaxBytes`、ストリームセッションの制限が適用されます。

### プロトコル定義のパース (@definition)

`@definition` で外部プロトコル定義を取り込みます（形式は自動検出、または `format=` で指定）。パース結果は detail と共に返され、UIC がメッセージ構造をレンダリングし、デバッグフォームを構築できます:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

対応形式：OpenAPI、AsyncAPI、GraphQL SDL、proto、WSDL、EDMX（OData）、JSON Schema、JSON-RPC、XML-RPC。`Config.DefinitionRoot` は読み取り可能なファイルをルートディレクトリに制限し（デフォルトはプロセスの作業ディレクトリ）、パースはファイルあたり 1 MiB と 50,000 行、ネスト参照の深さ 32 に制限されます。`operation=`、`message=`、`direction=`、`sample=`、`inline=` などのオプションは `parse` パッケージのコメントを参照してください。

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

Echo / Chi / Fiber はアダプターのコンストラクターを差し替えるだけです。`adapter.NewEcho(e)`、`adapter.NewChi(mux)`、`adapter.NewFiber(app)` の順で、残りのコードは完全に同じです。beego / Kratos / go-zero については下の「フレームワークアダプター」表を参照してください。

### 設定項目

| 設定項目 | デフォルト値 | 説明 |
|----------|--------------|------|
| `Prefix` | `/apidoc` | ドキュメントのマウントパス |
| `Title` | `API Docs` | ドキュメントのタイトル |
| `Desc` | — | ドキュメントの説明 |
| `GlobalParams` | — | グローバルパラメータ。すべての Action にマージされる |
| `Auth` | — | 認証設定: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | アプリ設定: `Key` / `Title` / `Password`（アプリ単位の独立パスワード） |
| `DebugOrigins` | — | ドキュメント API のクロスオリジン読み取りを許可する送信元（外部 UI、デフォルトではなし） |
| `PublicHost` | — | 外部 UI がデバッグに使用する絶対ベース URL。空の場合は同一オリジン |
| `ShareDir` | 一時ディレクトリ配下の `apidoc-go-shares` | UIC-1 共有レコードのディレクトリ |
| `DefinitionRoot` | プロセスの作業ディレクトリ | `@definition` のファイルルート。ルート外のパスとシンボリックリンクは拒否 |
| `Generator` / `CodeTemplates` | — | 宣言的ジェネレーターとコードテンプレート（レンダリングは副作用なし） |
| `ShareActions` | — | UI が解釈する共有アクションの宣言 |
| `Debug` | ゼロ値 = 無効 | サーバー側実行エンジン: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | —（バンドル UI なし） | パッケージ済み UIC-1 Vue ビルドのディレクトリ。値がある場合は `{Prefix}/` 配下で同一プロセスから静的 UI を提供 |

### UIC-1 API とバンドル Vue UI（任意）

ライブラリはデフォルトで 2 つの JSON ドキュメント API を提供します。レガシーの `/api/*` と UIC-1 の `/ui/v1/*` で、外部 UI がこれを利用します。`GET {Prefix}/ui/v1/capabilities` はコントラクト、バックエンドバージョン、機能マトリックス（ブラウザ/サーバーデバッグモード、shares、cache、generators、export、auth）を返し、UI はこれを使って利用可能なパネルを決定します。`/ui/v1/config` はメニューとジェネレーター・コードテンプレートの設定を返します。パッケージ済みの `apidoc-ui` ビルドを `Config.UIPath` に渡すと、同一プロセスが `{Prefix}/` 配下で Vue UI も直接提供します:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // scripts/package-ui.sh の出力ディレクトリ
})
```

ライブラリ自体はフロントエンドアセットを埋め込まないため、ビルド時の依存関係はありません。バンドル UI のリリースパッケージでは、`UIPath` を同梱の `dist/uic1-ui/` ディレクトリにデフォルト設定することを推奨します。上記の配線が推奨構成です。デプロイ手順:

1. UI リポジトリでビルドとパッケージング: `npm run package:ui`（出力 `dist/uic1-ui/`）。
2. 対象ディレクトリにデプロイ（ディレクトリは存在し、空であるか以前の uic1-ui パッケージが入っている必要があります）: `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`。
3. 起動時に `UIPath` を設定。空にして再起動するとドキュメント API 専用モードにロールバックします。

動作:

- `{Prefix}/`、`{Prefix}/index.html`、`{Prefix}/config.js`、`{Prefix}/assets/*` は拡張子に応じた MIME タイプで静的ファイルとして提供されます。
- 拡張子のない未知のパスは `index.html` にフォールバックし（SPA フォールバック）、存在しない `.js` / `.css` などのアセットは 404 を返します。
- `{Prefix}`（末尾スラッシュなし）は相対アセットパスが正しくなるよう 307 で `{Prefix}/` にリダイレクトされます。
- `{Prefix}/api/*` と `{Prefix}/ui/v1/*` が優先され、未知の API パスは 404 を返し UI にフォールバックしません。
- ファイルアクセスは `UIPath` に制限され（`..` とルート外のシンボリックリンクは拒否）、ディレクトリ一覧は提供されません。ディレクトリや `index.html` がない場合、UI ルートは 500 を返し、ドキュメント API は影響を受けません。
- go-zero アダプターは固定 API ルートのみを登録し、プレフィックス配下の任意パスを転送しないため、このアダプターではバンドル UI を利用できません（他のアダプターはすべて対応）。
- 外部 UI を使い続けるデプロイでは変更は不要です。`config.js` の `HTTP.API_PREFIX` をサーバーの `Prefix` と一致させておくだけです。

### サーバー側プロトコル実行エンジン（ビルドタグ）

サーバー側リレーはデフォルトで実行エンジンをコンパイルしません。コンパイルされていないプロトコルを呼び出すと fail-closed エラーを返します。プロトコルファミリー別にビルドタグを選択し、`apidoc_debug_all` はその全てを集約します:

| ビルドタグ | プロトコルファミリー | プロトコル |
|-----------|-----------------|-----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3（QUIC、https のみ） |
| `apidoc_debug_sockets` | ソケット | tcp, udp, raw, unix（単項 + ストリーミング） |
| `apidoc_debug_rpc` | RPC / 金融 | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc（単項、ディスクリプターまたはサーバーリフレクション） |
| `apidoc_debug_messaging` | メッセージング | mqtt, stomp, amqp, kafka, xmpp（単項 + ストリーミング、`apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp` で個別選択も可能） |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | 産業 | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | メディア | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | 全て | 上記の全ファミリー |

セキュリティ境界: `Debug.EnableServerSide` はデフォルトで無効です。有効にしても対象は `AllowHosts` ホワイトリスト（`*.example.com` のようなサフィックス一致）を通過する必要があり、プライベート/ループバック/リンクローカル/CGNAT およびクラウドメタデータアドレスはデフォルトで拒否されます（`AllowPrivateNetworks` のみが許可）。DNS 解決後の IP 単位検証が接続の固定と共に行われ、`CONNECT`、リダイレクト、プロキシは禁止されます。リクエストとレスポンスは `TimeoutMS`、`MaxBytes`（デフォルト 1 MiB）、ストリームセッション（スコープあたり 4）、フレーム数、アイドルタイムアウトで制限されます。定義パースは `DefinitionRoot` 内の通常ファイルのみを読み取り、ファイルサイズ、行数、参照深さで制限されます。

### フレームワークアダプター

| フレームワーク | コンストラクター |
|----------------|------------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### アノテーション自動パース (go/ast)

ハンドラーの上に `@apidoc` コメントを書き、パース結果を登録します:

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

### パラメータ自動補完 (reflect)

`Doc.Params` が空の場合、Register はハンドラーのシグネチャから reflect でパラメータを推論します。struct 引数は本文フィールドに展開され（json タグに従う）、フレームワークのコンテキスト（gin.Context / echo.Context / fiber.Ctx）は自動的にスキップされます。

### HTTP キャッシュ (レスポンスキャッシュ + ETag)

`/api/menus`、`/api/protocols`、`/api/export` のレスポンスはドキュメントのバージョンごとに一度だけシリアライズされ、プロセス内にキャッシュされます。`Handler` の作成時にウォームアップされ、`Register` / `SetAppMeta` で自動的に無効化されます。レスポンスには引き続き `ETag` + `Cache-Control: private, max-age=300` が付与され、再訪時は 304 になります。設定は不要です。

### エクスポート

| 形式 | エンドポイント | 説明 |
|------|----------|-------------|
| JSON | `GET /apidoc/api/export` | プロジェクト全体のツリー |
| TypeScript | `GET /apidoc/api/export?format=typescript` | インターフェースの型定義 |

### Mock データ

詳細ページに Mock の例が自動で表示されます。`Doc.Params[].Mock` でカスタマイズするか、フィールドの型から自動生成されます（string→"sample", int→0, bool→true, ...）。

### サンプルプロジェクト

`example/` には 8 つのフレームワークサーバー（net/http :8081、Gin :8082、Echo :8083、Chi :8084、Fiber :8085、beego :8086、Kratos :8087、go-zero :8088）が同梱されています。`go run ./example` で全部起動でき、レガシーのドキュメント API は各ポートの `/apidoc/api/...`、UIC-1 API は `/apidoc/ui/v1/...` にあり、外部 UI または `UIPath` 経由のバンドル Vue UI が利用できます。

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
