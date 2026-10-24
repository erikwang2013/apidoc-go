// Package kafka implements an apidoc StreamDebugger for Kafka using franz-go.
package kafka

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-messaging/internal/msg"
)

// New returns a Kafka debugger. URL form: kafka://host:9092/topic
func New() apidoc.StreamDebugger { return debugger{} }

var _ apidoc.StreamDebugger = debugger{}

type debugger struct{}

func (debugger) Protocol() string { return "kafka" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"PRODUCE", "CONSUME"},
		Encodings:    msg.Encodings,
		BodyIn:       "payload",
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "client_id", Label: "Client ID", Type: "text", Default: "apidoc-debug"},
		},
	}
}

// Exec produces one record and returns its partition and offset.
func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if !strings.EqualFold(req.Method, "PRODUCE") {
		return nil, fmt.Errorf("kafka: %s needs a streaming session (subscribe/consume); use PRODUCE here", req.Method)
	}
	start := time.Now()
	u, topic, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	payload, err := msg.Payload(req.Body, req.Encoding)
	if err != nil {
		return nil, err
	}
	client, err := newClient(u, req.Options, "")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	record, err := client.ProduceSync(ctx, &kgo.Record{
		Topic: topic, Value: payload, Headers: headers(req.Headers),
	}).First()
	if err != nil {
		return nil, fmt.Errorf("kafka: produce: %w", err)
	}
	return &apidoc.DebugResponse{
		Status: "PRODUCED",
		Headers: map[string]string{
			"topic":     record.Topic,
			"partition": strconv.Itoa(int(record.Partition)),
			"offset":    strconv.FormatInt(record.Offset, 10),
		},
		Duration: time.Since(start),
	}, nil
}

// Open consumes the topic from the beginning and streams records until Close.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, topic, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	client, err := newClient(u, req.Options, topic)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("kafka: connect: %w", err)
	}
	sctx, cancel := context.WithCancel(context.Background())
	s := &session{
		client: client, topic: topic, cancel: cancel,
		events: msg.NewEventQueue(), done: make(chan struct{}),
	}
	go s.consume(sctx)
	return s, nil
}

type session struct {
	client *kgo.Client
	topic  string
	cancel context.CancelFunc
	events *msg.EventQueue
	done   chan struct{}
	once   sync.Once
}

func (s *session) consume(ctx context.Context) {
	defer close(s.done)
	defer s.events.Close()
	for {
		fetches := s.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			s.events.Push(msg.Err(fmt.Errorf("kafka: %s[%d]: %w", topic, partition, err)))
		})
		fetches.EachRecord(func(record *kgo.Record) {
			s.events.Push(msg.In(record.Topic, record.Value))
		})
	}
}

func (s *session) Send(ctx context.Context, payload []byte, meta map[string]string) error {
	payload, err := msg.Payload(payload, meta["encoding"])
	if err != nil {
		return err
	}
	_, err = s.client.ProduceSync(ctx, &kgo.Record{
		Topic: s.topic, Value: payload, Headers: headers(meta),
	}).First()
	if err != nil {
		return fmt.Errorf("kafka: produce: %w", err)
	}
	s.events.Push(msg.Out(s.topic, payload))
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events.Ch() }

func (s *session) Close() error {
	s.once.Do(func() {
		s.cancel()
		s.client.Close()
		<-s.done
	})
	return nil
}

func target(raw string) (*url.URL, string, error) {
	u, err := msg.URL(raw, "kafka", "kafka")
	if err != nil {
		return nil, "", err
	}
	topic := strings.TrimPrefix(u.Path, "/")
	if topic == "" {
		return nil, "", fmt.Errorf("kafka: url %q has no topic path", raw)
	}
	return u, topic, nil
}

func newClient(u *url.URL, opts map[string]string, consumeTopic string) (*kgo.Client, error) {
	kopts := []kgo.Opt{
		kgo.SeedBrokers(msg.Addr(u, "9092")),
		kgo.ClientID(msg.Option(opts, "client_id", "apidoc-debug")),
	}
	if consumeTopic != "" {
		kopts = append(kopts, kgo.ConsumeTopics(consumeTopic))
	}
	client, err := kgo.NewClient(kopts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: client: %w", err)
	}
	return client, nil
}

func headers(values map[string]string) []kgo.RecordHeader {
	if len(values) == 0 {
		return nil
	}
	out := make([]kgo.RecordHeader, 0, len(values))
	for k, v := range values {
		if k == "encoding" {
			continue
		}
		out = append(out, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	return out
}
