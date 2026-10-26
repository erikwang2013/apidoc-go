//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

const isoTestFields = `[
    {"field":"2","type":"llvar","len":19,"kind":"n"},
    {"field":"3","type":"fixed","len":6,"kind":"n"},
    {"field":"4","type":"fixed","len":12,"kind":"n"},
    {"field":"39","type":"fixed","len":2,"kind":"n"}
]`

func TestISO8583GoldenVectors(t *testing.T) {
	specs, err := parseISOFields(isoTestFields)
	if err != nil {
		t.Fatal(err)
	}
	request, err := encodeISOMessage("0200", specs, map[string]any{
		"2": "4111111111111111",
		"3": "000000",
		"4": "000000000100",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte("0200"), 0x70, 0, 0, 0, 0, 0, 0, 0)
	want = append(want, "164111111111111111000000000000000100"...)
	if !bytes.Equal(request, want) {
		t.Fatalf("request = %x, want %x", request, want)
	}

	response := append([]byte("0210"), 0x40, 0, 0, 0, 0x02, 0, 0, 0)
	response = append(response, "164111111111111111"...)
	response = append(response, "00"...)
	value, err := decodeISOMessage(response, specs)
	if err != nil {
		t.Fatal(err)
	}
	wantValue := map[string]any{
		"mti":    "0210",
		"fields": map[string]any{"2": "4111111111111111", "39": "00"},
	}
	if !reflect.DeepEqual(value, wantValue) {
		t.Fatalf("response = %#v, want %#v", value, wantValue)
	}
}

func TestISO8583SecondaryBitmapAndBinary(t *testing.T) {
	fields, err := parseISOFields(`[{"field":"70","type":"fixed","len":3,"kind":"n"}]`)
	if err != nil {
		t.Fatal(err)
	}
	message, err := encodeISOMessage("0800", fields, map[string]any{"70": "301"})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte("0800"), 0x80, 0, 0, 0, 0, 0, 0, 0)
	want = append(want, 0x04, 0, 0, 0, 0, 0, 0, 0)
	want = append(want, "301"...)
	if !bytes.Equal(message, want) {
		t.Fatalf("secondary = %x, want %x", message, want)
	}
	if value, err := decodeISOMessage(message, fields); err != nil ||
		value["fields"].(map[string]any)["70"] != "301" {
		t.Fatalf("secondary decode = %#v err=%v", value, err)
	}

	binary, err := parseISOFields(`[{"field":"55","type":"lllvar","len":999,"kind":"b"}]`)
	if err != nil {
		t.Fatal(err)
	}
	message, err = encodeISOMessage("0200", binary, map[string]any{"55": "9f2608aabbccddeeff"})
	if err != nil {
		t.Fatal(err)
	}
	want = append([]byte("0200"), 0, 0, 0, 0, 0, 0, 0x02, 0)
	want = append(want, "018"...)
	want = append(want, 0x9f, 0x26, 0x08, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff)
	if !bytes.Equal(message, want) {
		t.Fatalf("binary = %x, want %x", message, want)
	}
	value, err := decodeISOMessage(message, binary)
	if err != nil {
		t.Fatal(err)
	}
	if got := value["fields"].(map[string]any)["55"]; got != "9F2608AABBCCDDEEFF" {
		t.Fatalf("binary decode = %v", got)
	}
}

func TestISO8583FieldsFromParamsAndUnknownField(t *testing.T) {
	specs, err := isoFieldsFromParams([]Param{
		{Name: "pan", Type: "llvar:n19", Field: "2"},
		{Name: "processing_code", Type: "n6", Field: "3"},
		{Name: "amount", Type: "n12", Field: "4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := encodeISOMessage("0200", specs, map[string]any{
		"2": "4111111111111111",
		"3": "000000",
		"4": "000000000100",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(message, []byte("164111111111111111")) {
		t.Fatalf("param schema encoding = %x", message)
	}

	full, err := parseISOFields(isoTestFields)
	if err != nil {
		t.Fatal(err)
	}
	_, err = encodeISOMessage("0200", full, map[string]any{"5": "1"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unknown field = %v", err)
	}
}
