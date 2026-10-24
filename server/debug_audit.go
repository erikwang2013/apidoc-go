package server

import (
	"context"
	"net/url"
	"time"

	"github.com/erikwang2013/apidoc-go/debug"
)

// redactTarget strips URL userinfo before it reaches audit/log output.
func redactTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

const (
	auditOK      = "ok"
	auditError   = "error"
	auditTimeout = "timeout"
	auditDenied  = "denied"
)

// audit emits one event. A panicking callback must not take the debug
// endpoint down with it.
func (h *apiHandler) audit(ev debug.AuditEvent) {
	if h.o.DebugAudit == nil {
		return
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	defer func() { _ = recover() }()
	h.o.DebugAudit(ev)
}

// policyErr checks the optional library target policy before an adapter
// call; nil means allowed.
func (h *apiHandler) policyErr(ctx context.Context, target string) error {
	if h.o.DebugPolicy == nil {
		return nil
	}
	return h.o.DebugPolicy.Check(ctx, target)
}
