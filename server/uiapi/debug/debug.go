// Package debug implements the server-side protocol executors for the UIC-1
// debug relay. Protocol families live in build-tag gated files; the default
// build compiles only this dispatcher and the SSRF guard, so go build ./...
// keeps its dependency graph unchanged.
package debug

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultTimeout is the fallback for DebugConfig.TimeoutMS.
const DefaultTimeout = 10 * time.Second

// MaxMessageBytes is the single client message cap from the contract (1 MiB).
const MaxMessageBytes = 1 << 20

// Config is the security-relevant subset of the library DebugConfig.
type Config struct {
	EnableServerSide     bool
	AllowPrivateNetworks bool
	AllowHosts           []string
	AllowUnixSockets     bool
	TimeoutMS            int
	MaxBytes             int
}

func (c Config) Timeout() time.Duration {
	if c.TimeoutMS <= 0 {
		return DefaultTimeout
	}
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

// Limit is the request/response byte cap.
func (c Config) Limit() int {
	if c.MaxBytes <= 0 {
		return MaxMessageBytes
	}
	return c.MaxBytes
}

// Payload is a binary-safe body: encoding is utf8/text, hex or base64.
type Payload struct {
	Encoding string
	Data     string
}

func (p *Payload) Decode() ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	switch p.Encoding {
	case "", "utf8", "text":
		return []byte(p.Data), nil
	case "hex":
		b, err := hex.DecodeString(strings.TrimSpace(p.Data))
		if err != nil {
			return nil, fmt.Errorf("invalid hex body: %v", err)
		}
		return b, nil
	case "base64":
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(p.Data))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 body: %v", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported body encoding `%s`; use utf8, hex or base64", p.Encoding)
	}
}

// FromBytes returns a utf8 payload when the bytes are valid UTF-8, else base64.
func FromBytes(b []byte) Payload {
	if utf8.Valid(b) {
		return Payload{Encoding: "utf8", Data: string(b)}
	}
	return Payload{Encoding: "base64", Data: base64.StdEncoding.EncodeToString(b)}
}

// Param is the server-side wire schema of one endpoint parameter. Field is the
// thrift field id / ISO 8583 field number; it never comes from the client.
type Param struct {
	Name     string
	Type     string
	Field    string
	Required bool
	Children []Param
}

// Endpoint is the server-collected schema used by binary protocols.
type Endpoint struct {
	URL      string
	Method   string
	Protocol string
	Params   []Param
	Returned []Param
}

// Request is one executor request. Target and Opts are server-derived; the
// handler strips client-supplied reserved keys before building it.
type Request struct {
	Kind      string
	Target    string
	Method    string
	Headers   map[string]string
	Body      *Payload
	Opts      map[string]string
	Endpoint  *Endpoint
	TimeoutMS int
}

// Timeout clamps the request timeout to the configured maximum.
func (r *Request) Timeout(cfg Config) time.Duration {
	if r.TimeoutMS > 0 {
		d := time.Duration(r.TimeoutMS) * time.Millisecond
		if d < cfg.Timeout() {
			return d
		}
	}
	return cfg.Timeout()
}

// Response is a unary executor result.
type Response struct {
	Status  int
	MS      int64
	Headers map[string]string
	Body    Payload
}

// StreamSession is an open upstream stream. ReadFrame returns the next chunk;
// it must return an error once the stream closes or fails.
type StreamSession interface {
	ReadFrame() ([]byte, error)
	Close() error
}

// Executor runs one unary request.
type Executor func(Config, *Request) (*Response, error)

// StreamOpener opens one bidirectional stream.
type StreamOpener func(Config, *Request) (StreamSession, error)

var (
	executors = map[string]Executor{}
	streams   = map[string]StreamOpener{}
)

// relayOrder keeps capability output deterministic.
var relayOrder = []string{
	"http", "http2", "http3", "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook",
	"tcp", "udp", "raw", "unix", "mqtt", "stomp", "amqp", "kafka", "xmpp",
	"coap", "lwm2m", "modbus", "dnp3", "iec104", "bacnet", "opcua",
	"rtsp", "sip", "rtp", "rtcp", "rtmp",
	"grpc", "thrift", "dubbo", "hessian", "iso8583",
}

// RegisterExecutor is called from protocol-family init functions.
func RegisterExecutor(kind string, fn Executor) {
	executors[strings.ToLower(kind)] = fn
}

// RegisterStream is called from protocol-family init functions.
func RegisterStream(kind string, fn StreamOpener) {
	streams[strings.ToLower(kind)] = fn
}

// HasExecutor reports whether kind has a compiled unary executor.
func HasExecutor(kind string) bool {
	_, ok := executors[strings.ToLower(kind)]
	return ok
}

// HasStream reports whether kind has a compiled stream opener.
func HasStream(kind string) bool {
	_, ok := streams[strings.ToLower(kind)]
	return ok
}

// Execute dispatches a unary request, failing closed when the family is not
// compiled into this build.
func Execute(cfg Config, req *Request) (*Response, error) {
	fn, ok := executors[strings.ToLower(req.Kind)]
	if !ok {
		return nil, fmt.Errorf("server executor for `%s` is not compiled in this build", req.Kind)
	}
	return fn(cfg, req)
}

// OpenStream dispatches a stream request, failing closed when the family is
// not compiled into this build.
func OpenStream(cfg Config, req *Request) (StreamSession, error) {
	fn, ok := streams[strings.ToLower(req.Kind)]
	if !ok {
		return nil, fmt.Errorf("server stream executor for `%s` is not compiled in this build", req.Kind)
	}
	return fn(cfg, req)
}

// RelayIDs lists the protocol ids actually wired in this build.
func RelayIDs() []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range relayOrder {
		if HasExecutor(id) || HasStream(id) {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	extra := []string{}
	for id := range executors {
		if !seen[id] {
			extra = append(extra, id)
			seen[id] = true
		}
	}
	for id := range streams {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	return append(ids, extra...)
}
