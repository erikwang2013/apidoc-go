# apidoc-go — Go জেনেরিক API ডকুমেন্টেশন প্লাগইন

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## প্রকল্প পরিচিতি

**apidoc-go** একটি জেনেরিক Go API ডকুমেন্টেশন প্লাগইন লাইব্রেরি: ইন্টারফেস ডকুমেন্টেশন **টাইপযুক্ত স্ট্রাক্ট** হিসেবে রুট রেজিস্ট্রেশনের সাথে একসাথে ঘোষণা করা হয় — ডকুমেন্টেশন ও রুট একসাথে জন্ম নেয়; এমবেডেড Web UI অনলাইন ব্রাউজিং ও অনলাইন ডিবাগিং সুবিধা দেয় এবং এতে পাসওয়ার্ড অথেনটিকেশন, মাল্টি-অ্যাপ / মাল্টি-ভার্সন ম্যানেজমেন্ট, Mock ডেটা এবং JSON / TypeScript এক্সপোর্ট ক্ষমতা অন্তর্নির্মিত। একবার সংযুক্ত করলেই সব ফ্রেমওয়ার্কে কাজ করে — বিদ্যমান প্রজেক্টে কোনো পরিবর্তনের প্রয়োজন নেই।

## প্রকল্পের বৈশিষ্ট্য

| # | বৈশিষ্ট্য | বিবরণ |
|---|------|------|
| 1 | স্বয়ংক্রিয় ডকুমেন্টেশন জেনারেশন | টাইপযুক্ত Doc ঘোষণা, রুট রেজিস্ট্রেশনের সাথে; এক জায়গায় ঘোষণা, ডকুমেন্টেশন ও রুট একসাথে |
| 2 | অনলাইন ডিবাগিং | ব্রাউজার থেকে সরাসরি বাস্তব ইন্টারফেসে অনুরোধ; সার্ভার-সাইড ফরওয়ার্ডিং নেই, স্বাভাবিকভাবেই SSRF নেই |
| 3 | Mock ডেটা | ফিল্ড-স্তরের Mock উদাহরণ; ইন্টারফেস ইন্টিগ্রেশন দ্রুততর |
| 4 | মাল্টি-অ্যাপ / মাল্টি-ভার্সন | App / Version ট্রি-ভিত্তিক ম্যানেজমেন্ট; একটি প্লাগইনে পুরো প্রজেক্টের ডকুমেন্টেশন |
| 5 | পাসওয়ার্ড অথেনটিকেশন | গ্লোবাল পাসওয়ার্ড + অ্যাপ-স্তরের পাসওয়ার্ড; HMAC Token · কনস্ট্যান্ট-টাইম তুলনা |
| 6 | Markdown ডকুমেন্টেশন | goldmark নিরাপদ রেন্ডারিং; নেটিভ HTML স্বয়ংক্রিয়ভাবে অপসারিত |
| 7 | মাল্টি-ফ্রেমওয়ার্ক অ্যাডাপ্টেশন | net/http · Gin · Echo · Chi · Fiber; একবার সংযুক্ত, সব ফ্রেমওয়ার্কে কার্যকর |
| 8 | JSON / TypeScript এক্সপোর্ট | ইন্টারফেস টাইপ এক ক্লিকে এক্সপোর্ট; ফ্রন্ট-ব্যাক ইন্টিগ্রেশন আরও সাবলীল |
| 9 | নিরাপত্তা সুরক্ষা | SSRF নেই · CORS হোয়াইটলিস্ট সীমাবদ্ধতা · XSS প্রতিরোধ · পাথ ট্রাভার্সাল প্রতিরোধ |

## আর্কিটেকচার ওভারভিউ

![প্রজেক্ট আর্কিটেকচার](../../docs/svg/architecture.svg)

![প্রজেক্টের বৈশিষ্ট্য](../../docs/svg/features.svg)

![প্রজেক্ট লাইফসাইকেল](../../docs/svg/lifecycle.svg)

## প্রকল্প কাঠামো

```
apidoc-go/
├── apidoc.go            # মূল এন্ট্রি পয়েন্ট: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response টাইপ
├── model/               # ডেটা মডেল
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # স্টোরেজ ও ডি-ডুপ্লিকেশন
│   └── store.go         #   (app, version, method, url) ডি-ডুপ্লিকেশন, পরেরটি আগেরটিকে ওভাররাইট করে
├── auth/                # অথেনটিকেশন
│   └── auth.go          #   পাসওয়ার্ড যাচাই · HMAC Token ইস্যু ও যাচাই
├── server/              # JSON API + এমবেডেড Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # ফ্রেমওয়ার্ক অ্যাডাপ্টেশন লেয়ার (টাইপযুক্ত ইন্টারফেস)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
└── docs/                # ডকুমেন্টেশন ও সম্পদ
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # আর্কিটেকচার ডায়াগ্রাম / ফিচার ডায়াগ্রাম / লাইফসাইকেল ডায়াগ্রাম
```

