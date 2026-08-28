# apidoc-go — ملحق توثيق واجهات برمجة التطبيقات العام للغة Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## نبذة عن المشروع

**apidoc-go** هي مكتبة إضافات عامة لتوثيق واجهات برمجة التطبيقات (API) للغة Go: تُعلَن وثائق الواجهات في **هياكل مصنّفة الأنواع (typed structs)** في نفس وقت تسجيل المسارات، فتُولَد الوثائق مع المسارات معًا؛ وتتضمن واجهة ويب مدمجة للتصفح والاختبار المباشر عبر الإنترنت، مع مصادقة بكلمة مرور، وإدارة متعددة التطبيقات/الإصدارات، وبيانات Mock، وقدرة تصدير JSON / TypeScript، والتحليل التلقائي للتعليقات التوضيحية، والتخزين المؤقت HTTP، والإكمال التلقائي للمعاملات. تكامل واحد يغطي جميع الأطر، دون الحاجة إلى تعديل مشروعك الحالي.

## ميزات المشروع

| # | الميزة | الوصف |
|---|------|------|
| 1 | توليد تلقائي للوثائق | إعلان Doc مصنّف الأنواع، يُسجَّل مع المسار؛ إعلان واحد يولّد الوثائق مع المسار |
| 2 | اختبار مباشر عبر الإنترنت | يُرسَل الطلب من المتصفح مباشرة إلى الواجهة الحقيقية دون وساطة من الخادم، فلا SSRF بطبيعته |
| 3 | بيانات Mock | أمثلة Mock على مستوى الحقل، لتسريع تكامل الواجهات |
| 4 | تطبيقات / إصدارات متعددة | إدارة شجرية لـ App / Version، ومكوّن إضافي واحد يغطي وثائق المشروع كله |
| 5 | مصادقة بكلمة مرور | كلمة مرور عامة + كلمة مرور لكل تطبيق، رمز HMAC Token · مقارنة زمنية ثابتة |
| 6 | وثائق Markdown | عرض آمن عبر goldmark، مع إزالة HTML الأصلي تلقائيًا |
| 7 | تكيّف مع أطر متعددة | net/http · Gin · Echo · Chi · Fiber، تكامل واحد يغطي كل الأطر |
| 8 | تصدير JSON / TypeScript | تصدير أنواع الواجهات بضغطة واحدة، لتنسيق التكامل بين الواجهة الأمامية والخلفية |
| 9 | الحماية الأمنية | بلا SSRF · تقييد CORS بالقائمة البيضاء · منع XSS · منع تجاوز المسارات |
| 10 | التحليل التلقائي للتعليقات التوضيحية | يولّد go/ast الوثائق من التعليقات؛ ويكفي وسم `@apidoc` |
| 11 | تخزين HTTP مؤقت | ETag + 304، استجابات التوثيق تُفتح فورًا |
| 12 | إكمال تلقائي للمعاملات | يستنتج reflect معاملات الطلب من توقيع المعالج (handler) |

## نظرة عامة على البنية

![بنية المشروع](../../docs/svg/architecture.svg)

![ميزات المشروع](../../docs/svg/features.svg)

![دورة حياة المشروع](../../docs/svg/lifecycle.svg)

## بنية المشروع

```
apidoc-go/
├── apidoc.go            # نقطة الدخول الأساسية: New / Register / Mount / Handler
├── doc.go               # أنواع Route / Doc / Param / Response
├── model/               # نموذج البيانات
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # التخزين وإزالة التكرار
│   └── store.go         #   إزالة التكرار حسب (app, version, method, url)، والأخير يحل محل السابق
├── auth/                # المصادقة
│   └── auth.go          #   التحقق من كلمة المرور · إصدار رمز HMAC Token والتحقق منه
├── server/              # JSON API + واجهة ويب مدمجة
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # طبقة محولات الأطر (واجهات مصنّفة الأنواع)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # المحلل التلقائي للتعليقات التوضيحية
│   └── parse.go         #   go/ast · وسوم تعليقات @apidoc
├── export/              # التصدير
│   └── export.go        #   تعريفات واجهات TypeScript
├── mock/                # بيانات Mock
│   └── mock.go          #   توليد أمثلة على مستوى الحقل
├── example/             # مشروع نموذجي (5 أطر :8081–:8085)
│   ├── main.go
│   └── handlers/        #   أمثلة تعليقات @apidoc
└── docs/                # الوثائق والمواد
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # مخطط البنية / مخطط الميزات / مخطط دورة الحياة
```

## دليل الاستخدام

### التركيب

