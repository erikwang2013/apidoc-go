# apidoc-go — Plugin universal de documentação de API para Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Sobre o projeto

**apidoc-go** é uma biblioteca de plugin universal de documentação de API para Go: a documentação das interfaces é declarada como **structs tipados** junto com o registro das rotas, de modo que a documentação nasce ao mesmo tempo que a rota. A Web UI integrada oferece navegação e depuração on-line, com autenticação por senha, gestão de múltiplos aplicativos/versões, dados Mock e exportação para JSON / TypeScript, além de parsing automático de anotações, cache HTTP e preenchimento automático de parâmetros. Integre uma única vez e funcionará com todos os frameworks, sem precisar modificar seu projeto existente.

## Funcionalidades

| # | Função | Descrição |
|---|--------|-----------|
| 1 | Geração automática de documentação | Declaração tipada de Doc junto com o registro da rota; declare uma única vez e a documentação nasce junto com a rota |
| 2 | Depuração on-line | Solicite as interfaces reais diretamente do navegador, sem encaminhamento pelo servidor, naturalmente sem SSRF |
| 3 | Dados Mock | Exemplos Mock por campo, adiante-se na integração de interfaces |
| 4 | Múltiplos aplicativos / versões | Gestão hierárquica de App / Version, um único plugin cobre a documentação de todo o projeto |
| 5 | Autenticação por senha | Senha global + senha por aplicativo, HMAC Token · comparação em tempo constante |
| 6 | Documentos Markdown | Renderização segura com goldmark, o HTML nativo é removido automaticamente |
| 7 | Suporte a vários frameworks | net/http · Gin · Echo · Chi · Fiber, integre uma única vez e funcionará com todos os frameworks |
| 8 | Exportação JSON / TypeScript | Exporte os tipos das interfaces com um clique, integração frontend-backend mais fluida |
| 9 | Proteção de segurança | Sem SSRF · CORS restrito por lista de permissões · proteção anti-XSS · proteção contra path traversal |
| 10 | Parsing automático de anotações | go/ast gera a documentação a partir de comentários; basta o marcador `@apidoc` |
| 11 | Cache HTTP | ETag + 304, as respostas da documentação abrem instantaneamente |
| 12 | Preenchimento automático de parâmetros | reflect infere os parâmetros da solicitação a partir da assinatura do handler |

## Visão geral da arquitetura

![Arquitetura do projeto](../../svg/architecture.svg)

![Funcionalidades do projeto](../../svg/features.svg)

![Ciclo de vida do projeto](../../svg/lifecycle.svg)

## Estrutura do projeto

```
apidoc-go/
├── apidoc.go            # Ponto de entrada principal: New / Register / Mount / Handler
├── doc.go               # Tipos Route / Doc / Param / Response
├── model/               # Modelo de dados
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Armazenamento e deduplicação
│   └── store.go         #   Deduplicação por (app, version, method, url); a última sobrescreve
├── auth/                # Autenticação
│   └── auth.go          #   Validação de senha · emissão e verificação de HMAC Token
├── server/              # API JSON + Web UI integrada
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Camada de adaptação de frameworks (interface tipada)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Parser automático de anotações
│   └── parse.go         #   go/ast · marcadores de comentário @apidoc
├── export/              # Exportação
│   └── export.go        #   Definições de interfaces TypeScript
├── mock/                # Dados Mock
│   └── mock.go          #   Geração de exemplos por campo
├── example/             # Projeto de exemplo (5 frameworks :8081–:8085)
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
	// Visite http://localhost:8080/apidoc no navegador
}
```

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

Para Echo / Chi / Fiber basta trocar o construtor do adaptador: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)`; o restante do código é idêntico.

### Opções de configuração

| Opção | Valor padrão | Descrição |
|-------|--------------|-----------|
| `Prefix` | `/apidoc` | Caminho de montagem da documentação |
| `Title` | `API Docs` | Título da documentação |
| `Desc` | — | Descrição da documentação |
| `GlobalParams` | — | Parâmetros globais, mesclados em cada Action |
| `Auth` | — | Configuração de autenticação: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Configuração de aplicativos: `Key` / `Title` / `Password` (senha independente por aplicativo) |
| `DebugOrigins` | — | Origens permitidas para depuração entre domínios (nenhuma por padrão) |

### Adaptadores de frameworks

| Framework | Construtor |
|-----------|------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

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

### Cache HTTP (ETag)

Todos os endpoints da documentação enviam `ETag` + `Cache-Control: private, max-age=300` automaticamente; visitas repetidas recebem 304. Nenhuma configuração é necessária.

### Exportação

| Formato | Endpoint | Descrição |
|---------|----------|-----------|
| JSON | `GET /apidoc/api/export` | Árvore completa do projeto |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Definições de tipos das interfaces |

### Dados Mock

A página de detalhes mostra um exemplo Mock automaticamente: personalize com `Doc.Params[].Mock` ou deixe que seja gerado a partir do tipo do campo (string→"sample", int→0, bool→true, ...).

### Projeto de exemplo

`example/` inclui 5 servidores de framework (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085). Inicie todos com `go run ./example`.

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
