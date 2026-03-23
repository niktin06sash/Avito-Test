package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"

	"test-backend/internal/config"
	"test-backend/internal/database"
	"test-backend/internal/handler"
	"test-backend/internal/logger"
	"test-backend/internal/repository"
	"test-backend/internal/server"
	"test-backend/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logrusLogger := logger.InitMain()
	logrusLogger.Debug("loading config...")
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	handlerLog := logger.Handler(logrusLogger)
	serviceLog := logger.Service(logrusLogger)

	logrusLogger.Debug("connecting to database...")
	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logrusLogger.Debug("init database error:", err)
		return
	}
	defer db.Close()

	logrusLogger.Debug("creating repository...")
	repo := repository.New(db)

	logrusLogger.Debug("creating service...")
	svc := service.New(repo, serviceLog)

	logrusLogger.Debug("creating handler...")
	h := handler.New(svc, cfg.JWTSecret, handlerLog)
	srv := server.New(cfg.HTTPAddr, h.Routes())

	errCh := make(chan error, 1)
	go func() {
		logrusLogger.Info("starting server on", cfg.HTTPAddr)
		if err := srv.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logrusLogger.Info("shutting down server...")
		if err := srv.Shutdown(context.Background()); err != nil {
			logrusLogger.Error("server shutdown error:", err)
		}
	case err := <-errCh:
		logrusLogger.Error("server error:", err)
	}

	logrusLogger.Info("server stopped")
}
