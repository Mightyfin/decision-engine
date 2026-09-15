package eventbus

import (
	"context"
	"encoding/json"
	"github.com/nats-io/nats.go"
	"os"
	"testing"
	"time"
)

func TestRestrictedPublisherDurableReplay(t *testing.T) {
	url := os.Getenv("EVENTBUS_ACL_TEST_URL")
	if url == "" {
		t.Skip("isolated ACL broker required")
	}
	nc, err := nats.Connect(url, nats.UserInfo("bootstrap", os.Getenv("EVENTBUS_ACL_TEST_BOOTSTRAP_PASSWORD")))
	if err != nil {
		t.Fatal("bootstrap connection failed")
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = js.StreamInfo(streamName); err != nats.ErrStreamNotFound {
		t.Fatal("absent test stream required")
	}
	if _, err = js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"mightyfin.decision.>"}, Storage: nats.MemoryStorage}); err != nil {
		t.Fatal(err)
	}
	defer js.DeleteStream(streamName)
	opts, err := ConnectionCredentials("", "decision-publisher", os.Getenv("EVENTBUS_ACL_TEST_PUBLISHER_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	p, closeP, err := NewPublisher(url, "", "sandbox", opts...)
	if err != nil {
		t.Fatal("publisher connection failed")
	}
	defer closeP()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = p.EnsureStream(ctx); err != nil {
		t.Fatal("restricted stream info", err)
	}
	event := Event{ID: 1, Type: "credit.offer.accepted", AggregateID: "synthetic-aggregate", TenantID: "synthetic-tenant", Payload: json.RawMessage(`{}`), OccurredAt: time.Now().UTC()}
	for i := 0; i < 2; i++ {
		if err = p.Publish(ctx, event); err != nil {
			t.Fatal("authorized durable publish", err)
		}
	}
	info, err := js.StreamInfo(streamName)
	if err != nil || info.State.Msgs != 1 {
		t.Fatal("duplicate durable event", err)
	}
	msg, err := js.GetMsg(streamName, 1)
	if err != nil || msg.Subject != "mightyfin.decision.credit.offer.accepted" {
		t.Fatal("event subject mismatch", err)
	}
	var data struct {
		Tenant      string `json:"tenant_id"`
		Environment string `json:"environment"`
	}
	if json.Unmarshal(msg.Data, &data) != nil || data.Tenant != "synthetic-tenant" || data.Environment != "sandbox" {
		t.Fatal("scope lost")
	}
}