## ব্যবহার নির্দেশিকা

### ইনস্টলেশন

```bash
go get github.com/erikwang2013/apidoc-go
```

### দ্রুত শুরু (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "আমার API ডকুমেন্টেশন"})

	// ডকুমেন্টেশন এবং রুট একসাথে ঘোষণা করা হয়
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
			Title:   "অভিবাদন",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "আপনার নাম"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// ব্রাউজারে http://localhost:8080/apidoc খুলুন
}
```

### দ্রুত শুরু (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "আমার API ডকুমেন্টেশন"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "অভিবাদন"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Echo / Chi / Fiber-এর ক্ষেত্রে শুধু অ্যাডাপ্টার কনস্ট্রাক্টর পরিবর্তন করুন: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — বাকি কোড সম্পূর্ণ একই থাকে।

### কনফিগ অপশন

| অপশন | ডিফল্ট | বিবরণ |
|--------|--------|------|
| `Prefix` | `/apidoc` | ডকুমেন্টেশন মাউন্ট পাথ |
| `Title` | `API Docs` | ডকুমেন্টেশন শিরোনাম |
| `Desc` | — | ডকুমেন্টেশন বিবরণ |
| `GlobalParams` | — | গ্লোবাল প্যারামিটার, প্রতিটি Action-এ মিশে যায় |
| `Auth` | — | অথেনটিকেশন কনফিগ: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | অ্যাপ কনফিগ: `Key` / `Title` / `Password` (অ্যাপ-স্তরের স্বাধীন পাসওয়ার্ড) |
| `DebugOrigins` | — | ক্রস-অরিজিন ডিবাগিংয়ের অনুমতিপ্রাপ্ত অরিজিন (ডিফল্ট: কিছুই নেই) |

### ফ্রেমওয়ার্ক অ্যাডাপ্টার

| ফ্রেমওয়ার্ক | কনস্ট্রাক্টর | সংস্করণ |
|------|--------|------|
| net/http | `adapter.NewNetHTTP(mux)` | স্ট্যান্ডার্ড লাইব্রেরি |
| Gin | `adapter.NewGin(engine)` | v1.10.0 |
| Echo | `adapter.NewEcho(e)` | v4.12.0 |
| Chi | `adapter.NewChi(mux)` | v5.3.2 |
| Fiber | `adapter.NewFiber(app)` | v2.52.5 |

## বহুভাষিক ডকুমেন্টেশন

| ভাষা | লিংক |
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

## আমাদের সমর্থন করুন

প্রকল্পটি আপনার কাজে লাগলে অনুগ্রহ করে স্ক্যান করে ডোনেশন করুন — এতে আমাদের এটি বজায় রাখার এবং নিয়মিত আপডেট করার উৎসাহ জাগবে!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat Pay ডোনেশন" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay ডোনেশন" />
  <p>WeChat Pay (বামে) · Alipay (ডানে)</p>
</div>

### গ্লোবাল ট্রান্সফার ডোনেশন

**【প্রাপকের তথ্য】**
প্রাপকের নাম: WANG KEXUN
প্রাপকের অ্যাকাউন্ট নম্বর: 881015918251

**【প্রাপক ব্যাংক】**
ZA Bank
SWIFT কোড: AABLHKHHXXX
ব্যাংকের নাম: ZA Bank Limited
ব্যাংক কোড: 387
ব্যাংকের ঠিকানা: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【ক্রস-বর্ডার রেমিট্যান্স করেসপন্ডেন্ট ব্যাংক (যদি প্রয়োজন হয়)】**

অনুগ্রহ করে লক্ষ্য করুন: এটি ক্রস-বর্ডার রেমিট্যান্স করেসপন্ডেন্ট (মধ্যস্থতাকারী) ব্যাংকের তথ্য, প্রাপক ব্যাংকের তথ্য নয়। আপনার রেমিটিং ব্যাংককে জিজ্ঞাসা করুন এই তথ্য প্রদান করা প্রয়োজন কিনা।

হংকং ডলার, রেনমিনবি এবং মার্কিন ডলার জমার জন্য করেসপন্ডেন্ট ব্যাংক হলো Citibank ——

ব্যাংকের নাম: Citibank N.A. Hong Kong
SWIFT কোড: CITIHKHXXXX
ব্যাংক কোড: 006
শাখার নাম: Hong Kong Branch
শাখা কোড: 391
ব্যাংকের ঠিকানা: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

অন্যান্য মুদ্রার জন্য করেসপন্ডেন্ট ব্যাংক হলো BNY Mellon ——

ব্যাংকের নাম: THE BANK OF NEW YORK MELLON
SWIFT কোড: IRVTUS3NXXX
ব্যাংকের ঠিকানা: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
