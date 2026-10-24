package msg

import (
	"encoding/base64"
	"testing"
)

func TestPayload(t *testing.T) {
	cases := []struct {
		encoding string
		body     string
		want     string
	}{
		{"text", "hello", "hello"},
		{"json", `{"a":1}`, `{"a":1}`},
		{"hex", "6869", "hi"},
		{"HEX", "6869", "hi"},
		{"base64", base64.StdEncoding.EncodeToString([]byte("hi")), "hi"},
	}
	for _, tc := range cases {
		got, err := Payload([]byte(tc.body), tc.encoding)
		if err != nil {
			t.Fatalf("%s: %v", tc.encoding, err)
		}
		if string(got) != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.encoding, got, tc.want)
		}
	}
	if _, err := Payload([]byte("zz"), "hex"); err == nil {
		t.Fatal("want hex decode error")
	}
	if _, err := Payload([]byte("!"), "base64"); err == nil {
		t.Fatal("want base64 decode error")
	}
}

func TestEventEncoding(t *testing.T) {
	text := In("topic", []byte("hello"))
	if text.Direction != "in" || text.Data != "hello" || text.Encoding != "text" || text.Name != "topic" {
		t.Fatalf("text event = %+v", text)
	}
	binary := Out("topic", []byte{0xff, 0x00})
	if binary.Encoding != "base64" || binary.Data != base64.StdEncoding.EncodeToString([]byte{0xff, 0x00}) {
		t.Fatalf("binary event = %+v", binary)
	}
}

func TestOptions(t *testing.T) {
	opts := map[string]string{"a": "x", "empty": ""}
	if got := Option(opts, "a", "def"); got != "x" {
		t.Fatalf("Option = %q", got)
	}
	if got := Option(opts, "empty", "def"); got != "def" {
		t.Fatalf("empty Option = %q", got)
	}
	if !BoolOption(map[string]string{"b": "true"}, "b", false) {
		t.Fatal("BoolOption true")
	}
	if !BoolOption(nil, "missing", true) {
		t.Fatal("BoolOption default")
	}
	if !BoolOption(map[string]string{"b": "nonsense"}, "b", true) {
		t.Fatal("invalid BoolOption must fall back to the default")
	}
}

func TestURLAndAddr(t *testing.T) {
	u, err := URL("mqtt://host/topic", "mqtt", "mqtt", "mqtts")
	if err != nil {
		t.Fatal(err)
	}
	if got := Addr(u, "1883"); got != "host:1883" {
		t.Fatalf("Addr = %q", got)
	}
	u, err = URL("mqtt://host:1884/topic", "mqtt", "mqtt", "mqtts")
	if err != nil {
		t.Fatal(err)
	}
	if got := Addr(u, "1883"); got != "host:1884" {
		t.Fatalf("Addr with port = %q", got)
	}
	if _, err := URL("http://host/topic", "mqtt", "mqtt", "mqtts"); err == nil {
		t.Fatal("want scheme error")
	}
	if _, err := URL("mqtt:///topic", "mqtt", "mqtt", "mqtts"); err == nil {
		t.Fatal("want host error")
	}
}

func TestEventQueueCloseTwice(t *testing.T) {
	q := NewEventQueue()
	q.Push(In("a", []byte("x")))
	select {
	case ev := <-q.Ch():
		if ev.Data != "x" {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("event not queued")
	}
	q.Close()
	q.Close()
	if _, ok := <-q.Ch(); ok {
		t.Fatal("queue not closed")
	}
}