```bash
go get github.com/erikwang2013/apidoc-go
```

### البدء السريع (net/http)

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

	// الإعلان عن التوثيق مع المسار معًا
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
	// افتح http://localhost:8080/apidoc في المتصفح
}
```

### البدء السريع (Gin)

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

بالنسبة إلى Echo / Chi / Fiber يكفي استبدال مُنشئ المحول: `adapter.NewEcho(e)` و `adapter.NewChi(mux)` و `adapter.NewFiber(app)`، وبقية الكود متطابق تمامًا.

### خيارات الإعداد

| خيار الإعداد | القيمة الافتراضية | الوصف |
|--------|--------|------|
| `Prefix` | `/apidoc` | مسار تركيب الوثائق |
| `Title` | `API Docs` | عنوان الوثائق |
| `Desc` | — | وصف الوثائق |
| `GlobalParams` | — | معاملات عامة تُدمج في كل Action |
| `Auth` | — | إعدادات المصادقة: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | إعدادات التطبيقات: `Key` / `Title` / `Password` (كلمة مرور مستقلة لكل تطبيق) |
| `DebugOrigins` | — | المصادر المسموح بها للاختبار عبر النطاقات (بلا افتراضيًا) |

### محولات الأطر

| الإطار | المُنشئ |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

### التحليل التلقائي للتعليقات التوضيحية (go/ast)
اكتب تعليقات `@apidoc` فوق المعالج (handler)، ثم سجّل نتائج التحليل:

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

### الإكمال التلقائي للمعاملات (reflect)
عندما يكون `Doc.Params` فارغًا، يستنتج Register المعاملات عبر reflect من توقيع المعالج: تُوسَّع وسائط البنية (struct) إلى حقول body (وفق json tags)، وتُتخطى سياقات الأطر (gin.Context / echo.Context / fiber.Ctx) تلقائيًا.

### التخزين المؤقت HTTP (ETag)
جميع نقاط نهاية التوثيق تحمل `ETag` + `Cache-Control: private, max-age=300` تلقائيًا؛ وتعود الزيارات المتكررة بـ 304. لا حاجة إلى أي إعداد.

### التصدير
| الصيغة | نقطة النهاية | الوصف |
|--------|----------|------|
| JSON | `GET /apidoc/api/export` | شجرة المشروع الكاملة |
| TypeScript | `GET /apidoc/api/export?format=typescript` | تعريفات أنواع الواجهات |

### بيانات Mock
تعرض صفحة التفاصيل مثال Mock تلقائيًا: خصّصه عبر `Doc.Params[].Mock`، أو اتركه يُولَّد من نوع الحقل (string→"sample"، int→0، bool→true، ...).

### المشروع النموذجي
يضم `example/` خمسة خوادم أطر (net/http :8081، Gin :8082، Echo :8083، Chi :8084، Fiber :8085). شغّلها كلها عبر `go run ./example`.

## وثائق متعددة اللغات

| اللغة | الرابط |
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

## ادعمنا

إذا كان هذا المشروع مفيدًا لك، فمرحبًا بك في مسح رمز الاستجابة السريعة (QR) للتبرع ودعمنا، فهذا يمنحنا الحافز لمواصلة الصيانة والتحديث!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="التبرع عبر WeChat Pay" />
  <img src="../../alipay.png" width="130" height="130" alt="التبرع عبر Alipay" />
  <p>WeChat Pay (يسار) · Alipay (يمين)</p>
</div>

### التبرع عبر التحويل البنكي الدولي

**【معلومات المستفيد】**
Beneficiary Name: WANG KEXUN
Account Number: 881015918251

**【البنك المستفيد】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Bank Name: ZA Bank Limited
Bank Code: 387
Bank Address: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【البنك الوكيل للتحويلات الدولية (إن لزم)】**

يرجى الانتباه: هذه معلومات البنك الوكيل للتحويلات الدولية (البنك الوسيط)، وليست معلومات البنك المستفيد. يُرجى الاستفسار من البنك المُرسِل عما إذا كان يلزم تقديم معلومات البنك الوسيط.

البنك الوكيل للتحويلات بالدولار الهونغ كونغي واليوان الصيني والدولار الأمريكي هو Citibank ——

Bank Name: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Bank Code: 006
Branch Name: Hong Kong Branch
Branch Code: 391
Bank Address: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

أما التحويلات بغيرها من العملات فالبنك الوكيل لها هو BNY Mellon ——

Bank Name: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Bank Address: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">صُنع بـ ❤️ بواسطة <a href="https://erik.xyz">erik.xyz</a></div>
