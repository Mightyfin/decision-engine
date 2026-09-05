package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const streamName = "DECISION_ENGINE_EVENTS"

type envelope struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     string          `json:"version"`
	OccurredAt  time.Time       `json:"occurred_at"`
	TenantID    string          `json:"tenant_id"`
	AggregateID string          `json:"aggregate_id"`
	Data        json.RawMessage `json:"data"`
}

type Publisher struct{ js nats.JetStreamContext }

func NewPublisher(url, token string) (*Publisher, func(), error) {
	options := []nats.Option{nats.Name("decision-engine-outbox-publisher")}
	if token != "" {
		options = append(options, nats.Token(token))
	}
	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	return &Publisher{js: js}, nc.Close, nil
}

func (p *Publisher) EnsureStream(ctx context.Context) error {
	_, err := p.js.StreamInfo(streamName, nats.Context(ctx))
	if err == nil {
		return nil
	}
	if err != nats.ErrStreamNotFound {
		return err
	}
	_, err = p.js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"mightyfin.decision.>"}}, nats.Context(ctx))
	return err
}

func (p *Publisher) Publish(ctx context.Context, event Event) error {
	data, err := json.Marshal(envelope{
		ID: fmt.Sprintf("decision-outbox-%d", event.ID), Type: event.Type, Version: "1",
		OccurredAt: event.OccurredAt.UTC(), TenantID: event.TenantID, AggregateID: event.AggregateID, Data: event.Payload,
	})
	if err != nil {
		return err
	}
	subject := "mightyfin.decision." + strings.ReplaceAll(event.Type, " ", "-")
	_, err = p.js.Publish(subject, data, nats.MsgId(fmt.Sprintf("decision-outbox-%d", event.ID)), nats.Context(ctx))
	return err
}
