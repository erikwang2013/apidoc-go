package sse

import (
	"errors"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	input := ": comment\r\nevent: greeting\r\nid: 9\r\ndata: one\r\ndata: two\r\n\r\ndata: incomplete\r\n"
	var got []Event
	if err := Read(strings.NewReader(input), func(ev Event) error {
		got = append(got, ev)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("events = %+v", got)
	}
	if got[0].Name != "greeting" || got[0].ID != "9" || got[0].Data != "one\ntwo" {
		t.Fatalf("event = %+v", got[0])
	}
}

func TestReadStopsOnError(t *testing.T) {
	boom := errors.New("boom")
	err := Read(strings.NewReader("data: x\n\n"), func(Event) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
