<?php

namespace Tests\Fixtures\Protocol;

use erikwang2013\apidoc\annotation\{Protocol, Channel, Debug, Definition, Param, Returned, Title, Method, Url};

#[Protocol('mqtt')]
class MqttFixture
{
    #[Channel('/sensors/+/temp', action: 'publish')]
    #[Debug(driver: 'mqtt', target: 'tcp://broker:1883', encoding: 'json', meta: ['qos' => 1])]
    #[Param(name: 'value', type: 'number', require: true)]
    #[Title('Publish temperature')]
    public function publishTemperature() {}
}

#[Protocol('grpc')]
class GrpcFixture
{
    #[Channel('helloworld.Greeter/SayHello', action: 'call')]
    #[Debug(driver: 'grpc', target: 'localhost:50051')]
    #[Title('SayHello')]
    public function sayHello() {}
}

#[Protocol('thrift')]
#[Debug(driver: 'thrift', target: 'tcp://127.0.0.1:19014', meta: ['method' => 'ping'])]
class ThriftFixture
{
    #[Channel('ping', action: 'call')]
    #[Title('Thrift ping')]
    public function ping() {}
}

#[Protocol('dubbo')]
#[Debug(driver: 'dubbo', target: 'tcp://127.0.0.1:19015', meta: ['method' => 'echo', 'service' => 'com.demo.Echo'])]
class DubboFixture
{
    #[Channel('com.demo.Echo#echo', action: 'call')]
    #[Title('Dubbo echo')]
    public function echo() {}
}

#[Protocol('http2')]
#[Debug(target: 'http://127.0.0.1:19016/echo')]
class Http2Fixture
{
    #[Channel('/echo', action: 'call')]
    #[Title('HTTP/2 echo')]
    public function echo() {}
}

#[Protocol('http3')]
#[Debug(target: 'https://127.0.0.1:19016/echo')]
class Http3Fixture
{
    #[Channel('/echo', action: 'call')]
    #[Title('HTTP/3 echo')]
    public function echo() {}
}

#[Protocol('tcp')]
class PositionalDebugFixture
{
    #[Channel('tcp://127.0.0.1:9000', action: 'send')]
    #[Debug('raw')]
    public function send() {}
}

#[Protocol('mqtt')]
class DefaultDriverFixture
{
    #[Debug(target: 'tcp://broker:1883')]
    public function publish() {}
}

class PlainHttpFixture
{
    #[Method('GET')] #[Url('/plain')] #[Title('Plain')]
    public function plain() {}
}

class DebugOnlyFixture
{
    #[Method('GET')] #[Url('/ping')] #[Title('Ping')]
    #[Debug(driver: 'http', target: 'https://api.example.com/ping')]
    public function ping() {}
}

// 本机 19001 已被 minio 占用,测试 TCP 回显改用 19011
#[Protocol('tcp')]
#[Debug(driver:'raw', target:'tcp://127.0.0.1:19011')]
class RawFixture {
    #[Channel('echo')] #[Title('Echo')]
    public function echo() {}
}

#[Protocol('udp')]
#[Debug(driver:'raw', target:'udp://127.0.0.1:19002')]
class UdpRawFixture {
    #[Channel('echo')] #[Title('UDP echo')]
    public function echo() {}
}

#[Protocol('http')]
#[Debug(driver:'http-relay', target:'http://127.0.0.1:19005/echo.php')]
class HttpRelayFixture {
    #[Channel('/echo.php')] #[Title('HTTP echo')]
    public function echo() {}
}

#[Protocol('http')]
#[Debug(driver:'http-relay', target:'http://127.0.0.1:19005/redirect.php')]
class HttpRedirectFixture {
    #[Channel('/redirect.php')] #[Title('HTTP cross-host redirect')]
    public function echo() {}
}

#[Protocol('http')]
#[Debug(driver:'http-relay', target:'http://127.0.0.1:19005/redirect-same.php')]
class HttpSameOriginRedirectFixture {
    #[Channel('/redirect-same.php')] #[Title('HTTP same-origin redirect')]
    public function echo() {}
}

