// Command e2e-server is the fixture the browser E2E drives. It serves the
// real apidoc UI with two debug actions: a TCP echo one-shot and a fake
// streaming one that also carries the P3 snippets and file-field surface.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/example/debugger"
)

func main() {
	tcpURL, stopEcho := startEcho()
	defer stopEcho()

	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{
		Prefix: "/apidoc", Title: "apidoc e2e",
		Debuggers:        []apidoc.Debugger{debugger.TCP(), &fakeStream{}},
		DebugAllowUnauth: true, // fixture only
	})
	must(s.Register(apidoc.Route{
		Method: "SEND",
		URL:    tcpURL,
		Doc: apidoc.Doc{
			Protocol: "tcp", Title: "TCP Echo", Author: "e2e",
			Params:    []apidoc.Param{{Name: "payload", In: "payload", Type: "text", Required: true, Desc: "bytes to echo"}},
			Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "echoed bytes"}},
		},
	}))
	// Browser-mode HTTP action: the UI fetches its handler directly.
	must(s.Register(apidoc.Route{
		Method: "GET",
		URL:    "/e2e/http",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("http-browser-ok"))
		}),
		Doc: apidoc.Doc{
			Title: "HTTP Browser", Author: "e2e",
			Responses: []apidoc.Response{{Name: "ok", Type: "string"}},
		},
	}))
	// Regression fixture: an action with params but no documented responses.
	// The server emits "responses": null for it.
	must(s.Register(apidoc.Route{
		Method: "SEND",
		URL:    tcpURL + "?variant=no-responses",
		Doc: apidoc.Doc{
			Protocol: "tcp", Title: "No Responses", Author: "e2e",
			Params: []apidoc.Param{{Name: "payload", In: "payload", Type: "text", Desc: "regression fixture"}},
		},
	}))
	must(s.Register(apidoc.Route{
		Method: "SUBSCRIBE",
		URL:    "fake://stream/demo",
		Doc: apidoc.Doc{
			Protocol: "fake", Title: "Fake Stream", Author: "e2e",
			Params:    []apidoc.Param{{Name: "payload", In: "payload", Type: "text", Desc: "message to send"}},
			Responses: []apidoc.Response{{Name: "events", Type: "stream", Desc: "welcome, sent and echo events"}},
		},
	}))
	must(s.Mount(adapter.NewNetHTTP(mux)))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()

	// One JSON line tells the driver where to point the browser.
	out, _ := json.Marshal(map[string]string{
		"url": "http://" + ln.Addr().String(), "tcp": tcpURL,
	})
	fmt.Println(string(out))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// startEcho runs a TCP echo listener on a random loopback port.
func startEcho() (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return "tcp://" + ln.Addr().String(), func() { _ = ln.Close(); wg.Wait() }
}

// fakeStream is a debug.StreamDebugger whose events are deterministic:
// welcome on open, sent+echo on each message.
type fakeStream struct{}

func (*fakeStream) Protocol() string { return "fake" }

func (*fakeStream) Spec() debug.Spec {
	return debug.Spec{
		Mode:         debug.ModeServer,
		Actions:      []string{"SUBSCRIBE", "PUBLISH"},
		Encodings:    []string{"text", "json"},
		Streaming:    true,
		ResponseView: debug.ViewTranscript,
		ConnectFields: []debug.Field{
			{Name: "topic", Label: "Topic", Type: "text", Default: "demo"},
			{Name: "cert", Label: "Client certificate", Type: "file"},
		},
		Snippets: map[string]string{
			"curl": "curl -N '{{url}}' -d '{{body}}'",
			"go":   "// {{method}} {{url}}\nclient.Send(ctx, []byte(`{{body}}`))",
		},
	}
}

func (*fakeStream) Exec(context.Context, debug.Request) (*debug.Response, error) {
	return nil, fmt.Errorf("fake: streaming protocol has no one-shot exec")
}

func (*fakeStream) Open(_ context.Context, req debug.Request) (debug.Session, error) {
	s := &fakeSession{ch: make(chan debug.Event, 16)}
	s.emit(debug.Event{Direction: "in", Name: "welcome", Data: "connected to " + req.URL})
	return s, nil
}

type fakeSession struct {
	mu     sync.Mutex
	ch     chan debug.Event
	closed bool
}

func (s *fakeSession) Events() <-chan debug.Event { return s.ch }

func (s *fakeSession) Send(_ context.Context, msg []byte, _ map[string]string) error {
	s.emit(debug.Event{Direction: "out", Name: "sent", Data: string(msg)})
	s.emit(debug.Event{Direction: "in", Name: "echo", Data: string(msg)})
	return nil
}

func (s *fakeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
	return nil
}

func (s *fakeSession) emit(ev debug.Event) {
	ev.Time = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- ev:
	default:
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
