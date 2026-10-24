// Package mqtt implements an apidoc StreamDebugger for MQTT 3.1.1.
package mqtt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-messaging/internal/msg"
)

// New returns an MQTT debugger. URL form: mqtt://host:1883/topic
// (mqtts:// enables TLS); subscribe topics may use +/# wildcards.
func New() apidoc.StreamDebugger { return debugger{} }

var _ apidoc.StreamDebugger = debugger{}

type debugger struct{}

func (debugger) Protocol() string { return "mqtt" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"PUBLISH", "SUBSCRIBE"},
		Encodings:    msg.Encodings,
		BodyIn:       "payload",
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "username", Label: "Username", Type: "text"},
			{Name: "password", Label: "Password", Type: "password", Secret: true},
			{Name: "client_id", Label: "Client ID", Type: "text", Placeholder: "auto"},
			{Name: "qos", Label: "QoS", Type: "select", Default: "0", Options: []string{"0", "1", "2"}},
		},
	}
}

// Exec publishes one message and disconnects.
func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if !strings.EqualFold(req.Method, "PUBLISH") {
		return nil, fmt.Errorf("mqtt: %s needs a streaming session (subscribe/consume); use PUBLISH here", req.Method)
	}
	start := time.Now()
	u, topic, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(topic, "+#") {
		return nil, fmt.Errorf("mqtt: publish topic %q must not contain wildcards", topic)
	}
	payload, err := msg.Payload(req.Body, req.Encoding)
	if err != nil {
		return nil, err
	}
	qos, err := parseQoS(req.Options)
	if err != nil {
		return nil, err
	}
	client, err := connect(ctx, u, req.Options, nil)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(250)

	if err := waitToken(ctx, client.Publish(topic, qos, msg.BoolOption(req.Options, "retained", false), payload)); err != nil {
		return nil, err
	}
	return &apidoc.DebugResponse{
		Status:   "PUBLISHED",
		Headers:  map[string]string{"topic": topic, "qos": strconv.Itoa(int(qos))},
		Duration: time.Since(start),
	}, nil
}

// Open subscribes and streams incoming messages until Close.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, topic, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	qos, err := parseQoS(req.Options)
	if err != nil {
		return nil, err
	}
	events := msg.NewEventQueue()
	client, err := connect(ctx, u, req.Options, func(err error) {
		events.Push(msg.Err(err))
		events.Close()
	})
	if err != nil {
		return nil, err
	}
	token := client.Subscribe(topic, qos, func(_ paho.Client, m paho.Message) {
		events.Push(msg.In(m.Topic(), m.Payload()))
	})
	if err := waitToken(ctx, token); err != nil {
		client.Disconnect(250)
		events.Close()
		return nil, err
	}
	return &session{
		client: client, topic: topic, qos: qos,
		retained: msg.BoolOption(req.Options, "retained", false),
		events:   events,
	}, nil
}

type session struct {
	client   paho.Client
	topic    string
	qos      byte
	retained bool
	events   *msg.EventQueue
	once     sync.Once
}

func (s *session) Send(ctx context.Context, payload []byte, meta map[string]string) error {
	payload, err := msg.Payload(payload, meta["encoding"])
	if err != nil {
		return err
	}
	if err := waitToken(ctx, s.client.Publish(s.topic, s.qos, s.retained, payload)); err != nil {
		return err
	}
	s.events.Push(msg.Out(s.topic, payload))
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events.Ch() }

func (s *session) Close() error {
	s.once.Do(func() {
		if token := s.client.Unsubscribe(s.topic); token.WaitTimeout(2 * time.Second) {
			_ = token.Error()
		}
		s.client.Disconnect(250)
		s.events.Close()
	})
	return nil
}

func target(raw string) (*url.URL, string, error) {
	u, err := msg.URL(raw, "mqtt", "mqtt", "mqtts")
	if err != nil {
		return nil, "", err
	}
	topic := strings.TrimPrefix(u.Path, "/")
	if topic == "" {
		return nil, "", fmt.Errorf("mqtt: url %q has no topic path", raw)
	}
	return u, topic, nil
}

func connect(ctx context.Context, u *url.URL, opts map[string]string, lost func(error)) (paho.Client, error) {
	o := paho.NewClientOptions().
		SetClientID(msg.Option(opts, "client_id", fmt.Sprintf("apidoc-%d", time.Now().UnixNano()))).
		SetAutoReconnect(false).
		SetConnectTimeout(5 * time.Second)
	if user := msg.Option(opts, "username", ""); user != "" {
		o.SetUsername(user)
	}
	if pass := msg.Option(opts, "password", ""); pass != "" {
		o.SetPassword(pass)
	}
	if u.Scheme == "mqtts" {
		o.AddBroker("ssl://" + msg.Addr(u, "8883"))
		o.SetTLSConfig(&tls.Config{ServerName: u.Hostname()})
	} else {
		o.AddBroker("tcp://" + msg.Addr(u, "1883"))
	}
	if lost != nil {
		o.SetConnectionLostHandler(func(_ paho.Client, err error) { lost(err) })
	}
	client := paho.NewClient(o)
	if err := waitToken(ctx, client.Connect()); err != nil {
		client.Disconnect(0)
		return nil, err
	}
	return client, nil
}

func waitToken(ctx context.Context, token paho.Token) error {
	if !token.WaitTimeout(msg.WaitTimeout(ctx, 5*time.Second)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("mqtt: broker did not respond in time")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: %w", err)
	}
	return nil
}

func parseQoS(opts map[string]string) (byte, error) {
	switch v := msg.Option(opts, "qos", "0"); v {
	case "0":
		return 0, nil
	case "1":
		return 1, nil
	case "2":
		return 2, nil
	default:
		return 0, fmt.Errorf("mqtt: invalid qos %q", v)
	}
}
