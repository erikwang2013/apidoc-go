// Package amqp implements an apidoc StreamDebugger for AMQP 0-9-1
// (RabbitMQ) using amqp091-go.
package amqp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	amqp091 "github.com/rabbitmq/amqp091-go"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-messaging/internal/msg"
)

// New returns an AMQP debugger. URL form: amqp://host:5672/queue
// (amqps:// enables TLS). The path names the queue; exchange and routing
// key can be overridden via options.
func New() apidoc.StreamDebugger {
	return &debugger{dial: dialURLDefault}
}

var _ apidoc.StreamDebugger = (*debugger)(nil)

type debugger struct {
	dial func(string) (amqpConn, error)
}

func (d *debugger) Protocol() string { return "amqp" }

func (d *debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"PUBLISH", "CONSUME"},
		Encodings:    msg.Encodings,
		BodyIn:       "payload",
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "username", Label: "Username", Type: "text"},
			{Name: "password", Label: "Password", Type: "password", Secret: true},
			{Name: "vhost", Label: "Virtual host", Type: "text", Default: "/"},
		},
	}
}

// Exec publishes one message and disconnects.
func (d *debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if !strings.EqualFold(req.Method, "PUBLISH") {
		return nil, fmt.Errorf("amqp: %s needs a streaming session (subscribe/consume); use PUBLISH here", req.Method)
	}
	start := time.Now()
	u, queue, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	payload, err := msg.Payload(req.Body, req.Encoding)
	if err != nil {
		return nil, err
	}
	conn, err := d.dial(dialURL(u, req.Options))
	if err != nil {
		return nil, fmt.Errorf("amqp: connect: %w", err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("amqp: channel: %w", err)
	}
	defer ch.Close()

	exchange, routingKey := publishTarget(queue, req.Options)
	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp091.Publishing{
		ContentType: msg.Option(req.Headers, "content-type", "text/plain"),
		Timestamp:   time.Now(),
		Headers:     table(req.Headers),
		Body:        payload,
	}); err != nil {
		return nil, fmt.Errorf("amqp: publish: %w", err)
	}
	return &apidoc.DebugResponse{
		Status:   "PUBLISHED",
		Headers:  map[string]string{"exchange": exchange, "routing_key": routingKey},
		Duration: time.Since(start),
	}, nil
}

// Open consumes the queue and streams deliveries until Close.
func (d *debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, queue, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	conn, err := d.dial(dialURL(u, req.Options))
	if err != nil {
		return nil, fmt.Errorf("amqp: connect: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("amqp: channel: %w", err)
	}
	// Passive declare accepts an existing queue with any arguments; only
	// create it (with debug defaults) when it does not exist yet. A 404
	// closes the channel, so the fallback needs a fresh one.
	q, err := ch.QueueDeclarePassive(queue, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		ch, err = conn.Channel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("amqp: channel: %w", err)
		}
		q, err = ch.QueueDeclare(queue, false, false, false, false, nil)
		if err != nil {
			ch.Close()
			conn.Close()
			return nil, fmt.Errorf("amqp: declare queue %q: %w", queue, err)
		}
	}
	sctx, cancel := context.WithCancel(context.Background())
	autoAck := msg.BoolOption(req.Options, "auto_ack", true)
	deliveries, err := ch.ConsumeWithContext(sctx, q.Name, "", autoAck, false, false, false, nil)
	if err != nil {
		cancel()
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("amqp: consume %q: %w", q.Name, err)
	}
	exchange, routingKey := publishTarget(q.Name, req.Options)
	s := &session{
		conn: conn, ch: ch, queue: q.Name, exchange: exchange, routingKey: routingKey,
		autoAck: autoAck, events: msg.NewEventQueue(), cancel: cancel,
		deliveries: deliveries, done: make(chan struct{}),
	}
	go s.consume(sctx)
	return s, nil
}

type session struct {
	conn       amqpConn
	ch         amqpChannel
	queue      string
	exchange   string
	routingKey string
	autoAck    bool
	events     *msg.EventQueue
	cancel     context.CancelFunc
	deliveries <-chan amqp091.Delivery
	done       chan struct{}
	once       sync.Once
}

func (s *session) consume(ctx context.Context) {
	defer close(s.done)
	defer s.events.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case delivery, ok := <-s.deliveries:
			if !ok {
				return
			}
			s.events.Push(msg.In(delivery.RoutingKey, delivery.Body))
			if !s.autoAck {
				_ = delivery.Ack(false)
			}
		}
	}
}

func (s *session) Send(ctx context.Context, payload []byte, meta map[string]string) error {
	payload, err := msg.Payload(payload, meta["encoding"])
	if err != nil {
		return err
	}
	exchange := msg.Option(meta, "exchange", s.exchange)
	routingKey := msg.Option(meta, "routing_key", s.routingKey)
	if err := s.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp091.Publishing{
		ContentType: msg.Option(meta, "content-type", "text/plain"),
		Timestamp:   time.Now(),
		Headers:     metaHeaders(meta),
		Body:        payload,
	}); err != nil {
		return fmt.Errorf("amqp: publish: %w", err)
	}
	s.events.Push(msg.Out(s.queue, payload))
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events.Ch() }

func (s *session) Close() error {
	s.once.Do(func() {
		s.cancel()
		_ = s.ch.Close()
		_ = s.conn.Close()
		<-s.done
	})
	return nil
}

func target(raw string) (*url.URL, string, error) {
	u, err := msg.URL(raw, "amqp", "amqp", "amqps")
	if err != nil {
		return nil, "", err
	}
	queue := strings.TrimPrefix(u.Path, "/")
	if queue == "" {
		return nil, "", fmt.Errorf("amqp: url %q has no queue path", raw)
	}
	return u, queue, nil
}

// dialURL rebuilds the client URI: the debug URL path is the queue, not the
// AMQP virtual host.
func dialURL(u *url.URL, opts map[string]string) string {
	du := url.URL{Scheme: u.Scheme, Host: u.Host}
	user := msg.Option(opts, "username", "")
	pass := msg.Option(opts, "password", "")
	if user == "" && u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}
	if user != "" || pass != "" {
		du.User = url.UserPassword(user, pass)
	}
	if vhost := msg.Option(opts, "vhost", ""); vhost != "" && vhost != "/" {
		du.Path = "/" + vhost
	}
	return du.String()
}

func publishTarget(queue string, opts map[string]string) (exchange, routingKey string) {
	return msg.Option(opts, "exchange", ""), msg.Option(opts, "routing_key", queue)
}

func table(values map[string]string) amqp091.Table {
	if len(values) == 0 {
		return nil
	}
	out := make(amqp091.Table, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

// metaHeaders drops send controls that ride in meta rather than being headers.
func metaHeaders(meta map[string]string) amqp091.Table {
	out := make(amqp091.Table, len(meta))
	for k, v := range meta {
		switch k {
		case "encoding", "exchange", "routing_key", "content-type":
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// amqpConn/amqpChannel are the subset of amqp091 used here. They let tests
// run against an in-memory broker; *amqp091.Channel satisfies amqpChannel.
type amqpConn interface {
	Channel() (amqpChannel, error)
	Close() error
}

type amqpChannel interface {
	QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error
	ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
	Close() error
}

var _ amqpChannel = (*amqp091.Channel)(nil)

type realConn struct{ *amqp091.Connection }

func (c realConn) Channel() (amqpChannel, error) {
	ch, err := c.Connection.Channel()
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func dialURLDefault(raw string) (amqpConn, error) {
	conn, err := amqp091.Dial(raw)
	if err != nil {
		return nil, err
	}
	return realConn{conn}, nil
}
