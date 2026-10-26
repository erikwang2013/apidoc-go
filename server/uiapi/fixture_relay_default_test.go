//go:build !apidoc_debug_http && !apidoc_debug_sockets && !apidoc_debug_rpc && !apidoc_debug_grpc && !apidoc_debug_http3 && !apidoc_debug_messaging && !apidoc_debug_mqtt && !apidoc_debug_stomp && !apidoc_debug_amqp && !apidoc_debug_kafka && !apidoc_debug_xmpp && !apidoc_debug_iot && !apidoc_debug_industrial && !apidoc_debug_all

package uiapi

// fixtureRelay keeps the shared fixture's empty relay list for the default
// build, where no relay executors are compiled.
func fixtureRelay(_, fixture any) any { return fixture }

// fixtureModes keeps the fixture's browser-only mode list for the default
// build, which compiles no relay executor.
func fixtureModes(_, fixture any) any { return fixture }
