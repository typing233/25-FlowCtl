package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowctl/flowctl/internal/api"
	"github.com/flowctl/flowctl/internal/auth"
	"github.com/flowctl/flowctl/internal/config"
	"github.com/flowctl/flowctl/internal/engine"
	"github.com/flowctl/flowctl/internal/runner"
	"github.com/flowctl/flowctl/internal/secrets"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		logger.Error("invalid database URL", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = int32(cfg.Database.MaxConns)
	poolCfg.MinConns = int32(cfg.Database.MinConns)
	poolCfg.MaxConnLifetime = cfg.Database.MaxConnLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to database")

	authService := auth.NewService(pool, cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry, cfg.Auth.RefreshExpiry)
	rbacEngine := auth.NewRBACEngine(pool)

	var oidcProvider *auth.OIDCProvider
	if cfg.Auth.OIDCIssuerURL != "" {
		oidcProvider, err = auth.NewOIDCProvider(ctx, auth.OIDCConfig{
			IssuerURL:    cfg.Auth.OIDCIssuerURL,
			ClientID:     cfg.Auth.OIDCClientID,
			ClientSecret: cfg.Auth.OIDCClientSecret,
			RedirectURL:  fmt.Sprintf("http://localhost:%d/auth/callback", cfg.Server.Port),
		}, pool)
		if err != nil {
			logger.Warn("OIDC provider not configured", "error", err)
		}
	}

	var samlProvider *auth.SAMLProvider
	if cfg.Auth.SAMLCertPath != "" && cfg.Auth.SAMLKeyPath != "" {
		samlProvider, err = auth.NewSAMLProvider(auth.SAMLConfig{
			CertPath:    cfg.Auth.SAMLCertPath,
			KeyPath:     cfg.Auth.SAMLKeyPath,
			EntityID:    cfg.Auth.SAMLEntityID,
			MetadataURL: cfg.Auth.SAMLMetadataURL,
			ACSURL:      fmt.Sprintf("http://localhost:%d/auth/saml/acs", cfg.Server.Port),
		}, pool)
		if err != nil {
			logger.Warn("SAML provider not configured", "error", err)
		}
	}

	vault, err := secrets.NewVault(pool, cfg.Encryption.MasterKey)
	if err != nil {
		logger.Error("failed to initialize secrets vault", "error", err)
		os.Exit(1)
	}

	deps := &api.Dependencies{
		Pool:         pool,
		AuthService:  authService,
		RBACEngine:   rbacEngine,
		OIDCProvider: oidcProvider,
		SAMLProvider: samlProvider,
		Vault:        vault,
	}

	router := api.NewRouter(deps)

	// Start scheduler if enabled
	if cfg.Scheduler.Enabled {
		runnerFactory := runner.DefaultFactory()
		executor := engine.NewExecutor(pool, runnerFactory, logger)
		scheduler := engine.NewScheduler(pool, cfg.Scheduler, executor, logger)

		if err := scheduler.Start(ctx); err != nil {
			logger.Error("failed to start scheduler", "error", err)
			os.Exit(1)
		}
		defer scheduler.Stop()
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	logger.Info("server stopped")
}
