// Package iso8583 packs and sends ISO 8583 messages using moov-io/iso8583.
package iso8583

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
	iso "github.com/moov-io/iso8583"
	"github.com/moov-io/iso8583/specs"
)

// New returns the ISO 8583 debugger. The message body is JSON of the form
// {"mti":"0100","fields":{"2":"4242...","3":"000000"}}.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("iso8583", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"json"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "spec", Label: "Spec: 87ascii, 87hex or inline JSON", Type: "text", Default: "87ascii"},
			{Name: "length_prefix", Label: "Length prefix", Type: "select", Default: "2byte", Options: []string{"2byte", "4ascii", "none"}},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}, exec)
}

// FieldSet is the JSON body accepted by Exec.
type FieldSet struct {
	MTI    string            `json:"mti"`
	Fields map[string]string `json:"fields"`
}

func loadSpec(name string) (*iso.MessageSpec, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "87ascii":
		return specs.Spec87ASCII, nil
	case "87hex":
		return specs.Spec87Hex, nil
	default:
		return specs.ImportJSON([]byte(name))
	}
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "iso8583" || u.Host == "" {
		return nil, errors.New("iso8583: url must be iso8583://host:port")
	}
	spec, err := loadSpec(req.Options["spec"])
	if err != nil {
		return nil, fmt.Errorf("iso8583: spec: %w", err)
	}
	var in FieldSet
	if err := json.Unmarshal(req.Body, &in); err != nil {
		return nil, fmt.Errorf("iso8583: body: %w", err)
	}
	if in.MTI == "" {
		return nil, errors.New("iso8583: body.mti is required")
	}

	msg := iso.NewMessage(spec)
	msg.MTI(in.MTI)
	ids := make([]int, 0, len(in.Fields))
	parsed := make(map[int]string, len(in.Fields))
	for k, v := range in.Fields {
		id, err := strconv.Atoi(k)
		if err != nil || id <= 1 || id > 128 {
			return nil, fmt.Errorf("iso8583: field %q: want 2..128", k)
		}
		ids = append(ids, id)
		parsed[id] = v
	}
	sort.Ints(ids)
	for _, id := range ids {
		if err := msg.Field(id, parsed[id]); err != nil {
			return nil, fmt.Errorf("iso8583: field %d: %w", id, err)
		}
	}
	packed, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("iso8583: pack: %w", err)
	}

	timeout, err := opts.Timeout(req.Options, "timeout_ms", 5*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	prefix := req.Options["length_prefix"]
	if err := writeFrame(conn, packed, prefix); err != nil {
		return nil, err
	}
	respBytes, err := readFrame(conn, prefix)
	if err != nil {
		return nil, err
	}
	resp := iso.NewMessage(spec)
	if err := resp.Unpack(respBytes); err != nil {
		return &apidoc.DebugResponse{
			Status: "UNPARSEABLE", Body: []byte(hex.EncodeToString(respBytes)),
			Encoding: "text",
		}, nil
	}
	mti, _ := resp.GetMTI()
	fields := make(map[string]string)
	for id := range resp.GetFields() {
		if id == 0 || id == 1 {
			continue
		}
		s, err := resp.GetString(id)
		if err != nil {
			b, _ := resp.GetBytes(id)
			s = hex.EncodeToString(b)
		}
		fields[strconv.Itoa(id)] = s
	}
	out, _ := json.Marshal(map[string]any{
		"mti": mti, "fields": fields, "raw_hex": hex.EncodeToString(respBytes),
	})
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     out,
		Encoding: "json",
		Headers:  map[string]string{"remote": u.Host, "spec": spec.Name},
	}, nil
}

func writeFrame(w io.Writer, msg []byte, prefix string) error {
	switch prefix {
	case "", "2byte":
		hdr := binary.BigEndian.AppendUint16(nil, uint16(len(msg)))
		_, err := w.Write(append(hdr, msg...))
		return err
	case "4ascii":
		_, err := fmt.Fprintf(w, "%04d%s", len(msg), msg)
		return err
	case "none":
		_, err := w.Write(msg)
		return err
	default:
		return fmt.Errorf("iso8583: unknown length_prefix %q", prefix)
	}
}

func readFrame(r io.Reader, prefix string) ([]byte, error) {
	var n int
	switch prefix {
	case "", "2byte":
		var hdr [2]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		n = int(binary.BigEndian.Uint16(hdr[:]))
	case "4ascii":
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		v, err := strconv.Atoi(string(hdr[:]))
		if err != nil {
			return nil, fmt.Errorf("iso8583: length header %q: %w", hdr, err)
		}
		n = v
	case "none":
		return io.ReadAll(r)
	default:
		return nil, fmt.Errorf("iso8583: unknown length_prefix %q", prefix)
	}
	if n <= 0 || n > 65535 {
		return nil, fmt.Errorf("iso8583: invalid frame length %d", n)
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}
