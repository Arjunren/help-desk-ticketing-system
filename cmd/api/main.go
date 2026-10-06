package main

import (
	"context"
	"github.com/Arjunren/help-desk-ticketing-system/internal/config"
	"github.com/Arjunren/help-desk-ticketing-system/internal/httpapi"
	"github.com/Arjunren/help-desk-ticketing-system/internal/postgres"
	"github.com/Arjunren/help-desk-ticketing-system/internal/service"
	"github.com/go-chi/cors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	svc := service.New(db, cfg.JWTSecret, cfg.JWTExpiry)
	handler := httpapi.New(svc, log).Router()
	handler = cors.Handler(cors.Options{AllowedOrigins: cfg.AllowedOrigins, AllowedMethods: []string{"GET", "POST", "PUT", "OPTIONS"}, AllowedHeaders: []string{"Authorization", "Content-Type"}, MaxAge: 300})(handler)
	server := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Info("server started", "address", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
