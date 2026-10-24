package hls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

const master = "#EXTM3U\n" +
	"#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=640x360\n" +
	"media.m3u8\n" +
	"#EXT-X-STREAM-INF:BANDWIDTH=2560000,RESOLUTION=1280x720\n" +
	"hi/media.m3u8\n"

const media = "#EXTM3U\n" +
	"#EXT-X-VERSION:3\n" +
	"#EXT-X-TARGETDURATION:4\n" +
	"#EXTINF:4.0,\n" +
	"seg0.ts\n" +
	"#EXTINF:2.5,\n" +
	"sub/seg1.ts\n" +
	"#EXT-X-ENDLIST\n"

func server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(master))
	})
	mux.HandleFunc("/media.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(media))
	})
	mux.HandleFunc("/hi/media.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(media))
	})
	mux.HandleFunc("/seg0.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte{0x47, 0x00, 0x01})
	})
	mux.HandleFunc("/sub/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte{0x47, 0x02, 0x03})
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestListMasterVariants(t *testing.T) {
	s := server(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: "hls://" + strings.TrimPrefix(s.URL, "http://") + "/master.m3u8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "200 OK" {
		t.Fatalf("status = %q", res.Status)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d, want 2: %+v", len(res.Events), res.Events)
	}
	want := s.URL + "/media.m3u8"
	if res.Events[0].Data != want {
		t.Fatalf("variant = %q, want %q", res.Events[0].Data, want)
	}
	if !strings.Contains(string(res.Body), "#EXT-X-STREAM-INF") {
		t.Fatal("body is not the master playlist")
	}
}

func TestVariantPlaylistSegments(t *testing.T) {
	s := server(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET",
		URL:    s.URL + "/master.m3u8",
		Options: map[string]string{
			"variant": "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// two variants plus the two segments of the selected variant
	if len(res.Events) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(res.Events), res.Events)
	}
	if res.Events[2].Name != "4.0s" || res.Events[2].Data != s.URL+"/seg0.ts" {
		t.Fatalf("segment 0 = %+v", res.Events[2])
	}
	if res.Events[3].Data != s.URL+"/sub/seg1.ts" {
		t.Fatalf("segment 1 = %+v", res.Events[3])
	}
	if res.Encoding != "text" || !strings.Contains(string(res.Body), "#EXTINF") {
		t.Fatalf("body = %q encoding = %q", res.Body, res.Encoding)
	}
}

func TestFetchSegment(t *testing.T) {
	s := server(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method:  "SEGMENT",
		URL:     s.URL + "/media.m3u8",
		Options: map[string]string{"segment": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Encoding != "base64" {
		t.Fatalf("encoding = %q", res.Encoding)
	}
	if string(res.Body) != string([]byte{0x47, 0x02, 0x03}) {
		t.Fatalf("body = %v", res.Body)
	}
	if len(res.Events) != 3 {
		t.Fatalf("events = %d, want 3: %+v", len(res.Events), res.Events)
	}
}

func TestFlatPlaylistAndBadInput(t *testing.T) {
	s := server(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: s.URL + "/media.m3u8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(res.Events))
	}

	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: "ftp://example.com/x.m3u8",
	}); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEGMENT", URL: s.URL + "/media.m3u8",
		Options: map[string]string{"segment": "9"},
	}); err == nil {
		t.Fatal("expected out of range error")
	}
}
