// Package sse parses text/event-stream bodies (the wire format shared by
// SSE, MCP streamable HTTP and A2A streaming responses).
package sse

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// maxEventBytes bounds one event's data so a hostile stream cannot grow the
// debug transcript without limit.
const maxEventBytes = 4 << 20

// Event is one dispatched server-sent event.
type Event struct {
	Name string
	Data string
	ID   string
}

// Read calls fn for every event read from r until the stream ends or fn
// returns an error. Comment lines and retry fields are ignored, and an
// incomplete trailing event is discarded as the SSE spec requires.
func Read(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var ev Event
	var data []string
	size := 0
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if len(data) > 0 {
				ev.Data = strings.Join(data, "\n")
				if err := fn(ev); err != nil {
					return err
				}
			}
			ev = Event{}
			data = nil
			size = 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			field, value = line, ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			ev.Name = value
		case "data":
			if size+len(value)+1 > maxEventBytes {
				return fmt.Errorf("sse: event exceeds %d bytes", maxEventBytes)
			}
			size += len(value) + 1
			data = append(data, value)
		case "id":
			ev.ID = value
		}
	}
	return sc.Err()
}
