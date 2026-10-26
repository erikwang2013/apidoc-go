![Apidoc शुभंकर](../../svg/apidoc-pet.svg)

# apidoc-go — Go सामान्य-उद्देश्य API दस्तावेज़ प्लगइन

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## परियोजना परिचय

**वर्तमान संस्करण: v1.1.0** ([Releases](https://github.com/erikwang2013/apidoc-go/releases) देखें)

**apidoc-go** एक सामान्य-उद्देश्य Go API दस्तावेज़ प्लगइन लाइब्रेरी है: इंटरफ़ेस दस्तावेज़ों को **टाइप किए गए स्ट्रक्ट्स** के रूप में रूट पंजीकरण के साथ ही घोषित किया जाता है — दस्तावेज़ और रूट एक साथ जन्म लेते हैं। दस्तावेज़ लीगेसी `/api/*` और UIC-1 `/ui/v1/*` दोनों JSON API के माध्यम से उपलब्ध कराए जाते हैं, इसलिए कोई भी बाहरी UI इन्हें रेंडर कर सकता है और परीक्षण अनुरोध भेज सकता है — या `Config.UIPath` के माध्यम से उसी प्रोसेस में पैकेज्ड Vue UI परोसा जा सकता है। इसमें पासवर्ड प्रमाणीकरण, बहु-ऐप / बहु-संस्करण प्रबंधन, Mock डेटा, JSON / TypeScript निर्यात, कमेंट और नौ-फ़ॉर्मेट प्रोटोकॉल-परिभाषा पार्सिंग, रिस्पॉन्स कैशिंग और स्वचालित पैरामीटर पूर्ति अंतर्निहित हैं; सर्वर-साइड प्रोटोकॉल एक्ज़ीक्यूटर build tag से माँग पर कंपाइल होते हैं और HTTP, RPC, socket, messaging, IoT, industrial तथा media प्रोटोकॉल परिवारों को कवर करते हैं।
एक बार जोड़ें, सभी फ्रेमवर्क पर काम करे — मौजूदा प्रोजेक्ट में किसी बदलाव की आवश्यकता नहीं।

## परियोजना की विशेषताएँ

| # | विशेषता | विवरण |
|---|------|------|
| 1 | दस्तावेज़ स्वतः जनरेट | टाइप किया हुआ Doc घोषणा, रूट पंजीकरण के साथ; एक स्थान पर घोषणा, दस्तावेज़ और रूट एक साथ |
| 2 | दस्तावेज़ API | लीगेसी `/api/*` (menus / protocols / detail / export) और UIC-1 `/ui/v1/*` (docs, debug, shares, generators, export), बाहरी UI या बंडल किए गए Vue UI के लिए |
| 3 | Mock डेटा | फ़ील्ड-स्तरीय Mock उदाहरण; इंटरफ़ेस समेकन में तेज़ी |
| 4 | बहु-ऐप / बहु-संस्करण | App / Version ट्री-आधारित प्रबंधन; एक प्लगइन पूरे प्रोजेक्ट के दस्तावेज़ों को कवर करता है |
| 5 | पासवर्ड प्रमाणीकरण | ग्लोबल पासवर्ड + ऐप-स्तरीय पासवर्ड; HMAC Token · निरंतर-समय तुलना |
| 6 | Markdown दस्तावेज़ | goldmark सुरक्षित रेंडरिंग; मूल HTML स्वतः हटा दिया जाता है |
| 7 | बहु-फ्रेमवर्क अनुकूलन | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero; एक बार जोड़ें, सभी फ्रेमवर्क पर काम करे |
| 8 | JSON / TypeScript निर्यात | इंटरफ़ेस प्रकार एक क्लिक में निर्यात; फ्रंट-बैक समेकन अधिक सुगम |
| 9 | सुरक्षा संरक्षण | पासवर्ड प्रमाणीकरण · CORS श्वेतसूची प्रतिबंध · XSS / पथ ट्रैवर्सल से सुरक्षा · सर्वर-साइड डिबग के लिए SSRF गार्ड और session/byte सीमाएँ |
| 10 | एनोटेशन और परिभाषा पार्सिंग | go/ast `@apidoc` / `@protocol` / `@definition` पार्स करता है; नौ अंतर्निहित परिभाषा फ़ॉर्मेट: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC और XML-RPC |
| 11 | HTTP कैशिंग | संस्करण-आधारित रिस्पॉन्स कैश + ETag/304, डॉक एंडपॉइंट मिलीसेकंड में उत्तर देते हैं |
| 12 | स्वचालित पैरामीटर पूर्ति | reflect हैंडलर सिग्नेचर से रिक्वेस्ट पैरामीटर का अनुमान लगाता है |
| 13 | किसी भी प्रोटोकॉल के एनोटेशन | `@protocol` प्रोटोकॉल चिह्नित करता है; detail protocol/method/url/params/responses/mock के साथ पार्स की गई प्रोटोकॉल परिभाषाएँ भी लौटाता है |
| 14 | बंडल किया गया Vue UI (वैकल्पिक) | `Config.UIPath` को UIC-1 पैकेज्ड बिल्ड पर इंगित करें ताकि उसी प्रोसेस में Vue UI परोसा जाए; खाली मान होने पर JSON API पर वापस चला जाता है |
| 15 | सर्वर-साइड प्रोटोकॉल एक्ज़ीक्यूटर | build tag से कंपाइल होने वाले सात प्रोटोकॉल परिवार; डिफ़ॉल्ट रूप से बंद और कंपाइल न होने पर fail closed, `apidoc_debug_all` सब कुछ सक्षम करता है |

## वास्तुकला अवलोकन

![परियोजना वास्तुकला](../../svg/architecture.svg)

![परियोजना की विशेषताएँ](../../svg/features.svg)

![परियोजना जीवनचक्र](../../svg/lifecycle.svg)

## परियोजना संरचना

```
apidoc-go/
├── apidoc.go            # मुख्य प्रवेश बिंदु: New / Register / Mount / Handler
├── VERSION              # प्रोजेक्ट संस्करण (v1.1.0); release workflow इसे टैग करता है
├── doc.go               # Route / Doc / Param / Response प्रकार
├── model/               # डेटा मॉडल
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   @definition बाहरी प्रोटोकॉल परिभाषाएँ
│   ├── protocol.go      #   URL स्कीम → प्रोटोकॉल
│   └── protocols.go     #   प्रोटोकॉल रजिस्ट्री / उपनाम / डिफ़ॉल्ट पोर्ट
├── store/               # भंडारण और डी-डुप्लिकेशन
│   └── store.go         #   (app, version, method, url) डी-डुप्लिकेशन, बाद वाला पिछले को ओवरराइड करता है
├── auth/                # प्रमाणीकरण
│   └── auth.go          #   पासवर्ड सत्यापन · HMAC Token जारी करना और सत्यापन
├── server/              # HTTP API
│   ├── server.go        #   legacy /api मार्ग · auth / CORS / कैशिंग
│   ├── ui.go            #   UIPath स्थिर Vue UI (SPA fallback · पथ-सीमित)
│   └── uiapi/           #   UIC-1 /ui/v1 API
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     दस्तावेज़ पठन · परिभाषा पार्सिंग
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators · code templates · shares · cache
│       ├── share.go
│       └── debug/       #     सर्वर-साइड प्रोटोकॉल एक्ज़ीक्यूटर (प्रति परिवार build-tag)
├── adapter/             # फ्रेमवर्क अनुकूलन परत (टाइप किए गए इंटरफ़ेस)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # एनोटेशन ऑटो-पार्सर
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   नौ प्रोटोकॉल-परिभाषा पार्सर
├── export/              # निर्यात
│   └── export.go        #   TypeScript इंटरफ़ेस परिभाषाएँ
├── mock/                # Mock डेटा
│   └── mock.go          #   फ़ील्ड-स्तरीय उदाहरण जनरेशन
├── example/             # नमूना प्रोजेक्ट (8 फ्रेमवर्क :8081–:8088)
│   ├── main.go
│   └── handlers/        #   @apidoc कमेंट उदाहरण
└── docs/                # दस्तावेज़ और संपत्तियाँ
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # वास्तुकला आरेख / विशेषताएँ आरेख / जीवनचक्र आरेख
```

## उपयोग निर्देश

### स्थापना

```bash
go get github.com/erikwang2013/apidoc-go
```

### त्वरित शुरुआत (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "मेरा API दस्तावेज़"})

	// दस्तावेज़ और रूट एक साथ घोषित किए जाते हैं
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
			Title:   "अभिवादन",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "आपका नाम"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// बाहरी UI /apidoc/api/menus या UIC-1 /apidoc/ui/v1/* पढ़ता है;
	// UIPath सेट होने पर सीधे http://localhost:8080/apidoc/ खोलें
}
```

### UIC-1 बंडल किया गया Vue UI और सर्वर-साइड एक्ज़ीक्यूटर सक्षम करना

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// वैकल्पिक: apidoc-ui के npm run package:ui का आउटपुट
	// (index.html / config.js / assets/)।
	// खाली छोड़ें तो केवल JSON API परोसा जाएगा और बाहरी UI का उपयोग जारी रहेगा।
	UIPath: "/srv/myapp/dist/uic1-ui",
	// सर्वर-साइड relay डिफ़ॉल्ट रूप से बंद है; सक्षम होने पर भी यह
	// SSRF श्वेतसूची और byte/session सीमाओं के अधीन रहता है।
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

एक्ज़ीक्यूटर build समय पर प्रोटोकॉल परिवार के अनुसार चुने जाते हैं: `go build -tags apidoc_debug_all` सभी को कंपाइल करता है, या `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` और `apidoc_debug_media` से उन्हें छाँटें। कंपाइल से बाहर रहे प्रोटोकॉल के कॉल सदैव fail closed होते हैं।

### किसी भी प्रोटोकॉल के एनोटेशन

गैर-HTTP इंटरफ़ेस `@method` + `@url` का पुन: उपयोग करते हैं और `@protocol` से प्रोटोकॉल निर्दिष्ट करते हैं (URL scheme से भी अनुमान लगाया जा सकता है):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title तापमान प्रकाशित करें
// @param qos int false "QoS" in=option default=1
// @param payload json true "संदेश निकाय" in=payload
func PublishTemp() {}
```

`@protocol` पूरी प्रोटोकॉल सूची और उनके उपनाम स्वीकार करता है (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); पूर्ण key, उपनाम, scheme और डिफ़ॉल्ट पोर्ट रजिस्ट्री `GET /api/protocols` पर उपलब्ध है। बाइनरी body को `@param ... type=binary` (या `hex`/`base64`) + `in=payload` से घोषित करें।

`/api/detail` और UIC-1 `/ui/v1/docDetail` `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` और पार्स की गई `definitions` लौटाते हैं।

डिबगिंग के दो चैनल हैं: सीधे ब्राउज़र से (HTTP / WebSocket / SSE, सदैव उपलब्ध) और सर्वर-साइड relay (`Debug` सक्षम होने तथा मेल खाते build tag के साथ)। सर्वर-साइड प्रोटोकॉल परिवार हैं HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), sockets (TCP / UDP / Raw / Unix), messaging (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industrial (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) और media (RTSP / SIP / RTP / RTCP / RTMP)।
डिफ़ॉल्ट बिल्ड कोई एक्ज़ीक्यूटर कंपाइल नहीं करते; सक्षम होने पर लक्ष्यों को पहले SSRF गार्ड पास करना होता है (`AllowHosts` श्वेतसूची, private/loopback/link-local/cloud-metadata पते डिफ़ॉल्ट रूप से अस्वीकृत, DNS resolution के बाद प्रति-IP सत्यापन तथा connection pinned), और वे timeouts, प्रति-संदेश 1 MiB सीमा, रिस्पॉन्स `MaxBytes` और stream-session सीमाओं से सीमित रहते हैं।

### प्रोटोकॉल परिभाषा पार्सिंग (@definition)

बाहरी प्रोटोकॉल परिभाषाएँ शामिल करने के लिए `@definition` का उपयोग करें (फ़ॉर्मेट स्वतः पहचाना जाता है, या `format=` से सेट करें); पार्स किया गया परिणाम detail के साथ लौटाया जाता है ताकि UIC संदेश संरचनाएँ रेंडर कर सके और डिबग फ़ॉर्म बना सके:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

समर्थित फ़ॉर्मेट: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC और XML-RPC। `Config.DefinitionRoot` पठनीय फ़ाइलों को रूट निर्देशिका तक सीमित करता है (डिफ़ॉल्ट: प्रोसेस की कार्यशील निर्देशिका); पार्सिंग प्रति फ़ाइल 1 MiB और 50,000 पंक्तियों तथा 32 की नेस्टेड-रेफ़रेंस गहराई तक सीमित है; `operation=`, `message=`, `direction=`, `sample=` और `inline=` जैसे विकल्पों के लिए `parse` पैकेज की टिप्पणियाँ देखें।

### त्वरित शुरुआत (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "मेरा API दस्तावेज़"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "अभिवादन"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Echo / Chi / Fiber के लिए केवल एडाप्टर कंस्ट्रक्टर बदलें: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — बाकी कोड बिल्कुल समान रहता है। beego / Kratos / go-zero के लिए नीचे फ्रेमवर्क एडाप्टर तालिका देखें।

### कॉन्फ़िगरेशन विकल्प

| विकल्प | डिफ़ॉल्ट | विवरण |
|--------|--------|------|
| `Prefix` | `/apidoc` | दस्तावेज़ माउंट पथ |
| `Title` | `API Docs` | दस्तावेज़ शीर्षक |
| `Desc` | — | दस्तावेज़ विवरण |
| `GlobalParams` | — | वैश्विक पैरामीटर, हर Action में मर्ज किए जाते हैं |
| `Auth` | — | प्रमाणीकरण कॉन्फ़िगरेशन: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | ऐप कॉन्फ़िगरेशन: `Key` / `Title` / `Password` (ऐप-स्तरीय स्वतंत्र पासवर्ड) |
| `DebugOrigins` | — | दस्तावेज़ API को क्रॉस-ओरिजिन पढ़ने के लिए अनुमत मूल (बाहरी UI, डिफ़ॉल्ट: कोई नहीं) |
| `PublicHost` | — | बाहरी UI द्वारा डिबगिंग के लिए उपयोग किया जाने वाला निरपेक्ष आधार URL; खाली होने का अर्थ same-origin है |
| `ShareDir` | टेम्प निर्देशिका के अंतर्गत `apidoc-go-shares` | UIC-1 शेयर-रिकॉर्ड निर्देशिका |
| `DefinitionRoot` | प्रोसेस की कार्यशील निर्देशिका | `@definition` फ़ाइल रूट; रूट से बाहर के पथ और symlink अस्वीकृत |
| `Generator` / `CodeTemplates` | — | घोषणात्मक generators और code templates (रेंडरिंग side-effect free है) |
| `ShareActions` | — | UI द्वारा व्याख्यायित share-action घोषणाएँ |
| `Debug` | शून्य मान = बंद | सर्वर-साइड एक्ज़ीक्यूटर: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (कोई बंडल UI नहीं) | पैकेज्ड UIC-1 Vue बिल्ड की निर्देशिका; खाली न होने पर `{Prefix}/` के अंतर्गत उसी प्रोसेस में स्थिर UI परोसता है |

### UIC-1 API और बंडल किया गया Vue UI (वैकल्पिक)

लाइब्रेरी डिफ़ॉल्ट रूप से दो JSON दस्तावेज़ API परोसती है: लीगेसी `/api/*` और UIC-1 `/ui/v1/*`, जिन्हें बाहरी UI उपभोग करता है। `GET {Prefix}/ui/v1/capabilities` अनुबंध, बैकएंड संस्करण और सुविधा मैट्रिक्स (browser/server डिबग मोड, shares, cache, generators, export, auth) लौटाता है; UI इससे तय करता है कि कौन-से पैनल उपलब्ध हैं। `/ui/v1/config` menus के साथ generator और code-template कॉन्फ़िगरेशन लौटाता है। पैकेज्ड `apidoc-ui` बिल्ड `Config.UIPath` को सौंपने के बाद, वही प्रोसेस Vue UI को सीधे `{Prefix}/` के अंतर्गत भी परोसता है:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // scripts/package-ui.sh का आउटपुट निर्देशिका
})
```

लाइब्रेरी स्वयं फ्रंटएंड संपत्तियाँ embed नहीं करती, इसलिए build-time निर्भरता नहीं है; bundled-UI release पैकेज को डिफ़ॉल्ट रूप से `UIPath` को शिप किए गए `dist/uic1-ui/` निर्देशिका पर इंगित करना चाहिए — ऊपर दिया गया wiring अनुशंसित सेटअप है। परिनियोजन चरण:

1. UI रिपॉज़िटरी में बिल्ड और पैकेज करें: `npm run package:ui` (आउटपुट `dist/uic1-ui/`)।
2. लक्ष्य निर्देशिका में परिनियोजित करें (उसे मौजूद और खाली होना चाहिए, या पिछला uic1-ui पैकेज रखना चाहिए): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`।
3. स्टार्टअप पर `UIPath` सेट करें; केवल-दस्तावेज़-API मोड पर वापस जाने के लिए इसे खाली छोड़कर पुनः आरंभ करें।

