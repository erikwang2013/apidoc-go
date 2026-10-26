//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func hessianDecodeBytes(t *testing.T, data []byte) any {
	t.Helper()
	input := data
	value, err := hessianDecode(&input)
	if err != nil {
		t.Fatalf("decode %x: %v", data, err)
	}
	if len(input) != 0 {
		t.Fatalf("decode %x left %d bytes", data, len(input))
	}
	return value
}

func TestHessianGoldenVectors(t *testing.T) {
	double := []byte{'D', 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}
	cases := []struct {
		value any
		want  []byte
	}{
		{nil, []byte{'N'}},
		{true, []byte{'T'}},
		{false, []byte{'F'}},
		{json.Number("1"), []byte{'I', 0, 0, 0, 1}},
		{json.Number("1099511627776"), []byte{'L', 0, 0, 1, 0, 0, 0, 0, 0}},
		{json.Number("1.5"), double},
		{"hello", []byte{'S', 0, 5, 'h', 'e', 'l', 'l', 'o'}},
		{[]any{json.Number("1"), "x"}, []byte{0x57, 'I', 0, 0, 0, 1, 'S', 0, 1, 'x', 'Z'}},
		{map[string]any{"a": json.Number("1")}, []byte{'H', 'S', 0, 1, 'a', 'I', 0, 0, 0, 1, 'Z'}},
	}
	for _, tc := range cases {
		got, err := hessianEncode(tc.value)
		if err != nil {
			t.Fatalf("encode %v: %v", tc.value, err)
		}
		if !bytes.Equal(got, tc.want) {
			t.Fatalf("encode %v = %x, want %x", tc.value, got, tc.want)
		}
	}
}

func TestHessianRoundtripAndCompactDecode(t *testing.T) {
	value := map[string]any{
		"s":    "hi",
		"i":    json.Number("42"),
		"l":    json.Number("5000000000"),
		"f":    json.Number("1.5"),
		"b":    true,
		"n":    nil,
		"list": []any{json.Number("1"), "two", false},
		"map":  map[string]any{"k": "v"},
	}
	encoded, err := hessianEncode(value)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"s":    "hi",
		"i":    int64(42),
		"l":    int64(5000000000),
		"f":    1.5,
		"b":    true,
		"n":    nil,
		"list": []any{int64(1), "two", false},
		"map":  map[string]any{"k": "v"},
	}
	if got := hessianDecodeBytes(t, encoded); !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip = %#v, want %#v", got, want)
	}

	compact := []struct {
		data []byte
		want any
	}{
		{[]byte{0x90}, int64(0)},
		{[]byte{0x80}, int64(-16)},
		{[]byte{0xbf}, int64(47)},
		{[]byte{0xc0, 0x00}, int64(-2048)},
		{[]byte{0xe0}, int64(0)},
		{[]byte{0xd8}, int64(-8)},
		{[]byte{0x05, 'h', 'e', 'l', 'l', 'o'}, "hello"},
		{[]byte{0x7a, 0x91, 0x92}, []any{int64(1), int64(2)}},
		{[]byte{0x5b}, 0.0},
		{[]byte{0x5c}, 1.0},
		{[]byte{0x5d, 0xfe}, -2.0},
		{[]byte{0x5e, 0xff, 0xfe}, -2.0},
		{[]byte{0x5f, 0x3f, 0xc0, 0, 0}, 1.5},
	}
	for _, tc := range compact {
		if got := hessianDecodeBytes(t, tc.data); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("decode %x = %#v, want %#v", tc.data, got, tc.want)
		}
	}
}

func TestHessianRejectsClassDefinitions(t *testing.T) {
	input := []byte{'C', 0x00}
	if _, err := hessianDecode(&input); err == nil {
		t.Fatal("class definition must be rejected")
	}
}

func TestHessianDepthAndLengthCaps(t *testing.T) {
	nested := bytes.Repeat([]byte{0x57}, hessianMaxDepth+2)
	if _, err := hessianDecode(&nested); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deep nesting error = %v", err)
	}
	for _, declared := range [][]byte{
		{'X', 0x7f, 0xff, 0xff, 0xff},
		{'X', 0xff, 0xff, 0xff, 0xff},
	} {
		if _, err := hessianDecode(&declared); err == nil {
			t.Fatalf("declared list length %x must be rejected", declared[1:])
		}
	}
}
