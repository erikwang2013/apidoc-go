![تميمة Apidoc](../../svg/apidoc-pet.svg)

# apidoc-go — ملحق توثيق واجهات برمجة التطبيقات العام للغة Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## نبذة عن المشروع

**الإصدار الحالي: v1.1.1** (راجع [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** هي مكتبة إضافات عامة لتوثيق واجهات برمجة التطبيقات (API) للغة Go: تُعلَن وثائق الواجهات في **هياكل مصنّفة الأنواع (typed structs)** في نفس وقت تسجيل المسارات، فتُولَد الوثائق مع المسارات معًا. تُقدَّم الوثائق عبر واجهتي JSON API: القديمة `/api/*` وواجهة UIC-1 `/ui/v1/*`، لتتمكن أي واجهة خارجية من عرضها وإرسال طلبات الاختبار — أو يمكن تقديم واجهة Vue مغلّفة في العملية نفسها عبر `Config.UIPath`. مدمج فيها: مصادقة بكلمة مرور، وإدارة متعددة التطبيقات/الإصدارات، وبيانات Mock، وتصدير JSON / TypeScript، وتحليل التعليقات التوضيحية وتعريفات البروتوكول بصيغها التسع، والتخزين المؤقت للاستجابات، والإكمال التلقائي للمعاملات؛ وتُصرَّف منفّذات البروتوكول من جهة الخادم عند الطلب حسب وسم البناء (build tag) وتغطي عائلات بروتوكولات HTTP وRPC والمقابس (socket) والمراسلة وإنترنت الأشياء (IoT) والصناعة والوسائط. تكامل واحد يغطي جميع الأطر، دون الحاجة إلى تعديل مشروعك الحالي.

## ميزات المشروع

| # | الميزة | الوصف |
|---|------|------|
| 1 | توليد تلقائي للوثائق | إعلان Doc مصنّف الأنواع، يُسجَّل مع المسار؛ إعلان واحد يولّد الوثائق مع المسار |
| 2 | API التوثيق | واجهة `/api/*` القديمة (menus / protocols / detail / export) وواجهة UIC-1 `/ui/v1/*` (docs وdebug وshares وgenerators وexport)، لواجهة خارجية أو واجهة Vue المرفقة |
| 3 | بيانات Mock | أمثلة Mock على مستوى الحقل، لتسريع تكامل الواجهات |
| 4 | تطبيقات / إصدارات متعددة | إدارة شجرية لـ App / Version، ومكوّن إضافي واحد يغطي وثائق المشروع كله |
| 5 | مصادقة بكلمة مرور | كلمة مرور عامة + كلمة مرور لكل تطبيق، رمز HMAC Token · مقارنة زمنية ثابتة |
| 6 | وثائق Markdown | عرض آمن عبر goldmark، مع إزالة HTML الأصلي تلقائيًا |
| 7 | تكيّف مع أطر متعددة | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero، تكامل واحد يغطي كل الأطر |
| 8 | تصدير JSON / TypeScript | تصدير أنواع الواجهات بضغطة واحدة، لتنسيق التكامل بين الواجهة الأمامية والخلفية |
| 9 | الحماية الأمنية | مصادقة بكلمة مرور · تقييد CORS بالقائمة البيضاء · منع XSS ومنع تجاوز المسارات · حارس SSRF وحدود الجلسات/البايتات لتصحيح الأخطاء من جهة الخادم |
| 10 | تحليل التعليقات التوضيحية والتعريفات | يحلل go/ast وسوم `@apidoc` / `@protocol` / `@definition`؛ وتسع صيغ تعريفات مدمجة: OpenAPI وAsyncAPI وGraphQL وproto وWSDL وEDMX وJSON Schema وJSON-RPC وXML-RPC |
| 11 | تخزين HTTP مؤقت | ذاكرة استجابات مُصدَّرة حسب إصدار المستند + ETag/304، استجابات التوثيق بالمللي ثانية |
| 12 | إكمال تلقائي للمعاملات | يستنتج reflect معاملات الطلب من توقيع المعالج (handler) |
| 13 | تعليقات توضيحية لأي بروتوكول | يحدد `@protocol` البروتوكول؛ ويعيد detail القيم protocol/method/url/params/responses/mock إضافةً إلى تعريفات البروتوكول المحللة |
| 14 | واجهة Vue مرفقة (اختيارية) | وجّه `Config.UIPath` إلى حزمة UIC-1 المبنية لتقديم واجهة Vue في العملية نفسها؛ والقيمة الفارغة تعود إلى JSON API |
| 15 | منفّذات البروتوكول من جهة الخادم | سبع عائلات بروتوكولات تُصرَّف حسب وسم البناء؛ معطّلة افتراضيًا وتفشل بشكل مغلق عند عدم تصريفها، مع `apidoc_debug_all` لتفعيل كل شيء |

## نظرة عامة على البنية

![بنية المشروع](../../svg/architecture.svg)

![ميزات المشروع](../../svg/features.svg)

![دورة حياة المشروع](../../svg/lifecycle.svg)

## بنية المشروع

```
apidoc-go/
├── apidoc.go            # نقطة الدخول الأساسية: New / Register / Mount / Handler
├── VERSION              # إصدار المشروع (v1.1.1)؛ يعتمد عليه سير عمل الإصدار في إنشاء الوسوم
├── doc.go               # أنواع Route / Doc / Param / Response
├── model/               # نموذج البيانات
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   تعريفات بروتوكول خارجية @definition
│   ├── protocol.go      #   مخطط URL → البروتوكول
│   └── protocols.go     #   سجل البروتوكولات / الأسماء البديلة / المنافذ الافتراضية
├── store/               # التخزين وإزالة التكرار
│   └── store.go         #   إزالة التكرار حسب (app, version, method, url)، والأخير يحل محل السابق
├── auth/                # المصادقة
│   └── auth.go          #   التحقق من كلمة المرور · إصدار رمز HMAC Token والتحقق منه
├── server/              # واجهة HTTP API
│   ├── server.go        #   مسارات /api القديمة · المصادقة / CORS / التخزين المؤقت
│   ├── ui.go            #   واجهة Vue الثابتة عبر UIPath (SPA fallback · محصورة داخل المسار)
│   └── uiapi/           #   واجهة UIC-1 /ui/v1
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     قراءة المستندات · تحليل التعريفات
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators · code templates · shares · cache
│       ├── share.go
│       └── debug/       #     منفّذات البروتوكول من جهة الخادم (build-tag لكل عائلة)
├── adapter/             # طبقة محولات الأطر (واجهات مصنّفة الأنواع)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # المحلل التلقائي للتعليقات التوضيحية
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   تسعة محللات لتعريفات البروتوكول
├── export/              # التصدير
│   └── export.go        #   تعريفات واجهات TypeScript
├── mock/                # بيانات Mock
│   └── mock.go          #   توليد أمثلة على مستوى الحقل
├── example/             # مشروع نموذجي (8 أطر :8081–:8088)
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
	// تقرأ الواجهات الخارجية /apidoc/api/menus أو UIC-1 /apidoc/ui/v1/*؛
	// وعند تعيين UIPath، افتح http://localhost:8080/apidoc/ مباشرةً
}
```

### تفعيل واجهة Vue المرفقة UIC-1 ومنفّذات البروتوكول من جهة الخادم

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// اختياري: ناتج npm run package:ui من apidoc-ui
	// (index.html / config.js / assets/).
	// اتركه فارغًا لتقديم JSON API فقط والاستمرار في استخدام واجهة خارجية.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// الترحيل من جهة الخادم معطّل افتراضيًا؛ وعند تفعيله يظل خاضعًا
	// للقائمة البيضاء SSRF وحدود البايتات/الجلسات.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

تُختار المنفّذات وقت البناء حسب عائلة البروتوكول: `go build -tags apidoc_debug_all` يُصرِّفها كلها، أو قلّلها عبر `apidoc_debug_http` و`apidoc_debug_sockets` و`apidoc_debug_rpc` و`apidoc_debug_grpc` و`apidoc_debug_http3` و`apidoc_debug_messaging` و`apidoc_debug_iot` و`apidoc_debug_industrial` و`apidoc_debug_media`. والاستدعاءات إلى بروتوكولات لم تُصرَّف تفشل دائمًا بشكل مغلق.

### تعليقات توضيحية لأي بروتوكول

تعيد الواجهات غير HTTP استخدام `@method` + `@url`، وتحدد البروتوكول عبر `@protocol` (ويمكن استنتاجه أيضًا من مخطط URL):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title نشر درجة الحرارة
// @param qos int false "QoS" in=option default=1
// @param payload json true "جسم الرسالة" in=payload
func PublishTemp() {}
```

يقبل `@protocol` قائمة البروتوكولات الكاملة وأسمائها البديلة (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket)؛ السجل الكامل للمفاتيح والأسماء البديلة والمخططات والمنافذ الافتراضية متاح عبر `GET /api/protocols`. تُعلن أجسام الرسائل الثنائية باستخدام `@param ... type=binary` (أو `hex`/`base64`) + `in=payload`.

يعيد `/api/detail` وUIC-1 `/ui/v1/docDetail` القيم `protocol` و`protocol_info` و`method` و`url` و`params` و`responses` و`mock` و`definitions` المحللة. وللتصحيح قناتان: مباشرة من المتصفح (HTTP / WebSocket / SSE، متاحة دائمًا) والترحيل من جهة الخادم (مع تفعيل `Debug` ووسم البناء المطابق). عائلات البروتوكولات من جهة الخادم هي HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook) وRPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583) والمقابس (TCP / UDP / Raw / Unix) والمراسلة (MQTT / STOMP / AMQP / Kafka / XMPP) وإنترنت الأشياء (CoAP / LwM2M) والصناعة (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) والوسائط (RTSP / SIP / RTP / RTCP / RTMP). لا تصرّف عمليات البناء الافتراضية أي منفّذ؛ وعند التفعيل يجب أن تجتاز الأهداف أولًا حارس SSRF (القائمة البيضاء `AllowHosts`، مع رفض عناوين private/loopback/link-local/cloud-metadata افتراضيًا، والتحقق لكل IP بعد حل DNS مع تثبيت الاتصال)، وتكون محدودة بالمهلات الزمنية (timeouts) وحد 1 MiB لكل رسالة و`MaxBytes` للاستجابة وحدود جلسات البث.

### تحليل تعريفات البروتوكول (@definition)

استخدم `@definition` لإدراج تعريفات بروتوكول خارجية (تُكتشف الصيغة تلقائيًا، أو حددها عبر `format=`)؛ وتُعاد النتيجة المحللة مع detail لتتمكن UIC من عرض بنى الرسائل وبناء نماذج التصحيح:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

الصيغ المدعومة: OpenAPI وAsyncAPI وGraphQL SDL وproto وWSDL وEDMX (OData) وJSON Schema وJSON-RPC وXML-RPC. يحصر `Config.DefinitionRoot` الملفات القابلة للقراءة داخل دليل جذري (دليل عمل العملية افتراضيًا)؛ ويقتصر التحليل على 1 MiB و50,000 سطر لكل ملف وعمق مراجع متداخلة قدره 32؛ راجع تعليقات حزمة `parse` لخيارات مثل `operation=` و`message=` و`direction=` و`sample=` و`inline=`.

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

بالنسبة إلى Echo / Chi / Fiber يكفي استبدال مُنشئ المحول: `adapter.NewEcho(e)` و `adapter.NewChi(mux)` و `adapter.NewFiber(app)`، وبقية الكود متطابق تمامًا. أما beego / Kratos / go-zero فراجع جدول محولات الأطر أدناه.

### خيارات الإعداد

| خيار الإعداد | القيمة الافتراضية | الوصف |
|--------|--------|------|
| `Prefix` | `/apidoc` | مسار تركيب الوثائق |
| `Title` | `API Docs` | عنوان الوثائق |
| `Desc` | — | وصف الوثائق |
| `GlobalParams` | — | معاملات عامة تُدمج في كل Action |
| `Auth` | — | إعدادات المصادقة: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | إعدادات التطبيقات: `Key` / `Title` / `Password` (كلمة مرور مستقلة لكل تطبيق) |
| `DebugOrigins` | — | المصادر المسموح لها بقراءة API التوثيق عبر النطاقات (الواجهة الخارجية، بلا افتراضيًا) |
| `PublicHost` | — | عنوان URL أساسي مطلق تستخدمه الواجهات الخارجية للتصحيح؛ والقيمة الفارغة تعني الأصل نفسه (same-origin) |
| `ShareDir` | `apidoc-go-shares` تحت الدليل المؤقت | دليل سجلات المشاركة (share) الخاص بـ UIC-1 |
| `DefinitionRoot` | دليل عمل العملية | جذر ملفات `@definition`؛ تُرفض المسارات الخارجة عن الجذر والروابط الرمزية |
| `Generator` / `CodeTemplates` | — | مولّدات تعريفية وقوالب كود (التوليد بلا آثار جانبية) |
| `ShareActions` | — | إعلانات إجراءات المشاركة التي تفسرها الواجهة |
| `Debug` | القيمة الصفرية = معطّل | منفّذات من جهة الخادم: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (بلا واجهة مرفقة) | دليل حزمة Vue مبنية من UIC-1؛ وعندما لا يكون فارغًا، تُقدَّم الواجهة الثابتة في العملية نفسها تحت `{Prefix}/` |

### واجهة UIC-1 وواجهة Vue المرفقة (اختياري)

تقدّم المكتبة واجهتي JSON API للتوثيق افتراضيًا: `/api/*` القديمة وUIC-1 `/ui/v1/*`، وتستهلكهما واجهة خارجية. تعيد `GET {Prefix}/ui/v1/capabilities` العقد وإصدار الواجهة الخلفية ومصفوفة الميزات (أوضاع التصحيح في المتصفح/الخادم، والمشاركات، والتخزين المؤقت، والمولّدات، والتصدير، والمصادقة)؛ وتستخدمها الواجهة لتحديد اللوحات المتاحة. ويعيد `/ui/v1/config` القوائم إضافةً إلى إعدادات المولّدات وقوالب الكود. وبعد تمرير حزمة `apidoc-ui` المبنية إلى `Config.UIPath`، تقدم العملية نفسها واجهة Vue مباشرة تحت `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // مجلد مخرجات scripts/package-ui.sh
})
```

لا تضمّ المكتبة نفسها أصول الواجهة الأمامية، فلا يوجد اعتماد وقت البناء؛ ويُستحسن أن تشير حزمة إصدار الواجهة المرفقة إلى دليل `dist/uic1-ui/` المرفق عبر `UIPath` افتراضيًا — والربط أعلاه هو الإعداد الموصى به. خطوات النشر:

1. البناء والتغليف في مستودع الواجهة: `npm run package:ui` (المخرجات `dist/uic1-ui/`).
2. النشر إلى الدليل الهدف (يجب أن يكون موجودًا وفارغًا، أو أن يحتوي على حزمة uic1-ui السابقة): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. عيّن `UIPath` عند بدء التشغيل؛ واتركه فارغًا وأعد التشغيل للتراجع إلى وضع API التوثيق فقط.

السلوك:

- تُقدَّم `{Prefix}/` و`{Prefix}/index.html` و`{Prefix}/config.js` و`{Prefix}/assets/*` كملفات ثابتة، ويُحدَّد نوع MIME حسب الامتداد؛
- المسارات غير المعروفة التي بلا امتداد ترجع إلى `index.html` (SPA fallback)؛ والأصول المفقودة مثل `.js` / `.css` ترجع 404؛
- يعيد `{Prefix}` (بدون شرطة مائلة زائدة) التوجيه 307 إلى `{Prefix}/` لتبقى مسارات الأصول النسبية صحيحة؛
- تكون `{Prefix}/api/*` و`{Prefix}/ui/v1/*` لها الأولوية؛ ومسارات API غير المعروفة ترجع 404 ولا ترجع أبدًا إلى الواجهة؛
- يقتصر الوصول إلى الملفات على `UIPath` (تُرفض `..` والروابط الرمزية الخارجة عن الجذر)، دون سرد أدلة؛ وإذا كان الدليل أو `index.html` مفقودًا يرجع مسار الواجهة 500 ولا يتأثر API التوثيق؛
- لا يسجّل محول go-zero إلا مسارات API الثابتة ولا يمرر مسارات عشوائية تحت البادئة، لذا لا تتوفر الواجهة المرفقة مع ذلك المحول (جميع المحولات الأخرى تدعمها)؛
- عمليات النشر التي لا تزال تستخدم واجهة خارجية لا تحتاج إلى أي تغييرات؛ فقط أبقِ `HTTP.API_PREFIX` في `config.js` مساويًا لـ `Prefix` في الخادم.

### منفّذات البروتوكول من جهة الخادم (وسوم البناء)

لا يصرّف الترحيل من جهة الخادم أي منفّذات افتراضيًا، واستدعاء بروتوكول غير مصرَّف يعيد خطأً مغلقًا (fail-closed). اختر وسوم البناء حسب عائلة البروتوكول؛ ويجمع `apidoc_debug_all` جميعها:

| وسم البناء | عائلة البروتوكول | البروتوكولات |
|-----------|-----------------|-----------|
| `apidoc_debug_http` | HTTP / الويب | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC، https فقط) |
| `apidoc_debug_sockets` | المقابس | tcp, udp, raw, unix (أحادي + بث) |
| `apidoc_debug_rpc` | RPC / مالية | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (أحادي؛ descriptor أو server reflection) |
| `apidoc_debug_messaging` | المراسلة | mqtt, stomp, amqp, kafka, xmpp (أحادي + بث؛ ويمكن اختيارها منفردة أيضًا عبر `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | إنترنت الأشياء | coap, lwm2m |
| `apidoc_debug_industrial` | الصناعة | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | الوسائط | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | الكل | كل العائلات أعلاه |

حدود الأمان: يكون `Debug.EnableServerSide` معطّلًا افتراضيًا؛ وعند تفعيله يجب أن تجتاز الأهداف القائمة البيضاء `AllowHosts` (مطابقة اللاحقة مثل `*.example.com`)، وتُرفض عناوين private/loopback/link-local/CGNAT وcloud-metadata افتراضيًا (ولا يسمح بها إلا `AllowPrivateNetworks`)، ويجري التحقق لكل IP بعد حل DNS مع تثبيت الاتصال، وتُحظر `CONNECT` وإعادة التوجيه والوكلاء؛ وتكون الطلبات والاستجابات محدودة بـ `TimeoutMS` و`MaxBytes` (1 MiB افتراضيًا) وجلسات البث (4 لكل سياق) وعدد الإطارات ومُهلات الخمول. لا يقرأ تحليل التعريفات إلا الملفات العادية داخل `DefinitionRoot`، محدودًا بحجم الملف وعدد الأسطر وعمق المراجع.

### محولات الأطر

| الإطار | المُنشئ |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

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

### التخزين المؤقت HTTP (ذاكرة الاستجابات + ETag)
تُخزَّن استجابات `/api/menus` و`/api/protocols` و`/api/export` في الذاكرة بعد تسلسلها مرة واحدة لكل إصدار من المستند، وتُهيَّأ عند إنشاء `Handler` وتُبطَل تلقائيًا بعد `Register` / `SetAppMeta`. تظل الاستجابات تحمل `ETag` + `Cache-Control: private, max-age=300`، لذلك تحصل الزيارات المتكررة على 304. لا حاجة إلى أي إعداد.

### التصدير
| الصيغة | نقطة النهاية | الوصف |
|--------|----------|------|
| JSON | `GET /apidoc/api/export` | شجرة المشروع الكاملة |
| TypeScript | `GET /apidoc/api/export?format=typescript` | تعريفات أنواع الواجهات |

### بيانات Mock
تعرض صفحة التفاصيل مثال Mock تلقائيًا: خصّصه عبر `Doc.Params[].Mock`، أو اتركه يُولَّد من نوع الحقل (string→"sample"، int→0، bool→true، ...).

### المشروع النموذجي
يضم `example/` ثمانية خوادم أطر (net/http :8081، Gin :8082، Echo :8083، Chi :8084، Fiber :8085، beego :8086، Kratos :8087، go-zero :8088). شغّلها كلها عبر `go run ./example`؛ ويُقدَّم API التوثيق القديم على `/apidoc/api/...` في كل منفذ، وواجهة UIC-1 على `/apidoc/ui/v1/...`، ويمكن لواجهة خارجية أو لواجهة Vue المرفقة عبر `UIPath` أن تستهلكها.

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
