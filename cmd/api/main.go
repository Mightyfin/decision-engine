package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Mightyfin/decision-engine/auth"
	creditrisk "github.com/Mightyfin/decision-engine/credit-risk"
	"github.com/Mightyfin/decision-engine/httpapi"
	"github.com/Mightyfin/decision-engine/pricing"
	"github.com/Mightyfin/decision-engine/product"
	"github.com/Mightyfin/decision-engine/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		health()
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL, issuer, audience, environment := os.Getenv("DECISION_ENGINE_DATABASE_URL"), os.Getenv("DECISION_ENGINE_OIDC_ISSUER"), os.Getenv("DECISION_ENGINE_OIDC_AUDIENCE"), os.Getenv("DECISION_ENGINE_ENVIRONMENT")
	if databaseURL == "" || issuer == "" || audience == "" || environment == "" {
		log.Error("configuration rejected: database URL and OIDC settings are required")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	verifier, err := auth.NewOIDCVerifier(ctx, issuer, audience, environment)
	if err != nil {
		log.Error("OIDC unavailable", "error", err)
		os.Exit(1)
	}
	store := storage.Postgres{Pool: pool}
	credit := creditrisk.Service{Store: store, Products: product.Service{Store: store}, Pricing: pricing.QuoteFor}
	server := &http.Server{Addr: address(), Handler: httpapi.Server{Auth: verifier, Credit: credit, Applications: store, Pricing: store}.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func address() string {
	if value := os.Getenv("DECISION_ENGINE_HTTP_ADDRESS"); value != "" {
		return value
	}
	return ":8080"
}
func health() {
	port := strings.TrimPrefix(address(), ":")
	response, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://127.0.0.1:" + port + "/health/live")
	if err != nil || response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	_ = response.Body.Close()
}