व्यवहार:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` और `{Prefix}/assets/*` स्थिर फ़ाइलों के रूप में परोसी जाती हैं, MIME प्रकार एक्सटेंशन से निर्धारित होता है;
- अज्ञात बिना-एक्सटेंशन पथ `index.html` पर वापस जाते हैं (SPA fallback); अनुपस्थित `.js` / `.css` और समान संपत्तियाँ 404 लौटाती हैं;
- `{Prefix}` (बिना ट्रेलिंग स्लैश) 307 के साथ `{Prefix}/` पर रीडायरेक्ट करता है ताकि सापेक्ष संपत्ति पथ सही रहें;
- `{Prefix}/api/*` और `{Prefix}/ui/v1/*` को प्राथमिकता मिलती है; अज्ञात API पथ 404 लौटाते हैं और कभी UI पर वापस नहीं जाते;
- फ़ाइल पहुँच `UIPath` तक सीमित है (`..` और रूट से बाहर के symlink अस्वीकृत), कोई निर्देशिका सूचीकरण नहीं; यदि निर्देशिका या `index.html` अनुपस्थित हो तो UI मार्ग 500 लौटाता है और दस्तावेज़ API अप्रभावित रहता है;
- go-zero एडाप्टर केवल निश्चित API मार्ग पंजीकृत करता है और prefix के अंतर्गत मनमाने पथ अग्रेषित नहीं करता, इसलिए उस एडाप्टर के साथ बंडल UI उपलब्ध नहीं है (अन्य सभी एडाप्टर इसे समर्थन देते हैं);
- जो परिनियोजन अभी भी बाहरी UI का उपयोग करते हैं उन्हें कोई बदलाव नहीं चाहिए; बस `config.js` में `HTTP.API_PREFIX` को सर्वर `Prefix` के बराबर रखें।

### सर्वर-साइड प्रोटोकॉल एक्ज़ीक्यूटर (बिल्ड टैग)

सर्वर-साइड relay डिफ़ॉल्ट रूप से कोई एक्ज़ीक्यूटर कंपाइल नहीं करता, और कंपाइल न किए गए प्रोटोकॉल को कॉल करने पर fail-closed त्रुटि लौटती है। प्रोटोकॉल परिवार के अनुसार build tag चुनें; `apidoc_debug_all` उन सभी को एकत्र करता है:

| बिल्ड टैग | प्रोटोकॉल परिवार | प्रोटोकॉल |
|-----------|-----------------|-----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, केवल https) |
| `apidoc_debug_sockets` | सॉकेट | tcp, udp, raw, unix (unary + streaming) |
| `apidoc_debug_rpc` | RPC / वित्त | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unary; descriptor या server reflection) |
| `apidoc_debug_messaging` | मैसेजिंग | mqtt, stomp, amqp, kafka, xmpp (unary + streaming; `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp` से अलग-अलग भी चुनने योग्य) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | औद्योगिक | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | मीडिया | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | सभी | ऊपर का प्रत्येक परिवार |

सुरक्षा सीमा: `Debug.EnableServerSide` डिफ़ॉल्ट रूप से बंद है; सक्षम होने पर भी लक्ष्यों को `AllowHosts` श्वेतसूची पास करनी होती है (suffix मिलान जैसे `*.example.com`), private/loopback/link-local/CGNAT और cloud-metadata पते डिफ़ॉल्ट रूप से अस्वीकृत होते हैं (केवल `AllowPrivateNetworks` उन्हें अनुमति देता है), DNS resolution के बाद प्रति-IP सत्यापन होता है और connection pinned रहता है, तथा `CONNECT`, redirects और proxies वर्जित हैं; अनुरोध और प्रतिक्रियाएँ `TimeoutMS`, `MaxBytes` (डिफ़ॉल्ट 1 MiB), stream sessions (प्रति scope 4), frame counts और idle timeouts से सीमित हैं। परिभाषा पार्सिंग केवल `DefinitionRoot` के भीतर की नियमित फ़ाइलें पढ़ती है, जो फ़ाइल आकार, पंक्ति संख्या और reference गहराई से सीमित है।

### फ्रेमवर्क एडाप्टर

| फ्रेमवर्क | कंस्ट्रक्टर |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### एनोटेशन ऑटो-पार्सिंग (go/ast)
हैंडलर के ऊपर `@apidoc` कमेंट लिखें, फिर पार्स परिणाम रजिस्टर करें:

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

### स्वचालित पैरामीटर पूर्ति (reflect)
जब `Doc.Params` खाली होता है, Register हैंडलर सिग्नेचर से reflect द्वारा पैरामीटर का अनुमान लगाता है: स्ट्रक्ट आर्ग्युमेंट body फ़ील्ड में विस्तारित होते हैं (json टैग के अनुसार), और फ्रेमवर्क कॉन्टेक्स्ट (gin.Context / echo.Context / fiber.Ctx) स्वतः छोड़ दिए जाते हैं।

### HTTP कैशिंग (रिस्पॉन्स कैश + ETag)
`/api/menus`, `/api/protocols` और `/api/export` के रिस्पॉन्स हर डॉक संस्करण के लिए एक बार सीरियलाइज़ करके प्रोसेस में कैश किए जाते हैं। `Handler` बनाते समय कैश प्रीहीट होता है और `Register` / `SetAppMeta` के बाद स्वतः अमान्य हो जाता है। रिस्पॉन्स में `ETag` + `Cache-Control: private, max-age=300` बना रहता है, इसलिए बार-बार विज़िट पर 304 मिलता है। किसी कॉन्फ़िगरेशन की आवश्यकता नहीं।

### निर्यात
| फ़ॉर्मेट | एंडपॉइंट | विवरण |
|--------|----------|------|
| JSON | `GET /apidoc/api/export` | संपूर्ण प्रोजेक्ट ट्री |
| TypeScript | `GET /apidoc/api/export?format=typescript` | इंटरफ़ेस प्रकार परिभाषाएँ |

### Mock डेटा
विवरण पृष्ठ स्वतः एक Mock उदाहरण दिखाता है: `Doc.Params[].Mock` से इसे अनुकूलित करें, या फ़ील्ड प्रकार से जनरेट होने दें (string→"sample", int→0, bool→true, ...)।

### नमूना प्रोजेक्ट
`example/` में 8 फ्रेमवर्क सर्वर शामिल हैं (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088)। `go run ./example` से सभी चालू करें; लीगेसी दस्तावेज़ API प्रत्येक पोर्ट के `/apidoc/api/...` पर है, UIC-1 API `/apidoc/ui/v1/...` पर, और बाहरी UI या `UIPath` के माध्यम से बंडल किया गया Vue UI इन्हें उपभोग कर सकता है।

## बहु-भाषा दस्तावेज़

| भाषा | लिंक |
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

## हमें समर्थन दें

यदि यह प्रोजेक्ट आपके लिए उपयोगी है, तो कृपया स्कैन करके दान करें — इससे हमें इसे बनाए रखने और निरंतर अपडेट करते रहने की प्रेरणा मिलेगी!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat Pay दान" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay दान" />
  <p>WeChat Pay (बाएँ) · Alipay (दाएँ)</p>
</div>

### वैश्विक स्थानांतरण दान

**【प्राप्तकर्ता की जानकारी】**
प्राप्तकर्ता का नाम: WANG KEXUN
प्राप्तकर्ता का खाता संख्या: 881015918251

**【प्राप्तकर्ता बैंक】**
ZA Bank
SWIFT कोड: AABLHKHHXXX
बैंक का नाम: ZA Bank Limited
बैंक कोड: 387
बैंक का पता: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【क्रॉस-बॉर्डर रेमिटेंस संवाददाता बैंक (यदि आवश्यक हो)】**

कृपया ध्यान दें: यह क्रॉस-बॉर्डर रेमिटेंस संवाददाता (मध्यस्थ) बैंक की जानकारी है, न कि प्राप्तकर्ता बैंक की जानकारी। कृपया अपने रेमिटिंग बैंक से पूछें कि क्या यह जानकारी प्रदान करना आवश्यक है।

हांगकांग डॉलर, रॅन्मिन्बी और अमेरिकी डॉलर जमा करने के लिए संवाददाता बैंक Citibank है ——

बैंक का नाम: Citibank N.A. Hong Kong
SWIFT कोड: CITIHKHXXXX
बैंक कोड: 006
शाखा का नाम: Hong Kong Branch
शाखा कोड: 391
बैंक का पता: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

अन्य मुद्राओं के लिए संवाददाता बैंक BNY Mellon है ——

बैंक का नाम: THE BANK OF NEW YORK MELLON
SWIFT कोड: IRVTUS3NXXX
बैंक का पता: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
