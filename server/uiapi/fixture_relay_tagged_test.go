//go:build apidoc_debug_http || apidoc_debug_sockets || apidoc_debug_rpc || apidoc_debug_grpc || apidoc_debug_http3 || apidoc_debug_all

package uiapi

// fixtureRelay accepts the live compiled relay list; TestDebugCapabilitiesTagged
// asserts the exact ids for the aggregate build.
func fixtureRelay(live, _ any) any { return live }

// fixtureModes accepts the compiled build's relay modes; the tagged
// capability test pins their exact values.
func fixtureModes(live, _ any) any { return live }
