package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/apns"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/command"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/config"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/demo"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/httpapi"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/live"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
)

func main() {
	config.LoadDotEnv(".env", "../.env")
	cfg := config.FromEnv()
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("database", "err", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	if err := demo.Seed(db); err != nil {
		slog.Error("demo seed", "err", err.Error())
		os.Exit(1)
	}
	svc := &live.Service{DB: db}
	srv := httpapi.New(cfg, db, svc)
	if cfg.PrivateKeyFile != "" {
		signer, err := command.Load(cfg.PrivateKeyFile)
		if err != nil {
			slog.Error("virtual key was not loaded", "err", err.Error())
		} else {
			srv.Signer = signer
			slog.Info("virtual key loaded")
		}
	}
	if push, err := apns.New(cfg.APNSKeyFile, cfg.APNSKeyID, cfg.APNSTeamID, cfg.APNSBundleID, cfg.APNSProduction); err == nil {
		srv.APNS = push
		slog.Info("apns configured")
	}
	srv.UseFleet()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go srv.PrimeAwake(ctx)
	defer stop()
	if cfg.DemoAutoplay {
		go demo.Run(ctx, svc)
	}
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	slog.Info("honk listening", "addr", cfg.Addr, "domain", cfg.Domain, "teslaConfigured", cfg.TeslaConfigured(), "demoAutoplay", cfg.DemoAutoplay)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err.Error())
		os.Exit(1)
	}
}
