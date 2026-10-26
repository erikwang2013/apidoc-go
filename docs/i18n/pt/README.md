![Mascote do Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — Plugin universal de documentação de API para Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Sobre o projeto

**Versão atual: v1.1.1** (consulte [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** é uma biblioteca de plugin universal de documentação de API para Go: a documentação das interfaces é declarada como **structs tipados** junto com o registro das rotas, de modo que a documentação nasce ao mesmo tempo que a rota. A documentação é servida tanto pela API JSON legada `/api/*` quanto pela UIC-1 `/ui/v1/*`, para que qualquer interface externa possa renderizá-la e enviar requisições de teste — ou uma interface Vue empacotada possa ser servida no mesmo processo via `Config.UIPath`. Inclui: autenticação por senha, gestão de múltiplos aplicativos/versões, dados Mock, exportação JSON / TypeScript, parsing de comentários e de definições de protocolo em nove formatos, cache de respostas e preenchimento automático de parâmetros; os executores de protocolo do lado do servidor são compilados sob demanda por build tag e cobrem as famílias de protocolos HTTP, RPC, socket, mensageria, IoT, industrial e mídia. Integre uma única vez e funcionará com todos os frameworks — sem precisar modificar seu projeto existente.

## Funcionalidades

| # | Função | Descrição |
|---|--------|-----------|
| 1 | Geração automática de documentação | Declaração tipada de Doc junto com o registro da rota; declare uma única vez e a documentação nasce junto com a rota |
| 2 | API de documentação | `/api/*` legada (menus / protocols / detail / export) e UIC-1 `/ui/v1/*` (docs, debug, compartilhamentos, geradores, export), para interfaces externas ou para a interface Vue integrada |
| 3 | Dados Mock | Exemplos Mock por campo, adiante-se na integração de interfaces |
| 4 | Múltiplos aplicativos / versões | Gestão hierárquica de App / Version, um único plugin cobre a documentação de todo o projeto |
| 5 | Autenticação por senha | Senha global + senha por aplicativo, HMAC Token · comparação em tempo constante |
| 6 | Documentos Markdown | Renderização segura com goldmark, o HTML nativo é removido automaticamente |
| 7 | Suporte a vários frameworks | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, integre uma única vez e funcionará com todos os frameworks |
| 8 | Exportação JSON / TypeScript | Exporte os tipos das interfaces com um clique, integração frontend-backend mais fluida |
| 9 | Proteção de segurança | Autenticação por senha · CORS restrito por lista de permissões · proteção anti-XSS · proteção contra path traversal · proteção SSRF e limites de sessão/bytes para depuração no servidor |
| 10 | Parsing de anotações e definições | go/ast faz parsing de `@apidoc` / `@protocol` / `@definition`; nove formatos de definição integrados: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC e XML-RPC |
| 11 | Cache HTTP | Cache de respostas versionado + ETag/304, endpoints de documentação respondem em milissegundos |
| 12 | Preenchimento automático de parâmetros | reflect infere os parâmetros da solicitação a partir da assinatura do handler |
| 13 | Anotações para qualquer protocolo | `@protocol` marca o protocolo; detail retorna protocol/method/url/params/responses/mock além das definições de protocolo analisadas |
| 14 | Interface Vue integrada (opcional) | Aponte `Config.UIPath` para um build empacotado da UIC-1 para servir a interface Vue no mesmo processo; um valor vazio volta à API JSON |
| 15 | Executores de protocolo no servidor | Sete famílias de protocolos compiladas por build tag; desativados por padrão e fail closed quando não compilados, com `apidoc_debug_all` habilitando tudo |

## Visão geral da arquitetura

![Arquitetura do projeto](../../svg/architecture.svg)

![Funcionalidades do projeto](../../svg/features.svg)

![Ciclo de vida do projeto](../../svg/lifecycle.svg)

## Estrutura do projeto

```
apidoc-go/
├── apidoc.go            # Ponto de entrada principal: New / Register / Mount / Handler
├── VERSION              # Versão do projeto (v1.1.1); o workflow de release cria a tag a partir dela
├── doc.go               # Tipos Route / Doc / Param / Response
├── model/               # Modelo de dados
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Definições de protocolo externas @definition
│   ├── protocol.go      #   esquema de URL → protocolo
│   └── protocols.go     #   registro de protocolos / aliases / portas padrão
├── store/               # Armazenamento e deduplicação
│   └── store.go         #   Deduplicação por (app, version, method, url); a última sobrescreve
├── auth/                # Autenticação
│   └── auth.go          #   Validação de senha · emissão e verificação de HMAC Token
├── server/              # API HTTP
│   ├── server.go        #   rotas /api legadas · auth / CORS / cache
│   ├── ui.go            #   interface Vue estática via UIPath (fallback SPA · acesso restrito ao caminho)
│   └── uiapi/           #   API UIC-1 /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     leitura de documentos · parsing de definições
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     geradores · templates de código · compartilhamentos · cache
│       ├── share.go
│       └── debug/       #     executores de protocolo no servidor (build tag por família)
├── adapter/             # Camada de adaptação de frameworks (interface tipada)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Parser automático de anotações
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   nove parsers de definições de protocolo
├── export/              # Exportação
│   └── export.go        #   Definições de interfaces TypeScript
├── mock/                # Dados Mock
│   └── mock.go          #   Geração de exemplos por campo
├── example/             # Projeto de exemplo (8 frameworks :8081–:8088)
│   ├── main.go
│   └── handlers/        #   Exemplos de comentários @apidoc
└── docs/                # Documentação e recursos
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Diagramas: arquitetura / funcionalidades / ciclo de vida
```

## Como usar

### Instalação

```bash
go get github.com/erikwang2013/apidoc-go
```

### Início rápido (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "Minha documentação de API"})

	// A documentação é declarada junto com a rota
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
			Title:   "Saudação",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "Seu nome"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Interfaces externas leem /apidoc/api/menus ou a UIC-1 /apidoc/ui/v1/*;
	// com UIPath definido, abra http://localhost:8080/apidoc/ diretamente
}
```

### Habilitando a interface Vue UIC-1 integrada e os executores no servidor

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Opcional: a saída de `npm run package:ui` do apidoc-ui
	// (index.html / config.js / assets/).
	// Deixe vazio para servir apenas a API JSON e continuar usando uma interface externa.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// O relay no servidor vem desativado por padrão; quando habilitado, ainda está sujeito
	// à lista de permissões SSRF e aos limites de bytes/sessão.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Os executores são selecionados em tempo de compilação por família de protocolo: `go build -tags apidoc_debug_all` compila todos, ou restrinja com `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` e `apidoc_debug_media`. Chamadas a protocolos que não foram compilados sempre falham de forma fechada (fail closed).

### Anotações para qualquer protocolo

As interfaces não HTTP reutilizam `@method` + `@url` e indicam o protocolo com `@protocol` (também pode ser inferido do esquema da URL):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Publicar temperatura
// @param qos int false "QoS" in=option default=1
// @param payload json true "Corpo da mensagem" in=payload
func PublishTemp() {}
```

`@protocol` aceita a lista completa de protocolos e seus aliases (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); o registro completo de chave, alias, esquema e porta padrão está em `GET /api/protocols`. Corpos binários são declarados com `@param ... type=binary` (ou `hex`/`base64`) + `in=payload`.

`/api/detail` e a UIC-1 `/ui/v1/docDetail` retornam `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` e as `definitions` analisadas. A depuração tem dois canais: direto do navegador (HTTP / WebSocket / SSE, sempre disponível) e relay no servidor (com `Debug` habilitado e a build tag correspondente). As famílias de protocolo no servidor são HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), sockets (TCP / UDP / Raw / Unix), mensageria (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industrial (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) e mídia (RTSP / SIP / RTP / RTCP / RTMP). Builds padrão não compilam nenhum executor; quando habilitados, os alvos precisam primeiro passar pela proteção SSRF (lista de permissões `AllowHosts`, endereços privados/loopback/link-local/metadados de nuvem negados por padrão, validação por IP após a resolução DNS com a conexão fixada) e são limitados por timeouts, um limite de 1 MiB por mensagem, o `MaxBytes` da resposta e limites de sessão de streaming.

### Parsing de definições de protocolo (@definition)

Use `@definition` para incorporar definições de protocolo externas (o formato é detectado automaticamente, ou defina-o com `format=`); o resultado analisado é retornado junto com detail para que a UIC possa renderizar as estruturas das mensagens e montar formulários de depuração:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Formatos suportados: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC e XML-RPC. `Config.DefinitionRoot` restringe os arquivos legíveis a um diretório raiz (o diretório de trabalho do processo por padrão); o parsing é limitado a 1 MiB e 50.000 linhas por arquivo e a uma profundidade de referência aninhada de 32; consulte os comentários do pacote `parse` para opções como `operation=`, `message=`, `direction=`, `sample=` e `inline=`.

### Início rápido (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "Minha documentação de API"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "Saudação"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Para Echo / Chi / Fiber basta trocar o construtor do adaptador: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)`; o restante do código é idêntico. Para beego / Kratos / go-zero, consulte a tabela «Adaptadores de frameworks» abaixo.

### Opções de configuração

| Opção | Valor padrão | Descrição |
|-------|--------------|-----------|
| `Prefix` | `/apidoc` | Caminho de montagem da documentação |
| `Title` | `API Docs` | Título da documentação |
| `Desc` | — | Descrição da documentação |
| `GlobalParams` | — | Parâmetros globais, mesclados em cada Action |
| `Auth` | — | Configuração de autenticação: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Configuração de aplicativos: `Key` / `Title` / `Password` (senha independente por aplicativo) |
| `DebugOrigins` | — | Origens permitidas para ler a API de documentação entre domínios (interface externa, nenhuma por padrão) |
| `PublicHost` | — | URL base absoluta usada por interfaces externas para depuração; vazio significa mesma origem |
| `ShareDir` | `apidoc-go-shares` no diretório temporário | Diretório de registros de compartilhamento da UIC-1 |
| `DefinitionRoot` | diretório de trabalho do processo | Raiz de arquivos de `@definition`; caminhos fora da raiz e symlinks são rejeitados |
| `Generator` / `CodeTemplates` | — | Geradores declarativos e templates de código (a renderização não tem efeitos colaterais) |
| `ShareActions` | — | Declarações de ações de compartilhamento interpretadas pela UI |
| `Debug` | valor zero = desativado | Executores no servidor: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (sem interface integrada) | Diretório de um build Vue UIC-1 empacotado; quando não vazio, serve a interface estática no mesmo processo em `{Prefix}/` |

### API UIC-1 e interface Vue integrada (opcional)

A biblioteca serve por padrão duas APIs JSON de documentação: a legada `/api/*` e a UIC-1 `/ui/v1/*`, consumidas por uma interface externa. `GET {Prefix}/ui/v1/capabilities` retorna o contrato, a versão do backend e a matriz de funcionalidades (modos de depuração no navegador/servidor, compartilhamentos, cache, geradores, exportação, autenticação); a UI usa isso para decidir quais painéis estão disponíveis. `/ui/v1/config` retorna os menus mais a configuração de geradores e templates de código. Depois de informar um build empacotado do `apidoc-ui` em `Config.UIPath`, o mesmo processo também serve a interface Vue diretamente em `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // diretório de saída de scripts/package-ui.sh
})
```

A própria biblioteca não incorpora assets de frontend, portanto não há dependência em tempo de build; um pacote de release com a interface integrada deve apontar `UIPath` por padrão para o diretório `dist/uic1-ui/` distribuído — a ligação acima é a configuração recomendada. Etapas de implantação:

1. Compile e empacote no repositório da UI: `npm run package:ui` (saída em `dist/uic1-ui/`).
2. Implante no diretório de destino (ele deve existir e estar vazio, ou conter o pacote uic1-ui anterior): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Defina `UIPath` na inicialização; deixe-o vazio e reinicie para voltar ao modo somente API de documentação.

Comportamento:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` e `{Prefix}/assets/*` são servidos como arquivos estáticos, com o tipo MIME determinado pela extensão;
- caminhos sem extensão desconhecidos caem para `index.html` (fallback de SPA); assets ausentes como `.js` / `.css` e similares retornam 404;
- `{Prefix}` (sem barra final) redireciona com 307 para `{Prefix}/` para que os caminhos relativos dos assets permaneçam corretos;
- `{Prefix}/api/*` e `{Prefix}/ui/v1/*` têm precedência; caminhos de API desconhecidos retornam 404 e nunca caem para a UI;
- o acesso a arquivos fica restrito a `UIPath` (`..` e symlinks fora da raiz são rejeitados), sem listagem de diretórios; se o diretório ou o `index.html` estiver ausente, a rota da UI retorna 500 e a API de documentação não é afetada;
- o adaptador go-zero registra apenas as rotas fixas da API e não encaminha caminhos arbitrários sob o prefixo, portanto a interface integrada não está disponível com esse adaptador (todos os outros adaptadores a suportam);
- implantações que ainda usam uma interface externa não precisam de alterações; basta manter `HTTP.API_PREFIX` em `config.js` igual ao `Prefix` do servidor.

### Executores de protocolo no servidor (build tags)

O relay no servidor não compila nenhum executor por padrão, e chamar um protocolo não compilado retorna um erro fail closed. Selecione as build tags por família de protocolo; `apidoc_debug_all` agrega todas:

| Build tag | Família de protocolo | Protocolos |
|-----------|----------------------|------------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, somente https) |
| `apidoc_debug_sockets` | Sockets | tcp, udp, raw, unix (unário + streaming) |
| `apidoc_debug_rpc` | RPC / financeiro | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unário; descriptor ou reflexão do servidor) |
| `apidoc_debug_messaging` | Mensageria | mqtt, stomp, amqp, kafka, xmpp (unário + streaming; também selecionáveis individualmente com `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industrial | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Mídia | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Todas | todas as famílias acima |

Limite de segurança: `Debug.EnableServerSide` vem desativado por padrão; quando habilitado, os alvos ainda precisam passar pela lista de permissões `AllowHosts` (correspondência por sufixo como `*.example.com`), endereços privados/loopback/link-local/CGNAT e de metadados de nuvem são negados por padrão (somente `AllowPrivateNetworks` os permite), a validação por IP ocorre após a resolução DNS com a conexão fixada, e `CONNECT`, redirecionamentos e proxies são proibidos; requisições e respostas são limitadas por `TimeoutMS`, `MaxBytes` (1 MiB por padrão), sessões de streaming (4 por escopo), contagem de frames e timeouts de inatividade. O parsing de definições lê apenas arquivos regulares dentro de `DefinitionRoot`, limitado por tamanho de arquivo, número de linhas e profundidade de referência.

### Adaptadores de frameworks

| Framework | Construtor |
|-----------|------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### Parsing automático de anotações (go/ast)

Escreva comentários `@apidoc` acima de um handler e registre os resultados do parsing:

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

### Preenchimento automático de parâmetros (reflect)

Quando `Doc.Params` está vazio, o Register infere os parâmetros com reflect a partir da assinatura do handler: argumentos struct são expandidos em campos do corpo (seguindo as tags json) e contextos de framework (gin.Context / echo.Context / fiber.Ctx) são ignorados automaticamente.

### Cache HTTP (cache de respostas + ETag)

As respostas de `/api/menus`, `/api/protocols` e `/api/export` são serializadas uma vez por versão do documento e armazenadas em cache no processo. O cache é pré-aquecido na criação do `Handler` e invalidado automaticamente por `Register` / `SetAppMeta`. As respostas continuam carregando `ETag` + `Cache-Control: private, max-age=300`, então visitas repetidas recebem 304. Nenhuma configuração é necessária.

### Exportação

| Formato | Endpoint | Descrição |
|---------|----------|-----------|
| JSON | `GET /apidoc/api/export` | Árvore completa do projeto |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Definições de tipos das interfaces |

### Dados Mock

A página de detalhes mostra um exemplo Mock automaticamente: personalize com `Doc.Params[].Mock` ou deixe que seja gerado a partir do tipo do campo (string→"sample", int→0, bool→true, ...).

### Projeto de exemplo

`example/` inclui 8 servidores de framework (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Inicie todos com `go run ./example`; a API de documentação legada é servida em `/apidoc/api/...` em cada porta, a API UIC-1 em `/apidoc/ui/v1/...`, e tanto uma interface externa quanto a interface Vue integrada via `UIPath` podem consumi-las.

## Documentação multilíngue

| Idioma | Link |
|--------|------|
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

## Apoie-nos

Se este projeto foi útil para você, fique à vontade para escanear os QR codes e nos apoiar com uma gorjeta — isso nos motiva a continuar mantendo e atualizando!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="Gorjeta com WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="Gorjeta com Alipay" />
  <p>WeChat Pay (esquerda) · Alipay (direita)</p>
</div>

### Doações por transferência bancária internacional

**【Dados do beneficiário】**
Nome do beneficiário: WANG KEXUN
Número da conta: 881015918251

**【Banco beneficiário】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Nome do banco: ZA Bank Limited
Código do banco: 387
Endereço do banco: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Banco correspondente para remessas transfronteiriças (se necessário)】**

Observe que estas são as informações do banco correspondente (intermediário) para remessas transfronteiriças, e não as do banco beneficiário. Consulte seu banco emissor para saber se é necessário fornecer as informações do banco correspondente.

Para remessas em dólar de Hong Kong, renminbi e dólar americano, o banco correspondente é o Citibank —

Nome do banco: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Código do banco: 006
Nome da agência: Hong Kong Branch
Código da agência: 391
Endereço do banco: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Para remessas em outras moedas, o banco correspondente é o BNY Mellon —

Nome do banco: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Endereço do banco: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
