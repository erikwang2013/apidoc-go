// echo.thrift is the service exercised by the example adapter and its test
// server. The Thrift IDL compiler is not required: thriftdbg hand-writes the
// two structs (echo_args, echo_result) with the official runtime library.

namespace go echo

service Echo {
  string echo(1: string msg)
}
