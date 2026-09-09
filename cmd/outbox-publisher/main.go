package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Mightyfin/decision-engine/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
)

const idleInterval = time.Second

func main() {
	databaseURL, natsURL := os.Getenv("DECISION_ENGINE_DATABASE_URL"), os.Getenv("DECISION_ENGINE_NATS_URL")
	if databaseURL == "" || natsURL == "" {
		log.Fatal("DECISION_ENGINE_DATABASE_URL and DECISION_ENGINE_NATS_URL are required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	publisher, closePublisher, err := eventbus.NewPublisher(natsURL, os.Getenv("DECISION_ENGINE_NATS_TOKEN"), os.Getenv("DECISION_ENGINE_ENVIRONMENT"))
	if err != nil {
		log.Fatal(err)
	}
	defer closePublisher()
	if err := publisher.EnsureStream(ctx); err != nil {
		log.Fatal(err)
	}

	store := eventbus.Store{Pool: pool}
	for {
		event, err := store.Claim(ctx)
		if err != nil {
			log.Printf("claim outbox event: %v", err)
			time.Sleep(idleInterval)
			continue
		}
		if event == nil {
			time.Sleep(idleInterval)
			continue
		}
		if err := publisher.Publish(ctx, *event); err != nil {
			log.Printf("publish outbox event %d: %v", event.ID, err)
			if failErr := store.Fail(ctx, event.ID, err.Error(), retryDelay(event.ID)); failErr != nil {
				log.Printf("reschedule outbox event %d: %v", event.ID, failErr)
			}
			continue
		}
		if err := store.Complete(ctx, event.ID); err != nil {
			log.Printf("complete outbox event %d: %v", event.ID, err)
		}
	}
}

func retryDelay(eventID int64) time.Duration {
	// A fixed, bounded delay avoids a failing broker creating a hot database loop.
	return 5*time.Second + time.Duration(eventID%5)*time.Second
}
