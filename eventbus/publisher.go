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
	Environment string          `json:"environment"`
	AggregateID string          `json:"aggregate_id"`
	Data        json.RawMessage `json:"data"`
}

type Publisher struct {
	js          nats.JetStreamContext
	environment string
}

func NewPublisher(url, token, environment string) (*Publisher, func(), error) {
	if !validEnvironment(environment) {
		return nil, nil, fmt.Errorf("explicit event environment required")
	}
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
	return &Publisher{js: js, environment: environment}, nc.Close, nil
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
	if !validEnvironment(p.environment) {
		return fmt.Errorf("explicit event environment required")
	}
	var scope struct {
		Environment string `json:"environment"`
	}
	if json.Unmarshal(event.Payload, &scope) != nil || (scope.Environment != "" && scope.Environment != p.environment) {
		return fmt.Errorf("event environment mismatch")
	}
	data, err := json.Marshal(envelope{
		ID: fmt.Sprintf("decision-outbox-%d", event.ID), Type: event.Type, Version: "1",
		OccurredAt: event.OccurredAt.UTC(), TenantID: event.TenantID, Environment: p.environment, AggregateID: event.AggregateID, Data: event.Payload,
	})
	if err != nil {
		return err
	}
	subject := "mightyfin.decision." + strings.ReplaceAll(event.Type, " ", "-")
	_, err = p.js.Publish(subject, data, nats.MsgId(fmt.Sprintf("%s:decision-outbox-%d", p.environment, event.ID)), nats.Context(ctx))
	return err
}

func validEnvironment(v string) bool {
	switch v {
	case "local", "dev", "staging", "sandbox", "production":
		return true
	}
	return false
}
