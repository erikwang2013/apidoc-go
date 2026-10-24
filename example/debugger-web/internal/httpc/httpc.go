// Package httpc provides the adapters' shared HTTP client. Redirects are not
// followed, so a debug target cannot redirect the request past the library's
// target policy check.
package httpc

import "net/http"

// Client returns 3xx responses to the caller instead of following them.
var Client = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
