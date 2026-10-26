![Mascotte d'Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — Plugin de documentation API universel pour Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Présentation du projet

**Version actuelle : v1.1.1** (voir [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** est une bibliothèque de plugin de documentation API universelle pour Go : la documentation des interfaces est déclarée sous forme de **structures typées** en même temps que l'enregistrement des routes — documentation et route naissent ensemble. La documentation est exposée via l'API JSON historique `/api/*` et l'API UIC-1 `/ui/v1/*`, afin que toute interface externe puisse l'afficher et envoyer des requêtes de test — ou une interface Vue empaquetée peut être servie dans le même processus via `Config.UIPath`. La bibliothèque intègre : authentification par mot de passe, gestion multi-applications / multi-versions, données Mock, export JSON / TypeScript, analyse des commentaires et des définitions de protocole en neuf formats, cache de réponses et complétion automatique des paramètres ; les exécuteurs de protocole côté serveur sont compilés à la demande par build tag et couvrent les familles de protocoles HTTP, RPC, socket, messagerie, IoT, industriel et média. Une seule intégration, compatible avec tous les frameworks — sans modifier vos projets existants.

## Fonctionnalités

| # | Fonctionnalité | Description |
|---|----------------|-------------|
| 1 | Génération automatique de documentation | Déclaration Doc typée lors de l'enregistrement de la route ; déclarez une fois, documentation et route naissent ensemble |
| 2 | API de documentation | API historique `/api/*` (menus / protocols / detail / export) et API UIC-1 `/ui/v1/*` (docs, debug, partages, générateurs, export), pour les interfaces externes ou l'interface Vue intégrée |
| 3 | Données Mock | Exemples Mock au niveau des champs, une longueur d'avance dans l'intégration des interfaces |
| 4 | Multi-applications / multi-versions | Gestion arborescente App / Version, un seul plugin couvre toute la documentation du projet |
| 5 | Authentification par mot de passe | Mot de passe global + mot de passe par application, jeton HMAC · comparaison en temps constant |
| 6 | Documentation Markdown | Rendu sécurisé avec goldmark, le HTML natif est automatiquement supprimé |
| 7 | Adaptation multi-frameworks | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, une seule intégration compatible avec tous les frameworks |
| 8 | Export JSON / TypeScript | Exportez les types d'interface en un clic, une intégration frontend-backend plus fluide |
| 9 | Protection de sécurité | Authentification par mot de passe · liste blanche CORS · protection XSS / anti-traversée de chemin · garde SSRF et limites de session/octets pour le débogage côté serveur |
| 10 | Analyse des annotations et définitions | go/ast analyse `@apidoc` / `@protocol` / `@definition` ; neuf formats de définitions intégrés : OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC et XML-RPC |
| 11 | Cache HTTP | Cache de réponses versionné + ETag/304, les endpoints de documentation répondent en millisecondes |
| 12 | Complétion automatique des paramètres | reflect déduit les paramètres de requête à partir de la signature du handler |
| 13 | Annotations pour tout protocole | `@protocol` indique le protocole ; detail renvoie protocol/method/url/params/responses/mock ainsi que les définitions de protocole analysées |
| 14 | Interface Vue intégrée (optionnelle) | Pointez `Config.UIPath` vers un build empaqueté UIC-1 pour servir l'interface Vue dans le même processus ; une valeur vide revient à l'API JSON |
| 15 | Exécuteurs de protocole côté serveur | Sept familles de protocoles compilées par build tag ; les exécuteurs sont désactivés par défaut et échouent en mode fermé lorsqu'ils ne sont pas compilés, `apidoc_debug_all` activant tout |

## Vue d'ensemble de l'architecture

![Architecture du projet](../../svg/architecture.svg)

![Fonctionnalités du projet](../../svg/features.svg)

![Cycle de vie du projet](../../svg/lifecycle.svg)

## Structure du projet

```
apidoc-go/
├── apidoc.go            # Points d'entrée principaux : New / Register / Mount / Handler
├── VERSION              # Version du projet (v1.1.1) ; le workflow de release crée le tag à partir de ce fichier
├── doc.go               # Types Route / Doc / Param / Response
├── model/               # Modèles de données
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   Définitions de protocole externes @definition
│   ├── protocol.go      #   schéma d'URL → protocole
│   └── protocols.go     #   registre des protocoles / alias / ports par défaut
├── store/               # Stockage et déduplication
│   └── store.go         #   Déduplication (app, version, method, url), les derniers écrasent
├── auth/                # Authentification
│   └── auth.go          #   Vérification du mot de passe · Émission et validation des jetons HMAC
├── server/              # API HTTP
│   ├── server.go        #   routes historiques /api · auth / CORS / cache
│   ├── ui.go            #   interface Vue statique UIPath (repli SPA · accès confiné au chemin)
│   └── uiapi/           #   API UIC-1 /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     lectures de documents · analyse des définitions
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     générateurs · modèles de code · partages · cache
│       ├── share.go
│       └── debug/       #     exécuteurs de protocole côté serveur (build tag par famille)
├── adapter/             # Couche d'adaptation des frameworks (interfaces typées)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Analyseur automatique d'annotations
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   neuf analyseurs de définitions de protocole
├── export/              # Exportations
│   └── export.go        #   Définitions d'interfaces TypeScript
├── mock/                # Données Mock
│   └── mock.go          #   Génération d'exemples au niveau des champs
├── example/             # Projet d'exemple (8 frameworks :8081–:8088)
│   ├── main.go
│   └── handlers/        #   Exemples de commentaires @apidoc
└── docs/                # Documentation et ressources
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Diagramme d'architecture / de fonctionnalités / de cycle de vie
```

## Utilisation

### Installation

```bash
go get github.com/erikwang2013/apidoc-go
```

### Démarrage rapide (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "Ma documentation API"})

	// Déclarez la documentation en même temps que la route
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
			Title:   "Salutation",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "Votre nom"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Les interfaces externes lisent /apidoc/api/menus ou l'API UIC-1 /apidoc/ui/v1/* ;
	// avec UIPath défini, ouvrez directement http://localhost:8080/apidoc/
}
```

### Activation de l'interface Vue intégrée UIC-1 et des exécuteurs côté serveur

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Optionnel : la sortie de npm run package:ui d'apidoc-ui
	// (index.html / config.js / assets/).
	// Laissez vide pour ne servir que l'API JSON et continuer à utiliser une interface externe.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// Le relais côté serveur est désactivé par défaut ; une fois activé, il reste soumis
	// à la liste blanche SSRF et aux limites d'octets/session.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Les exécuteurs sont sélectionnés à la compilation par famille de protocoles : `go build -tags apidoc_debug_all` les compile tous, ou limitez-les avec `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` et `apidoc_debug_media`. Les appels à des protocoles non compilés échouent toujours en mode fermé.

### Annotations pour tout protocole

Les interfaces non HTTP réutilisent `@method` + `@url` et précisent le protocole avec `@protocol` (il peut aussi être déduit du schéma de l'URL) :

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Publier la température
// @param qos int false "QoS" in=option default=1
// @param payload json true "Corps du message" in=payload
func PublishTemp() {}
```

`@protocol` accepte la liste complète des protocoles et leurs alias (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket) ; le registre complet des clés, alias, schémas et ports par défaut se trouve à `GET /api/protocols`. Les corps binaires se déclarent avec `@param ... type=binary` (ou `hex`/`base64`) + `in=payload`.

`/api/detail` et l'API UIC-1 `/ui/v1/docDetail` renvoient `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` et les `definitions` analysées. Le débogage dispose de deux canaux : directement depuis le navigateur (HTTP / WebSocket / SSE, toujours disponible) et le relais côté serveur (avec `Debug` activé et le build tag correspondant). Les familles de protocoles côté serveur sont HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), sockets (TCP / UDP / Raw / Unix), messagerie (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industriel (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) et média (RTSP / SIP / RTP / RTCP / RTMP). Les builds par défaut ne compilent aucun exécuteur ; une fois activés, les cibles doivent d'abord passer la garde SSRF (liste blanche `AllowHosts`, adresses privées/loopback/link-local/métadonnées cloud refusées par défaut, validation par IP après résolution DNS avec la connexion épinglée), et sont limités par des délais d'expiration, une limite de 1 Mio par message, le `MaxBytes` de la réponse et des limites de sessions de flux.

### Analyse des définitions de protocole (@definition)

Utilisez `@definition` pour importer des définitions de protocole externes (le format est détecté automatiquement, ou fixez-le avec `format=`) ; le résultat analysé est renvoyé avec detail afin que l'UIC puisse afficher les structures de messages et construire les formulaires de débogage :

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Formats pris en charge : OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC et XML-RPC. `Config.DefinitionRoot` confine les fichiers lisibles à un répertoire racine (le répertoire de travail du processus par défaut) ; l'analyse est limitée à 1 Mio et 50 000 lignes par fichier, et à une profondeur de références imbriquées de 32 ; voir les commentaires du paquet `parse` pour les options telles que `operation=`, `message=`, `direction=`, `sample=` et `inline=`.

### Démarrage rapide (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "Ma documentation API"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "Salutation"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Pour Echo / Chi / Fiber, il suffit de remplacer le constructeur de l'adaptateur : `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — le reste du code reste strictement identique. Pour beego / Kratos / go-zero, voir le tableau « Adaptateurs de framework » ci-dessous.

### Options de configuration

| Option | Valeur par défaut | Description |
|--------|-------------------|-------------|
| `Prefix` | `/apidoc` | Chemin de montage de la documentation |
| `Title` | `API Docs` | Titre de la documentation |
| `Desc` | — | Description de la documentation |
| `GlobalParams` | — | Paramètres globaux, fusionnés dans chaque Action |
| `Auth` | — | Configuration d'authentification : `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Configuration des applications : `Key` / `Title` / `Password` (mot de passe indépendant par application) |
| `DebugOrigins` | — | Origines autorisées à lire l'API de documentation en cross-origin (interface externe, aucune par défaut) |
| `PublicHost` | — | URL de base absolue utilisée par les interfaces externes pour le débogage ; vide signifie même origine |
| `ShareDir` | `apidoc-go-shares` dans le répertoire temporaire | Répertoire des enregistrements de partage UIC-1 |
| `DefinitionRoot` | répertoire de travail du processus | Racine des fichiers `@definition` ; les chemins hors racine et les liens symboliques sont rejetés |
| `Generator` / `CodeTemplates` | — | Générateurs déclaratifs et modèles de code (le rendu est sans effet de bord) |
| `ShareActions` | — | Déclarations d'actions de partage interprétées par l'interface |
| `Debug` | valeur zéro = désactivé | Exécuteurs côté serveur : `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (aucune interface intégrée) | Répertoire d'un build Vue UIC-1 empaqueté ; si non vide, sert l'interface statique dans le même processus sous `{Prefix}/` |

### API UIC-1 et interface Vue intégrée (optionnelle)

La bibliothèque sert par défaut deux API JSON de documentation : l'API historique `/api/*` et l'API UIC-1 `/ui/v1/*`, consommées par une interface externe. `GET {Prefix}/ui/v1/capabilities` renvoie le contrat, la version du backend et la matrice des fonctionnalités (modes de débogage navigateur/serveur, partages, cache, générateurs, export, auth) ; l'interface s'en sert pour déterminer quels panneaux sont disponibles. `/ui/v1/config` renvoie les menus ainsi que la configuration des générateurs et des modèles de code. Après avoir fourni un build empaqueté d'`apidoc-ui` à `Config.UIPath`, le même processus sert aussi directement l'interface Vue sous `{Prefix}/` :

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // répertoire de sortie de scripts/package-ui.sh
})
```

La bibliothèque n'embarque elle-même aucun asset frontend, il n'y a donc aucune dépendance à la compilation ; un paquet de release avec interface intégrée doit pointer `UIPath` par défaut vers le répertoire `dist/uic1-ui/` livré — la configuration ci-dessus est la recommandation. Étapes de déploiement :

1. Compilez et empaquetez dans le dépôt de l'interface : `npm run package:ui` (sortie `dist/uic1-ui/`).
2. Déployez vers le répertoire cible (il doit exister et être vide, ou contenir le paquet uic1-ui précédent) : `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Définissez `UIPath` au démarrage ; laissez-le vide et redémarrez pour revenir au mode API de documentation seule.

Comportement :

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` et `{Prefix}/assets/*` sont servis comme fichiers statiques, le type MIME étant déterminé par l'extension ;
- les chemins inconnus sans extension reviennent à `index.html` (repli SPA) ; les assets manquants `.js` / `.css` et similaires renvoient 404 ;
- `{Prefix}` (sans barre oblique finale) redirige en 307 vers `{Prefix}/` afin que les chemins d'assets relatifs restent corrects ;
- `{Prefix}/api/*` et `{Prefix}/ui/v1/*` ont la priorité ; les chemins d'API inconnus renvoient 404 et ne reviennent jamais à l'interface ;
- l'accès aux fichiers est confiné à `UIPath` (`..` et les liens symboliques hors racine sont rejetés), sans listage de répertoire ; si le répertoire ou `index.html` est manquant, la route de l'interface renvoie 500 et l'API de documentation n'est pas affectée ;
- l'adaptateur go-zero n'enregistre que les routes d'API fixes et ne transmet pas de chemins arbitraires sous le préfixe, l'interface intégrée n'est donc pas disponible avec cet adaptateur (tous les autres adaptateurs la prennent en charge) ;
- les déploiements qui utilisent encore une interface externe n'ont besoin d'aucun changement ; gardez simplement `HTTP.API_PREFIX` dans `config.js` égal au `Prefix` du serveur.

### Exécuteurs de protocole côté serveur (build tags)

Le relais côté serveur ne compile aucun exécuteur par défaut, et l'appel à un protocole non compilé renvoie une erreur en mode fermé. Sélectionnez les build tags par famille de protocoles ; `apidoc_debug_all` les agrège tous :

| Build tag | Famille de protocoles | Protocoles |
|-----------|-----------------------|------------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, https uniquement) |
| `apidoc_debug_sockets` | Sockets | tcp, udp, raw, unix (unaires + streaming) |
| `apidoc_debug_rpc` | RPC / finance | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unaire ; descripteur ou réflexion de serveur) |
| `apidoc_debug_messaging` | Messagerie | mqtt, stomp, amqp, kafka, xmpp (unaires + streaming ; également sélectionnables individuellement avec `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industriel | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Média | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | Tout | toutes les familles ci-dessus |

Frontière de sécurité : `Debug.EnableServerSide` est désactivé par défaut ; une fois activé, les cibles doivent encore passer la liste blanche `AllowHosts` (correspondance par suffixe comme `*.example.com`), les adresses privées/loopback/link-local/CGNAT et de métadonnées cloud sont refusées par défaut (seul `AllowPrivateNetworks` les autorise), la validation par IP a lieu après la résolution DNS avec la connexion épinglée, et `CONNECT`, les redirections et les proxys sont interdits ; les requêtes et réponses sont limitées par `TimeoutMS`, `MaxBytes` (1 Mio par défaut), les sessions de flux (4 par portée), le nombre de trames et les délais d'inactivité. L'analyse des définitions ne lit que des fichiers réguliers dans `DefinitionRoot`, avec des limites de taille de fichier, de nombre de lignes et de profondeur de références.

### Adaptateurs de framework

| Framework | Constructeur |
|-----------|--------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### Analyse automatique des annotations (go/ast)
Écrivez des commentaires `@apidoc` au-dessus d'un handler, puis enregistrez les résultats de l'analyse :

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

### Complétion automatique des paramètres (reflect)
Lorsque `Doc.Params` est vide, Register les déduit par reflect à partir de la signature du handler : les arguments struct sont développés en champs du body (selon les json tags), et les contextes de framework (gin.Context / echo.Context / fiber.Ctx) sont ignorés automatiquement.

### Cache HTTP (cache de réponses + ETag)
Les réponses de `/api/menus`, `/api/protocols` et `/api/export` sont sérialisées une fois par version du document et mises en cache dans le processus. Le cache est préchauffé à la création du `Handler` et invalidé automatiquement par `Register` / `SetAppMeta`. Les réponses portent toujours `ETag` + `Cache-Control: private, max-age=300`, donc les visites répétées reçoivent un 304. Aucune configuration nécessaire.

### Exportation
| Format | Endpoint | Description |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Arborescence complète du projet |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Définitions de types d'interfaces |

### Données Mock
La page de détail affiche automatiquement un exemple Mock : personnalisez-le avec `Doc.Params[].Mock`, ou laissez-le se générer à partir du type du champ (string→"sample", int→0, bool→true, ...).

### Projet d'exemple
`example/` fournit 8 serveurs de frameworks (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Démarrez-les tous avec `go run ./example` ; l'API de documentation historique est servie sur `/apidoc/api/...` de chaque port, l'API UIC-1 sur `/apidoc/ui/v1/...`, et une interface externe ou l'interface Vue intégrée via `UIPath` peuvent les consommer.

## Documentation multilingue

| Langue | Lien |
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

## Soutenez-nous

Si ce projet vous a été utile, n'hésitez pas à nous soutenir en scannant les codes QR pour faire un don — cela nous motive à continuer à maintenir et à mettre à jour le projet !

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="Don WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="Don Alipay" />
  <p>WeChat Pay (à gauche) · Alipay (à droite)</p>
</div>

### Dons par virement bancaire international

**【Informations sur le bénéficiaire】**
Nom du bénéficiaire : WANG KEXUN
Numéro de compte : 881015918251

**【Banque du bénéficiaire】**
ZA Bank
Code SWIFT : AABLHKHHXXX
Nom de la banque : ZA Bank Limited
Code banque : 387
Adresse de la banque : Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Banque correspondante pour virements transfrontaliers (si nécessaire)】**

Veuillez noter qu'il s'agit des informations de la banque correspondante (banque intermédiaire) pour les virements transfrontaliers, et non de celles de la banque du bénéficiaire. Demandez à votre banque émettrice si les informations de la banque correspondante sont requises.

Pour les virements en dollars de Hong Kong, en yuans chinois et en dollars américains, la banque correspondante est Citibank —

Nom de la banque : Citibank N.A. Hong Kong
Code SWIFT : CITIHKHXXXX
Code banque : 006
Nom de l'agence : Hong Kong Branch
Code agence : 391
Adresse de la banque : Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

Pour les virements dans d'autres devises, la banque correspondante est BNY Mellon —

Nom de la banque : THE BANK OF NEW YORK MELLON
Code SWIFT : IRVTUS3NXXX
Adresse de la banque : THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Fait avec ❤️ par <a href="https://erik.xyz">erik.xyz</a></div>
