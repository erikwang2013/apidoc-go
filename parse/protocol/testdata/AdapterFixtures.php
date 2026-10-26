<?php
declare(strict_types = 1);

namespace Tests\Fixtures\Protocol;

use erikwang2013\apidoc\annotation\Definition;
use erikwang2013\apidoc\annotation\Protocol;
use erikwang2013\apidoc\annotation\Title;

#[Protocol('rest')]
class OpenApiFixture
{
    #[Definition(file: 'tests/fixtures/protocol/openapi/api.yaml', operation: 'getUser')]
    #[Title('Get user')]
    public function getUser() {}

    #[Definition(file: 'tests/fixtures/protocol/openapi/api.yaml', operation: 'createUser')]
    #[Title('Create user')]
    public function createUser() {}

    #[Definition(file: 'tests/fixtures/protocol/openapi/swagger.yaml', operation: 'ping')]
    #[Title('Ping')]
    public function ping() {}
}

#[Protocol('mqtt')]
class AsyncApiFixture
{
    #[Channel('/sensors/+/temp', action: 'subscribe')]
    #[Definition(file: 'tests/fixtures/protocol/asyncapi/asyncapi.yaml', operation: 'sensors/+/temp:publish')]
    #[Title('Temperature')]
    public function onTemp() {}
}

#[Protocol('graphql')]
class GraphqlFixture
{
    #[Definition(file: 'tests/fixtures/protocol/graphql/schema.graphql', operation: 'Query.user')]
    #[Title('User')]
    public function user() {}
}

#[Protocol('grpc')]
class ProtoFixture
{
    #[Definition(file: 'tests/fixtures/protocol/proto/helloworld.proto', operation: 'helloworld.Greeter/SayHello')]
    #[Title('SayHello')]
    public function sayHello() {}

    #[Definition(file: 'tests/fixtures/protocol/proto/helloworld.proto', operation: 'helloworld.Greeter/Search')]
    #[Title('Search')]
    public function search() {}
}

#[Protocol('grpc')]
class ProtoBadFixture
{
    #[Definition(file: 'tests/fixtures/protocol/proto/unsupported.proto')]
    #[Title('Bad')]
    public function bad() {}
}

#[Protocol('soap')]
class WsdlFixture
{
    #[Definition(file: 'tests/fixtures/protocol/wsdl/calculator.wsdl', operation: 'CalculatorService/CalculatorPort/Add')]
    #[Title('Add')]
    public function add() {}
}

#[Protocol('odata')]
class EdmxFixture
{
    #[Definition(file: 'tests/fixtures/protocol/edmx/metadata.xml', operation: 'Products')]
    #[Title('Products')]
    public function products() {}
}

#[Protocol('json-rpc')]
class JsonRpcFixture
{
    #[Definition(file: 'tests/fixtures/protocol/json-rpc/methods.json', operation: 'user.get', sample: '{"jsonrpc":"2.0","method":"user.get","params":{"id":1},"id":1}')]
    #[Title('Get user')]
    public function getUser() {}

    #[Definition(file: 'tests/fixtures/protocol/json-rpc/methods.json', operation: 'user.get', sample: '{"jsonrpc":"2.0","method":"user.get","params":{},"id":1}')]
    #[Title('Get user bad')]
    public function getUserBad() {}
}

#[Protocol('xml-rpc')]
class XmlRpcFixture
{
    #[Definition(file: 'tests/fixtures/protocol/xml-rpc/methods.xml', operation: 'add')]
    #[Title('Add')]
    public function add() {}
}
