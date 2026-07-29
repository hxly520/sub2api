package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/config"
	"github.com/hxly520/sub2api/points-system/internal/httpapi"
	"github.com/hxly520/sub2api/points-system/internal/migrate"
	"github.com/hxly520/sub2api/points-system/internal/store"
	"github.com/hxly520/sub2api/points-system/internal/sub2client"
	"github.com/hxly520/sub2api/points-system/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.NewPointsPool(ctx, cfg.DatabaseURL, cfg.DatabaseSchema, cfg.DatabaseMaxConns)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := migrate.Run(ctx, db); err != nil {
		logger.Error("run migrations", "error", err)
		os.Exit(1)
	}
	pointsStore := store.New(db, cfg.Timezone)
	usageDB, err := store.NewReadOnlyUsagePool(ctx, cfg.UsageDatabaseURL)
	if err != nil {
		logger.Error("open Sub2API usage database", "error", err)
		os.Exit(1)
	}
	defer usageDB.Close()
	usageSource := store.NewPostgreSQLUsageSource(usageDB)
	if err := usageSource.Validate(ctx); err != nil {
		logger.Error("validate Sub2API usage source", "error", err)
		os.Exit(1)
	}
	pointsStore.SetUsageSource(usageSource)
	bridge, err := sub2client.New(cfg.Sub2URL, cfg.Sub2Key.ID, cfg.Sub2Key.Secret,
		&http.Client{Timeout: cfg.HTTPTimeout})
	if err != nil {
		logger.Error("configure Sub2API client", "error", err)
		os.Exit(1)
	}
	api, err := httpapi.New(cfg, pointsStore, logger)
	if err != nil {
		logger.Error("configure HTTP API", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Addr: cfg.ListenAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 32 << 10,
	}

	errorsCh := make(chan error, 4)
	go func() {
		logger.Info("points HTTP server started", "address", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	grants := &worker.BalanceGrantWorker{
		Store: pointsStore, Client: bridge, Interval: cfg.WorkerInterval, Lease: 30 * time.Second, Logger: logger,
	}
	go func() { errorsCh <- grants.Run(ctx) }()
	snapshots := &worker.SnapshotScheduler{Store: pointsStore, Location: cfg.Timezone,
		Logger: logger, ReconcileDays: cfg.UsageReconcileDays}
	go func() { errorsCh <- snapshots.Run(ctx) }()
	go func() { errorsCh <- cleanupLoop(ctx, pointsStore, logger) }()

	select {
	case <-ctx.Done():
	case err := <-errorsCh:
		if err != nil {
			logger.Error("service component stopped", "error", err)
		}
		stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown failed", "error", err)
	}
}

func cleanupLoop(ctx context.Context, pointsStore *store.Store, logger *slog.Logger) error {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := pointsStore.CleanupSecurityState(ctx, now.UTC()); err != nil {
				logger.Error("security state cleanup failed", "error", err)
			}
		}
	}
}
