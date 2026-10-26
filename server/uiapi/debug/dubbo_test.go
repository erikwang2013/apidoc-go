//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"reflect"
	"strings"
	"testing"
)

func TestDubboRequestEncoding(t *testing.T) {
	body, err := encodeDubboRequest("com.example.DemoService", "1.0.0", "sayHello", "Ljava/lang/String;", []any{"world"})
	if err != nil {
		t.Fatal(err)
	}
	input := body
	want := []any{
		"2.0.2",
		"com.example.DemoService",
		"1.0.0",
		"sayHello",
		"Ljava/lang/String;",
		"world",
		map[string]any{},
	}
	for _, expected := range want {
		got, err := hessianDecode(&input)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("field = %#v, want %#v", got, expected)
		}
	}
	if len(input) != 0 {
		t.Fatalf("%d trailing request bytes", len(input))
	}
}

func TestDubboDescriptorCounting(t *testing.T) {
	count, err := countJavaDescriptors("Ljava/lang/String;I[B")
	if err != nil || count != 3 {
		t.Fatalf("count = %d err=%v", count, err)
	}
	if _, err := countJavaDescriptors("Lbroken"); err == nil {
		t.Fatal("invalid descriptor must be rejected")
	}
}

func TestDubboResponseFlags(t *testing.T) {
	value, err := parseDubboResponse([]byte{0x01, 'S', 0, 2, 'o', 'k'})
	if err != nil || value != "ok" {
		t.Fatalf("value response = %#v err=%v", value, err)
	}
	if _, err := parseDubboResponse([]byte{0x02}); err != nil {
		t.Fatalf("null response = %v", err)
	}
	exception := []byte{0x00, 'H', 'S', 0, 13, 'd', 'e', 't', 'a', 'i', 'l', 'M', 'e', 's', 's', 'a', 'g', 'e', 'S', 0, 4, 'b', 'o', 'o', 'm', 'Z'}
	_, err = parseDubboResponse(exception)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("exception = %v", err)
	}
}
