// Package iec104 debugs IEC 60870-5-104 outstations with go-iecp5:
// general interrogation and single commands.
package iec104

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
	"github.com/riclolsen/go-iecp5/asdu"
	"github.com/riclolsen/go-iecp5/cs104"
)

// New returns the IEC 104 debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("iec104", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"INTERROGATION", "SINGLE_COMMAND"},
		Encodings:    []string{"json", "text"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "common_addr", Label: "Common address (ASDU)", Type: "number", Default: "1"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "iec104" || u.Hostname() == "" {
		return nil, errors.New("iec104: url must be iec104://host:port")
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "2404")
	}
	ca, err := opts.Uint(req.Options, "common_addr", 1, 65535)
	if err != nil {
		return nil, err
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 5*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}
	ioaStr := strings.TrimSpace(param(req, "ioa"))
	if ioaStr == "" {
		ioaStr = "0"
	}
	ioa, err := strconv.ParseUint(ioaStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("iec104: ioa: %w", err)
	}
	value, _ := strconv.ParseBool(param(req, "value"))

	h := &clientHandler{asdus: make(chan *asdu.ASDU, 64), errs: make(chan error, 1)}
	option := cs104.NewOption()
	if err := option.AddRemoteServer(addr); err != nil {
		return nil, err
	}
	option.SetAutoReconnect(false)
	client := cs104.NewClient(h, option)
	client.SetOnConnectHandler(func(c *cs104.Client) { c.SendStartDt() })
	client.SetConnectTimeoutHandler(func(*cs104.Client) {
		select {
		case h.errs <- fmt.Errorf("iec104: cannot connect to %s", addr):
		default:
		}
	})
	client.SetOnActivatedHandler(func(c *cs104.Client) {
		var err error
		switch req.Method {
		case "INTERROGATION":
			err = asdu.InterrogationCmd(c, asdu.CauseOfTransmission{Cause: asdu.Activation},
				asdu.CommonAddr(ca), asdu.QOIStation)
		case "SINGLE_COMMAND":
			err = asdu.SingleCmd(c, asdu.C_SC_NA_1, asdu.CauseOfTransmission{Cause: asdu.Activation},
				asdu.CommonAddr(ca), asdu.SingleCommandInfo{
					Ioa: asdu.InfoObjAddr(ioa), Value: value,
					Qoc: asdu.QualifierOfCommand{Qual: asdu.QOCNoAdditionalDefinition},
				})
		default:
			err = fmt.Errorf("iec104: unknown action %q", req.Method)
		}
		if err != nil {
			select {
			case h.errs <- err:
			default:
			}
		}
	})
	if err := client.Start(); err != nil {
		return nil, err
	}
	defer client.Close()

	var got []*asdu.ASDU
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	idle := time.NewTimer(time.Hour)
	defer idle.Stop()
	for {
		select {
		case a := <-h.asdus:
			got = append(got, a)
			idle.Reset(300 * time.Millisecond)
		case err := <-h.errs:
			return nil, err
		case <-idle.C:
			return result(got)
		case <-deadline.C:
			if len(got) > 0 {
				return result(got)
			}
			return nil, errors.New("iec104: no response before timeout")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func result(got []*asdu.ASDU) (*apidoc.DebugResponse, error) {
	type asduOut struct {
		Type       string `json:"type"`
		Cause      string `json:"cause"`
		CommonAddr uint16 `json:"common_addr"`
		Info       any    `json:"info,omitempty"`
	}
	out := make([]asduOut, 0, len(got))
	for _, a := range got {
		item := asduOut{
			Type: a.Type.String(), Cause: a.Coa.String(), CommonAddr: uint16(a.CommonAddr),
		}
		switch a.Type {
		case asdu.M_ME_NA_1:
			vals := a.GetMeasuredValueNormal()
			info := make([]map[string]any, len(vals))
			for i, v := range vals {
				info[i] = map[string]any{
					"ioa": uint(v.Ioa), "value": float64(v.Value) / 32768, "quality": v.Qds.String(),
				}
			}
			item.Info = info
		case asdu.M_ME_NC_1:
			vals := a.GetMeasuredValueFloat()
			info := make([]map[string]any, len(vals))
			for i, v := range vals {
				info[i] = map[string]any{"ioa": uint(v.Ioa), "value": v.Value, "quality": v.Qds.String()}
			}
			item.Info = info
		case asdu.C_SC_NA_1:
			cmd := a.GetSingleCmd()
			item.Info = map[string]any{"ioa": uint(cmd.Ioa), "value": cmd.Value, "qoc": cmd.Qoc.String()}
		}
		out = append(out, item)
	}
	body, _ := json.Marshal(map[string]any{"asdus": out})
	return &apidoc.DebugResponse{Status: "OK", Body: body, Encoding: "json"}, nil
}

func param(req apidoc.DebugRequest, name string) string {
	for _, p := range req.Params {
		if p.Name == name && p.Value != "" {
			return p.Value
		}
	}
	return req.Options[name]
}

// clientHandler implements cs104.ClientHandlerInterface. General handlers are
// no-ops; every incoming ASDU is reported through ASDUHandlerAll.
type clientHandler struct {
	asdus chan *asdu.ASDU
	errs  chan error
}

func (h *clientHandler) push(a *asdu.ASDU) {
	select {
	case h.asdus <- a:
	default:
	}
}

func (h *clientHandler) InterrogationHandler(asdu.Connect, *asdu.ASDU) error        { return nil }
func (h *clientHandler) CounterInterrogationHandler(asdu.Connect, *asdu.ASDU) error { return nil }
func (h *clientHandler) ReadHandler(asdu.Connect, *asdu.ASDU) error                 { return nil }
func (h *clientHandler) TestCommandHandler(asdu.Connect, *asdu.ASDU) error          { return nil }
func (h *clientHandler) ClockSyncHandler(asdu.Connect, *asdu.ASDU) error            { return nil }
func (h *clientHandler) ResetProcessHandler(asdu.Connect, *asdu.ASDU) error         { return nil }
func (h *clientHandler) DelayAcquisitionHandler(asdu.Connect, *asdu.ASDU) error     { return nil }
func (h *clientHandler) ASDUHandler(asdu.Connect, *asdu.ASDU, *cs104.Server, int) error {
	return nil
}
func (h *clientHandler) ASDUHandlerAll(_ asdu.Connect, a *asdu.ASDU, _ *cs104.Server, _ int) error {
	h.push(a)
	return nil
}
