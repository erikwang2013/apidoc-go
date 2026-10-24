// Package msg holds the small helpers shared by the messaging debug
// adapters: body decoding, URL/option parsing and the transcript queue.
package msg

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/erikwang2013/apidoc-go"
)

// Encodings are the payload encodings accepted by every messaging adapter.
var Encodings = []string{"text", "json", "hex", "base64"}

// Payload turns the debug body into the bytes on the wire. The server already
// base64-decoded the JSON field, so "hex"/"base64" mean the body text itself
// is hex/base64 encoded by the UI.
func Payload(body []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "hex":
		b, err := hex.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			return nil, fmt.Errorf("hex body: %w", err)
		}
		return b, nil
	case "base64":
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			return nil, fmt.Errorf("base64 body: %w", err)
		}
		return b, nil
	default:
		return body, nil
	}
}

// Option returns opts[key], or def when the key is missing or empty.
func Option(opts map[string]string, key, def string) string {
	if v := opts[key]; v != "" {
		return v
	}
	return def
}

// BoolOption parses a bool option, falling back to def when absent or invalid.
func BoolOption(opts map[string]string, key string, def bool) bool {
	v := opts[key]
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// URL parses raw and requires one of schemes plus a host.
func URL(raw, protocol string, schemes ...string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: bad url %q: %w", protocol, raw, err)
	}
	if u.Host == "" || !slices.Contains(schemes, strings.ToLower(u.Scheme)) {
		return nil, fmt.Errorf("%s: url %q must start with %s://host/", protocol, raw, strings.Join(schemes, " or "))
	}
	return u, nil
}

// Addr returns host:port, filling in defaultPort when the URL has none.
func Addr(u *url.URL, defaultPort string) string {
	if u.Port() != "" {
		return u.Host
	}
	return net.JoinHostPort(u.Hostname(), defaultPort)
}

// WaitTimeout caps d by the context deadline so blocking client calls honor ctx.
func WaitTimeout(ctx context.Context, d time.Duration) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem < d {
			if rem <= 0 {
				return time.Millisecond
			}
			return rem
		}
	}
	return d
}

// EventQueue is a bounded transcript channel. Protocol callbacks must never
// block, so events are dropped when the consumer falls behind.
type EventQueue struct {
	ch   chan apidoc.DebugEvent
	once sync.Once
}

// NewEventQueue returns a queue buffered for short bursts.
func NewEventQueue() *EventQueue {
	return &EventQueue{ch: make(chan apidoc.DebugEvent, 64)}
}

// Ch is the channel handed to Session.Events.
func (q *EventQueue) Ch() <-chan apidoc.DebugEvent { return q.ch }

// Push queues ev without blocking.
func (q *EventQueue) Push(ev apidoc.DebugEvent) {
	select {
	case q.ch <- ev:
	default:
	}
}

// Close closes the channel; safe to call more than once.
func (q *EventQueue) Close() { q.once.Do(func() { close(q.ch) }) }

// In builds an inbound transcript event.
func In(name string, payload []byte) apidoc.DebugEvent { return event("in", name, payload) }

// Out builds an outbound transcript event.
func Out(name string, payload []byte) apidoc.DebugEvent { return event("out", name, payload) }

// Err builds a transcript event for a connection or protocol error.
func Err(err error) apidoc.DebugEvent {
	return apidoc.DebugEvent{
		Time: time.Now(), Direction: "in", Name: "error",
		Data: err.Error(), Encoding: "text",
	}
}

func event(direction, name string, payload []byte) apidoc.DebugEvent {
	ev := apidoc.DebugEvent{Time: time.Now(), Direction: direction, Name: name}
	if utf8.Valid(payload) {
		ev.Data, ev.Encoding = string(payload), "text"
	} else {
		ev.Data, ev.Encoding = base64.StdEncoding.EncodeToString(payload), "base64"
	}
	return ev
}
