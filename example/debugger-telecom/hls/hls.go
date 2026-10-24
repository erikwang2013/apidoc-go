// Package hls is an apidoc debug adapter for HTTP Live Streaming playlists
// and segments. It only uses the standard library: a playlist is fetched,
// parsed into its segment list, and optionally one segment is downloaded.
package hls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

const maxBody = 4 << 20

// New returns the HLS debugger. It is a one-shot adapter: GET fetches and
// parses a playlist, SEGMENT fetches one segment by index.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("hls", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"GET", "SEGMENT"},
		Encodings:    []string{"text", "base64"},
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "segment", Label: "Segment index (SEGMENT)", Type: "number", Default: "0"},
			{Name: "variant", Label: "Variant index (master playlist)", Type: "number", Default: ""},
			{Name: "timeout_ms", Label: "HTTP timeout (ms)", Type: "number", Default: "10000"},
		},
	}, exec)
}

type segment struct {
	duration string
	uri      string
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "SEGMENT" {
		return nil, fmt.Errorf("hls: unsupported method %q", req.Method)
	}
	target, err := fetchURL(req.URL)
	if err != nil {
		return nil, err
	}

	timeout := optionDuration(req.Options, "timeout_ms", 10*time.Second)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	body, status, contentType, err := get(ctx, target)
	if err != nil {
		return nil, err
	}
	res := &apidoc.DebugResponse{
		Status:   status,
		Headers:  map[string]string{"content-type": contentType},
		Duration: time.Since(start),
		Encoding: "text",
	}

	if !isPlaylist(body) {
		res.Body = body
		if !strings.HasPrefix(contentType, "text/") && !strings.Contains(contentType, "mpegurl") {
			res.Encoding = "base64"
		}
		return res, nil
	}

	pl, err := parse(body, target)
	if err != nil {
		return nil, err
	}

	// A master playlist points at variant playlists; --variant selects one.
	if len(pl.variants) > 0 && len(pl.segments) == 0 {
		for _, v := range pl.variants {
			res.Events = append(res.Events, event("variant", v))
		}
		idx, ok := optionInt(req.Options, "variant")
		if !ok {
			res.Body = body
			return res, nil
		}
		if idx < 0 || idx >= len(pl.variants) {
			return nil, fmt.Errorf("hls: variant %d out of range (have %d)", idx, len(pl.variants))
		}
		target, err = url.Parse(pl.variants[idx])
		if err != nil {
			return nil, err
		}
		body, status, contentType, err = get(ctx, target)
		if err != nil {
			return nil, err
		}
		res.Status = status
		res.Headers["content-type"] = contentType
		pl, err = parse(body, target)
		if err != nil {
			return nil, err
		}
	}

	for _, s := range pl.segments {
		res.Events = append(res.Events, event(durName(s.duration), s.uri))
	}
	if method == "SEGMENT" {
		idx, _ := optionInt(req.Options, "segment")
		if idx < 0 || idx >= len(pl.segments) {
			return nil, fmt.Errorf("hls: segment %d out of range (have %d)", idx, len(pl.segments))
		}
		segURL, err := url.Parse(pl.segments[idx].uri)
		if err != nil {
			return nil, err
		}
		data, status, contentType, err := get(ctx, segURL)
		if err != nil {
			return nil, err
		}
		res.Status = status
		res.Headers["content-type"] = contentType
		res.Body = data
		res.Encoding = "base64"
		res.Events = append(res.Events, event("fetched "+durName(pl.segments[idx].duration), segURL.String()))
		return res, nil
	}

	res.Body = body
	return res, nil
}

func fetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("hls: bad url: %w", err)
	}
	switch u.Scheme {
	case "hls":
		u.Scheme = "http"
	case "hlss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return nil, fmt.Errorf("hls: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("hls: url has no host")
	}
	return u, nil
}

func get(ctx context.Context, u *url.URL) (body []byte, status, contentType string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, "", "", err
	}
	return body, resp.Status, resp.Header.Get("Content-Type"), nil
}

func isPlaylist(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U")
}

type playlist struct {
	segments []segment
	variants []string
}

// parse understands the subset of RFC 8216 needed to list segments:
// EXTINF, segment URIs and EXT-X-STREAM-INF variant URIs.
func parse(body []byte, base *url.URL) (*playlist, error) {
	pl := &playlist{}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	duration := ""
	variant := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			duration = strings.TrimPrefix(line, "#EXTINF:")
			if i := strings.IndexByte(duration, ','); i >= 0 {
				duration = duration[:i]
			}
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			variant = true
		case strings.HasPrefix(line, "#"):
			// other tags are unrelated to the transcript
		default:
			ref, err := url.Parse(line)
			if err != nil {
				return nil, fmt.Errorf("hls: bad uri %q: %w", line, err)
			}
			resolved := base.ResolveReference(ref).String()
			if variant {
				pl.variants = append(pl.variants, resolved)
				variant = false
			} else {
				pl.segments = append(pl.segments, segment{duration: duration, uri: resolved})
				duration = ""
			}
		}
	}
	if len(pl.segments) == 0 && len(pl.variants) == 0 {
		return nil, errors.New("hls: playlist contains no segments or variants")
	}
	return pl, nil
}

func event(name, data string) apidoc.DebugEvent {
	return apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: name, Data: data, Encoding: "text"}
}

func durName(d string) string {
	if d == "" {
		return "segment"
	}
	return d + "s"
}

func optionDuration(opts map[string]string, key string, def time.Duration) time.Duration {
	if v := opts[key]; v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

func optionInt(opts map[string]string, key string) (int, bool) {
	v := strings.TrimSpace(opts[key])
	if v == "" {
		if key == "segment" {
			return 0, false
		}
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
