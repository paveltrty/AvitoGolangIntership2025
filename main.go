package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"Avito2025/internal/config"
	"Avito2025/internal/service"
	"Avito2025/internal/storage"
	"Avito2025/internal/storage/postgres"
	httptransport "Avito2025/internal/transport/http"
)

func main() {
	cfg := config.Load()

	repo, cleanup, err := buildRepository(context.Background(), cfg)
	if err != nil {
		log.Fatalf("init repository: %v", err)
	}
	defer cleanup()

	svc := service.New(repo)
	handler := httptransport.NewHandler(svc)

	server := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: handler.Router(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("HTTP server listening on %s (storage=%s)", cfg.HTTP.Addr, cfg.Storage.Type)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
}

func buildRepository(ctx context.Context, cfg config.Config) (storage.Repository, func(), error) {
	switch cfg.Storage.Type {
	case "postgres":
		store, err := postgres.New(ctx, cfg.Storage.Postgres)
		if err != nil {
			return nil, nil, err
		}
		return store, store.Close, nil
	default:
		return nil, nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}