#[Protocol('http')]
#[Debug(driver:'http-relay', target:'http://127.0.0.1:19005/headers.php')]
class HttpHeadersFixture {
    #[Channel('/headers.php')] #[Title('HTTP headers echo')]
    public function echo() {}
}

#[Protocol('mqtt')]
class MqttPublishFixture
{
    #[Channel('/debug/topic', action: 'publish')]
    #[Debug(driver: 'mqtt', target: 'tcp://127.0.0.1:19003')]
    public function publish() {}
}

#[Protocol('mqtt')]
class MqttSubscribeFixture
{
    #[Channel('/debug/topic', action: 'subscribe')]
    #[Debug(driver: 'mqtt', target: 'tcp://127.0.0.1:19003')]
    public function subscribe() {}
}

#[Protocol('stomp')]
class StompSubscribeFixture
{
    #[Channel('/debug/queue', action: 'receive')]
    #[Debug(driver: 'stomp', target: 'tcp://127.0.0.1:19004')]
    public function receive() {}
}

#[Protocol('kafka')]
class KafkaDebugFixture
{
    #[Channel('debug', action: 'fetch')]
    #[Debug(driver: 'kafka', target: 'kafka://127.0.0.1:19021')]
    public function fetch() {}
}

#[Protocol('kafka')]
class KafkaMetadataHostFixture
{
    #[Channel('debug', action: 'fetch')]
    #[Debug(driver: 'kafka', target: 'kafka://127.0.0.1:19026')]
    public function fetch() {}
}

#[Protocol('amqp')]
class AmqpDebugFixture
{
    #[Channel('debug/queue', action: 'receive')]
    #[Debug(driver: 'amqp', target: 'amqp://127.0.0.1:19022')]
    public function receive() {}
}

#[Protocol('xmpp')]
class XmppDebugFixture
{
    #[Channel('debug@localhost', action: 'send')]
    #[Debug(driver: 'xmpp', target: 'xmpp://127.0.0.1:19023')]
    public function send() {}
}

#[Protocol('xmpp')]
class XmppNoStarttlsFixture
{
    #[Channel('debug@localhost', action: 'send')]
    #[Debug(driver: 'xmpp', target: 'xmpp://127.0.0.1:19024')]
    public function send() {}
}

#[Protocol('coap')]
class CoapDebugFixture
{
    #[Channel('/debug/coap', action: 'send')]
    #[Debug(driver: 'coap', target: 'udp://127.0.0.1:19006')]
    public function echo() {}
}

#[Protocol('modbus-tcp')]
class ModbusDebugFixture
{
    #[Channel('/debug/modbus', action: 'send')]
    #[Debug(driver: 'modbus-tcp', target: 'tcp://127.0.0.1:19007')]
    public function read() {}
}

#[Protocol('iso8583')]
class Iso8583ProxyFixture
{
    #[Channel('iso')]
    #[Title('ISO')]
    #[Debug(driver: 'iso8583', target: 'tcp://127.0.0.1:19008')]
    public function send() {}
}

#[Protocol('dnp3')]
class Dnp3DebugFixture
{
    #[Channel('/debug/dnp3')]
    #[Debug(driver: 'dnp3', target: 'dnp3://127.0.0.1:19030')]
    public function read() {}
}

#[Protocol('iec104')]
class Iec104DebugFixture
{
    #[Channel('/debug/iec104')]
    #[Debug(driver: 'iec104', target: 'iec104://127.0.0.1:19031')]
    public function interrogation() {}
}

#[Protocol('bacnet')]
class BacnetDebugFixture
{
    #[Channel('/debug/bacnet')]
    #[Debug(driver: 'bacnet', target: 'bacnet://127.0.0.1:19032')]
    public function whoIs() {}
}

#[Protocol('opcua')]
class OpcuaDebugFixture
{
    #[Channel('/debug/opcua')]
    #[Debug(driver: 'opcua', target: 'opc.tcp://127.0.0.1:19033')]
    public function read() {}
}

