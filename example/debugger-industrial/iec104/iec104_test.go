package iec104

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/riclolsen/go-iecp5/asdu"
	"github.com/riclolsen/go-iecp5/cs104"
)

// outstation answers interrogation with one measured value and confirms
// single commands.
type outstation struct{}

func (outstation) InterrogationHandler(c asdu.Connect, a *asdu.ASDU, _ asdu.QualifierOfInterrogation) error {
	return asdu.MeasuredValueNormal(c, false,
		asdu.CauseOfTransmission{Cause: asdu.InterrogatedByStation}, a.CommonAddr,
		asdu.MeasuredValueNormalInfo{Ioa: 100, Value: 16384, Qds: asdu.QDSGood})
}
func (outstation) CounterInterrogationHandler(asdu.Connect, *asdu.ASDU, asdu.QualifierCountCall) error {
	return nil
}
func (outstation) ReadHandler(asdu.Connect, *asdu.ASDU, asdu.InfoObjAddr) error { return nil }
func (outstation) ClockSyncHandler(asdu.Connect, *asdu.ASDU, time.Time) error   { return nil }
func (outstation) ResetProcessHandler(asdu.Connect, *asdu.ASDU, asdu.QualifierOfResetProcessCmd) error {
	return nil
}
func (outstation) DelayAcquisitionHandler(asdu.Connect, *asdu.ASDU, uint16) error { return nil }
func (outstation) ASDUHandlerAll(asdu.Connect, *asdu.ASDU, int) error             { return nil }

func (outstation) ASDUHandler(c asdu.Connect, a *asdu.ASDU) error {
	if a.Type != asdu.C_SC_NA_1 {
		return nil
	}
	cmd := a.GetSingleCmd()
	u := asdu.NewASDU(c.Params(), asdu.Identifier{
		Type:       asdu.C_SC_NA_1,
		Variable:   asdu.VariableStruct{IsSequence: false, Number: 1},
		Coa:        asdu.CauseOfTransmission{Cause: asdu.ActivationCon},
		CommonAddr: a.CommonAddr,
	})
	if err := u.AppendInfoObjAddr(cmd.Ioa); err != nil {
		return err
	}
	value := cmd.Qoc.Value()
	if cmd.Value {
		value |= 0x01
	}
	u.AppendBytes(value)
	return c.Send(u)
}

func startOutstation(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	srv := cs104.NewServer(outstation{})
	srv.LogMode(false)
	go func() { _ = srv.ListenAndServer(addr) }()
	t.Cleanup(func() { _ = srv.Close() })

	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("outstation did not start")
	return ""
}

type respJSON struct {
	ASDUs []struct {
		Type  string          `json:"type"`
		Cause string          `json:"cause"`
		Info  json.RawMessage `json:"info"`
	} `json:"asdus"`
}

func TestInterrogation(t *testing.T) {
	addr := startOutstation(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "INTERROGATION", URL: "iec104://" + addr,
		Options: map[string]string{"common_addr": "1", "timeout_ms": "5000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out respJSON
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ASDUs) == 0 || out.ASDUs[0].Type != "TID<M_ME_NA_1>" {
		t.Fatalf("asdus = %+v", out.ASDUs)
	}
	var info []struct {
		IOA   uint    `json:"ioa"`
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(out.ASDUs[0].Info, &info); err != nil {
		t.Fatal(err)
	}
	if len(info) != 1 || info[0].IOA != 100 || math.Abs(info[0].Value-0.5) > 0.001 {
		t.Fatalf("info = %+v", info)
	}
}

func TestSingleCommandConfirmation(t *testing.T) {
	addr := startOutstation(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SINGLE_COMMAND", URL: "iec104://" + addr,
		Params:  []apidoc.DebugValue{{Name: "ioa", Value: "100"}, {Name: "value", Value: "true"}},
		Options: map[string]string{"common_addr": "1", "timeout_ms": "5000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out respJSON
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ASDUs) == 0 || out.ASDUs[0].Type != "TID<C_SC_NA_1>" || out.ASDUs[0].Cause != "COT<ActivationCon>" {
		t.Fatalf("asdus = %+v", out.ASDUs)
	}
}

func TestUnknownAction(t *testing.T) {
	addr := startOutstation(t)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "EXPLODE", URL: "iec104://" + addr,
	})
	if err == nil {
		t.Fatal("want error for unknown action")
	}
}
