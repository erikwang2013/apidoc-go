# apidoc-go — Go सामान्य-उद्देश्य API दस्तावेज़ प्लगइन

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## परियोजना परिचय

**apidoc-go** एक सामान्य-उद्देश्य Go API दस्तावेज़ प्लगइन लाइब्रेरी है: इंटरफ़ेस दस्तावेज़ों को **टाइप किए गए स्ट्रक्ट्स** के रूप में रूट पंजीकरण के साथ ही घोषित किया जाता है — दस्तावेज़ और रूट एक साथ जन्म लेते हैं; एम्बेडेड Web UI ऑनलाइन ब्राउज़िंग और ऑनलाइन डिबगिंग प्रदान करता है, और इसमें पासवर्ड प्रमाणीकरण, बहु-ऐप / बहु-संस्करण प्रबंधन, Mock डेटा तथा JSON / TypeScript निर्यात क्षमताएँ अंतर्निहित हैं। एक बार जोड़ें, सभी फ्रेमवर्क पर काम करे — मौजूदा प्रोजेक्ट में किसी बदलाव की आवश्यकता नहीं।

## परियोजना की विशेषताएँ

| # | विशेषता | विवरण |
|---|------|------|
| 1 | दस्तावेज़ स्वतः जनरेट | टाइप किया हुआ Doc घोषणा, रूट पंजीकरण के साथ; एक स्थान पर घोषणा, दस्तावेज़ और रूट एक साथ |
| 2 | ऑनलाइन डिबगिंग | ब्राउज़र से सीधे वास्तविक इंटरफ़ेस का अनुरोध; कोई सर्वर फ़ॉरवर्डिंग नहीं, स्वाभाविक रूप से कोई SSRF नहीं |
| 3 | Mock डेटा | फ़ील्ड-स्तरीय Mock उदाहरण; इंटरफ़ेस समेकन में तेज़ी |
| 4 | बहु-ऐप / बहु-संस्करण | App / Version ट्री-आधारित प्रबंधन; एक प्लगइन पूरे प्रोजेक्ट के दस्तावेज़ों को कवर करता है |
| 5 | पासवर्ड प्रमाणीकरण | ग्लोबल पासवर्ड + ऐप-स्तरीय पासवर्ड; HMAC Token · निरंतर-समय तुलना |
| 6 | Markdown दस्तावेज़ | goldmark सुरक्षित रेंडरिंग; मूल HTML स्वतः हटा दिया जाता है |
| 7 | बहु-फ्रेमवर्क अनुकूलन | net/http · Gin · Echo · Chi · Fiber; एक बार जोड़ें, सभी फ्रेमवर्क पर काम करे |
| 8 | JSON / TypeScript निर्यात | इंटरफ़ेस प्रकार एक क्लिक में निर्यात; फ्रंट-बैक समेकन अधिक सुगम |
| 9 | सुरक्षा संरक्षण | कोई SSRF नहीं · CORS श्वेतसूची प्रतिबंध · XSS से सुरक्षा · पथ ट्रैवर्सल से सुरक्षा |

## वास्तुकला अवलोकन

![परियोजना वास्तुकला](../../docs/svg/architecture.svg)

![परियोजना की विशेषताएँ](../../docs/svg/features.svg)

![परियोजना जीवनचक्र](../../docs/svg/lifecycle.svg)

## परियोजना संरचना

```
apidoc-go/
├── apidoc.go            # मुख्य प्रवेश बिंदु: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response प्रकार
├── model/               # डेटा मॉडल
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # भंडारण और डी-डुप्लिकेशन
│   └── store.go         #   (app, version, method, url) डी-डुप्लिकेशन, बाद वाला पिछले को ओवरराइड करता है
├── auth/                # प्रमाणीकरण
│   └── auth.go          #   पासवर्ड सत्यापन · HMAC Token जारी करना और सत्यापन
├── server/              # JSON API + एम्बेडेड Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # फ्रेमवर्क अनुकूलन परत (टाइप किए गए इंटरफ़ेस)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
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
	// ब्राउज़र में http://localhost:8080/apidoc खोलें
}
```

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

Echo / Chi / Fiber के लिए केवल एडाप्टर कंस्ट्रक्टर बदलें: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — बाकी कोड बिल्कुल समान रहता है।

### कॉन्फ़िगरेशन विकल्प

| विकल्प | डिफ़ॉल्ट | विवरण |
|--------|--------|------|
| `Prefix` | `/apidoc` | दस्तावेज़ माउंट पथ |
| `Title` | `API Docs` | दस्तावेज़ शीर्षक |
| `Desc` | — | दस्तावेज़ विवरण |
| `GlobalParams` | — | वैश्विक पैरामीटर, हर Action में मर्ज किए जाते हैं |
| `Auth` | — | प्रमाणीकरण कॉन्फ़िगरेशन: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | ऐप कॉन्फ़िगरेशन: `Key` / `Title` / `Password` (ऐप-स्तरीय स्वतंत्र पासवर्ड) |
| `DebugOrigins` | — | क्रॉस-ओरिजिन डिबगिंग के लिए अनुमत मूल (डिफ़ॉल्ट: कोई नहीं) |

### फ्रेमवर्क एडाप्टर

| फ्रेमवर्क | कंस्ट्रक्टर | संस्करण |
|------|--------|------|
| net/http | `adapter.NewNetHTTP(mux)` | मानक पुस्तकालय |
| Gin | `adapter.NewGin(engine)` | v1.10.0 |
| Echo | `adapter.NewEcho(e)` | v4.12.0 |
| Chi | `adapter.NewChi(mux)` | v5.3.2 |
| Fiber | `adapter.NewFiber(app)` | v2.52.5 |

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