#[Protocol('rtp')]
class RtpDebugFixture
{
    #[Channel('/debug/rtp')]
    #[Debug(driver: 'rtp', target: 'rtp://127.0.0.1:19034')]
    public function send() {}
}

#[Protocol('rtcp')]
class RtcpDebugFixture
{
    #[Channel('/debug/rtcp')]
    #[Debug(driver: 'rtcp', target: 'rtcp://127.0.0.1:19035')]
    public function send() {}
}

#[Protocol('rtmp')]
class RtmpDebugFixture
{
    #[Channel('/debug/rtmp')]
    #[Debug(driver: 'rtmp', target: 'rtmp://127.0.0.1:19036')]
    public function connect() {}
}

#[Protocol('lwm2m')]
class Lwm2mDebugFixture
{
    #[Channel('/debug/lwm2m')]
    #[Debug(driver: 'lwm2m', target: 'lwm2m://127.0.0.1:19037')]
    public function read() {}
}

// 目录中本就以原始套接字为默认驱动的协议
// 特殊协议不注解 driver 时也必须回退到专用驱动,而不是 raw
#[Protocol('iec104')]
#[Debug(target: 'iec104://127.0.0.1:19031')]
class Iec104FallbackFixture
{
    #[Title('Interrogate')]
    public function interrogation() {}
}

#[Protocol('lwm2m')]
#[Debug(target: 'lwm2m://127.0.0.1:19037')]
class Lwm2mFallbackFixture
{
    #[Title('Read resource')]
    public function read() {}
}

#[Protocol('raw-socket')]
#[Channel('tcp://broker:9000')]
#[Debug(target: 'tcp://broker:9000')]
class P2RawFixture
{
    #[Title('Send raw frame')]
    public function publish() {}
}

#[Protocol('webrtc')]
class P2NoDriverFixture
{
    #[Title('Send WebRTC offer')]
    public function offer() {}
}

#[Protocol('json-rpc')]
class InlineFixture
{
    #[Param(name: 'id', type: 'integer', require: true, desc: 'ID')]
    #[Returned(name: 'ok', type: 'boolean', desc: 'OK')]
    #[Title('Inline')]
    public function call() {}
}

#[Protocol('webhooks')]
class JsonSchemaFixture
{
    #[Definition(file: 'tests/fixtures/protocol/json-schema/user.json', sample: '{"id":1,"name":"a"}')]
    #[Title('User')]
    public function user() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/invalid.json')]
    #[Title('Broken')]
    public function broken() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/webhook.json')]
    #[Title('Hook')]
    public function hook() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/user.json', sampleFile: 'tests/fixtures/protocol/json-schema/user-sample.json')]
    #[Title('User from file')]
    public function userFile() {}
}

#[Protocol('webhooks')]
class JsonSchemaRefsFixture
{
    #[Definition(file: 'tests/fixtures/protocol/json-schema/recursive.json')]
    #[Title('Recursive')]
    public function recursive() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/multi.json')]
    #[Title('Multi')]
    public function multi() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/chain.json', sample: '{"name":"a"}')]
    #[Title('Chain')]
    public function chain() {}

    #[Definition(file: 'tests/fixtures/protocol/json-schema/chain.json', sample: '{"name":5}')]
    #[Title('Chain invalid sample')]
    public function chainBad() {}
}

#[Protocol('webhooks')]
#[Definition(file: 'tests/fixtures/protocol/json-schema/user.json')]
class ClassDefinitionFixture
{
    #[Title('Inherited definition')]
    public function inherited() {}
}

#[Protocol('webhooks')]
class CacheFixture
{
    #[Definition(file: 'tests/fixtures/protocol/json-schema/cache.json')]
    #[Title('Cache')]
    public function user() {}
}

#[Protocol('http2')]
#[Debug(target: 'tcp://127.0.0.1:19016')]
class Http2SchemeMismatchFixture
{
    #[Title('HTTP/2 over tcp')]
    public function echo() {}
}

#[Protocol('http3')]
#[Debug(target: 'http://127.0.0.1:19016/echo')]
class Http3SchemeMismatchFixture
{
    #[Title('HTTP/3 over http')]
    public function echo() {}
}
