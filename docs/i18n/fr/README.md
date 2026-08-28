# apidoc-go — Plugin de documentation API universel pour Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Présentation du projet

**apidoc-go** est une bibliothèque de plugin de documentation API universelle pour Go : la documentation des interfaces est déclarée sous forme de **structures typées** en même temps que l'enregistrement des routes — documentation et route naissent ensemble. L'interface Web intégrée permet la consultation et le débogage en ligne, avec authentification par mot de passe, gestion multi-applications / multi-versions, données Mock et export JSON / TypeScript. Une seule intégration, compatible avec tous les frameworks — sans modifier vos projets existants.

## Fonctionnalités

| # | Fonctionnalité | Description |
|---|----------------|-------------|
| 1 | Génération automatique de documentation | Déclaration Doc typée lors de l'enregistrement de la route ; déclarez une fois, documentation et route naissent ensemble |
| 2 | Débogage en ligne | Appelez les vrais endpoints directement depuis le navigateur, sans relais côté serveur, naturellement sans SSRF |
| 3 | Données Mock | Exemples Mock au niveau des champs, une longueur d'avance dans l'intégration des interfaces |
| 4 | Multi-applications / multi-versions | Gestion arborescente App / Version, un seul plugin couvre toute la documentation du projet |
| 5 | Authentification par mot de passe | Mot de passe global + mot de passe par application, jeton HMAC · comparaison en temps constant |
| 6 | Documentation Markdown | Rendu sécurisé avec goldmark, le HTML natif est automatiquement supprimé |
| 7 | Adaptation multi-frameworks | net/http · Gin · Echo · Chi · Fiber, une seule intégration compatible avec tous les frameworks |
| 8 | Export JSON / TypeScript | Exportez les types d'interface en un clic, une intégration frontend-backend plus fluide |
| 9 | Protection de sécurité | Pas de SSRF · restriction par liste blanche CORS · protection XSS · protection anti-traversée de chemin |

## Vue d'ensemble de l'architecture

![Architecture du projet](../../svg/architecture.svg)

![Fonctionnalités du projet](../../svg/features.svg)

![Cycle de vie du projet](../../svg/lifecycle.svg)

## Structure du projet

```
apidoc-go/
├── apidoc.go            # Points d'entrée principaux : New / Register / Mount / Handler
├── doc.go               # Types Route / Doc / Param / Response
├── model/               # Modèles de données
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Stockage et déduplication
│   └── store.go         #   Déduplication (app, version, method, url), les derniers écrasent
├── auth/                # Authentification
│   └── auth.go          #   Vérification du mot de passe · Émission et validation des jetons HMAC
├── server/              # API JSON + interface Web intégrée
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Couche d'adaptation des frameworks (interfaces typées)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
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
	// Ouvrez http://localhost:8080/apidoc dans le navigateur
}
```

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

Pour Echo / Chi / Fiber, il suffit de remplacer le constructeur de l'adaptateur : `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — le reste du code reste strictement identique.

### Options de configuration

| Option | Valeur par défaut | Description |
|--------|-------------------|-------------|
| `Prefix` | `/apidoc` | Chemin de montage de la documentation |
| `Title` | `API Docs` | Titre de la documentation |
| `Desc` | — | Description de la documentation |
| `GlobalParams` | — | Paramètres globaux, fusionnés dans chaque Action |
| `Auth` | — | Configuration d'authentification : `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | Configuration des applications : `Key` / `Title` / `Password` (mot de passe indépendant par application) |
| `DebugOrigins` | — | Origines cross-origin autorisées pour le débogage (aucune par défaut) |

### Adaptateurs de framework

| Framework | Constructeur |
|-----------|--------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

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
