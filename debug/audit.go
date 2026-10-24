package debug

import "time"

// AuditEvent is one debug operation summary passed to Config.DebugAudit.
// It deliberately has no params, body or options fields so an audit log
// cannot leak message contents or secrets.
type AuditEvent struct {
	Time     time.Time
	ActionID string
	App      string
	Protocol string
	Method   string
	Target   string
	Status   string
	Duration time.Duration
	Bytes    int
	Error    string
}
