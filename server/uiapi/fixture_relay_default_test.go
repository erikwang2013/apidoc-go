//go:build !apidoc_debug_http && !apidoc_debug_sockets && !apidoc_debug_rpc && !apidoc_debug_all

package uiapi

// fixtureRelay keeps the shared fixture's empty relay list for the default
// build, where no relay executors are compiled.
func fixtureRelay(_, fixture any) any { return fixture }
