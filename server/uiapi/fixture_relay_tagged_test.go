//go:build apidoc_debug_http || apidoc_debug_sockets || apidoc_debug_rpc || apidoc_debug_all

package uiapi

// fixtureRelay accepts the live compiled relay list; TestDebugCapabilitiesTagged
// asserts the exact ids for the aggregate build.
func fixtureRelay(live, _ any) any { return live }
