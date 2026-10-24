// Package coapx is the shared CoAP request/response path used by the CoAP
// and LwM2M adapters.
package coapx

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/pool"
	"github.com/plgd-dev/go-coap/v3/udp"
)

// Options tune one CoAP exchange.
type Options struct {
	Scheme        string
	ContentFormat uint16 // used by POST/PUT
	Accept        uint16
	SetAccept     bool
	Timeout       time.Duration
}

// Do sends req and maps the CoAP response onto the debug response.
func Do(ctx context.Context, req apidoc.DebugRequest, o Options) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != o.Scheme || u.Host == "" {
		return nil, fmt.Errorf("%s: url must be %s://host:port/path", o.Scheme, o.Scheme)
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	conn, err := udp.Dial(u.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	cctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	var opts []message.Option
	if o.SetAccept {
		opts = append(opts, uintOption(message.Accept, o.Accept))
	}
	var resp *pool.Message
	switch req.Method {
	case "GET":
		resp, err = conn.Get(cctx, path, opts...)
	case "DELETE":
		resp, err = conn.Delete(cctx, path, opts...)
	case "POST":
		resp, err = conn.Post(cctx, path, message.MediaType(o.ContentFormat), bytes.NewReader(req.Body), opts...)
	case "PUT":
		resp, err = conn.Put(cctx, path, message.MediaType(o.ContentFormat), bytes.NewReader(req.Body), opts...)
	default:
		return nil, fmt.Errorf("%s: unknown action %q", o.Scheme, req.Method)
	}
	if err != nil {
		return nil, err
	}
	defer conn.ReleaseMessage(resp)

	var body []byte
	if b := resp.Body(); b != nil { // 2.04 Changed may arrive without a payload
		body, err = io.ReadAll(b)
		if err != nil {
			return nil, err
		}
	}
	headers := map[string]string{"remote": u.Host}
	enc := "hex"
	format, ferr := resp.Options().ContentFormat()
	if ferr == nil {
		headers["content_format"] = fmt.Sprint(uint16(format))
		enc = encodingOf(format)
	}
	if etag, err := resp.Options().GetBytes(message.ETag); err == nil {
		headers["etag"] = hex.EncodeToString(etag)
	}
	return &apidoc.DebugResponse{
		Status:   resp.Code().String(),
		Body:     body,
		Encoding: enc,
		Headers:  headers,
	}, nil
}

func encodingOf(mt message.MediaType) string {
	switch mt {
	case message.TextPlain, message.AppLinkFormat:
		return "text"
	case message.AppJSON, message.MediaType(11543): // 11543 = OMA LwM2M JSON
		return "json"
	case message.AppXML:
		return "xml"
	default:
		return "hex"
	}
}

func uintOption(id message.OptionID, v uint16) message.Option {
	b := make([]byte, 4)
	n, _ := message.EncodeUint32(b, uint32(v))
	return message.Option{ID: id, Value: b[:n]}
}
