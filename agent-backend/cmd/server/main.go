package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-ops-copilot/agent-backend/internal/agent"
	"ai-ops-copilot/agent-backend/internal/config"
	"ai-ops-copilot/agent-backend/internal/memory"
	"ai-ops-copilot/agent-backend/internal/tools"

	"cloud.google.com/go/firestore"
	"google.golang.org/adk/v2/model"
	adkgemini "google.golang.org/adk/v2/model/gemini"
	"google.golang.org/genai"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()
	logger.Info("Starting AI-Ops Copilot Agent Backend",
		"port", cfg.Port,
		"project_id", cfg.GCPProjectID,
		"vertex_project_id", cfg.VertexProjectID,
		"location", cfg.GCPLocation,
		"model", cfg.GeminiModel,
		"mock_mode", cfg.LocalMockMode,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize Memory Store
	var memStore memory.SessionStore
	var fsClient *firestore.Client

	if cfg.LocalMockMode && cfg.FirestoreEmulatorHost == "" {
		logger.Info("Using in-memory session store (Local Mock Mode)")
		memStore = memory.NewInMemoryStore()
	} else {
		fsStore, err := memory.NewFirestoreStore(ctx, cfg.GCPProjectID)
		if err != nil {
			logger.Error("Failed to initialize Firestore session store", "error", err)
			os.Exit(1)
		} else {
			logger.Info("Connected to Firestore", "project", cfg.GCPProjectID)
			memStore = fsStore
			fsClient = fsStore.Client()
		}
	}
	defer memStore.Close()

	// 2. Initialize Read-Only Tools Providers
	var grafanaClient tools.GrafanaClient
	var monitoringClient tools.MonitoringClient
	var loggingClient tools.LoggingClient
	var digestClient tools.DigestClient
	var billingClient tools.BillingClient

	if cfg.LocalMockMode {
		logger.Info("Initializing mock providers for local development")
		grafanaClient = tools.NewMockGrafanaClient()
		monitoringClient = tools.NewMockMonitoringClient()
		loggingClient = tools.NewMockLoggingClient()
		digestClient = tools.NewMockDigestClient()
	} else {
		grafanaClient = tools.NewRealGrafanaClient(cfg.GrafanaURL)

		mc, err := tools.NewRealMonitoringClient(ctx)
		if err != nil {
			logger.Error("Failed to initialize Cloud Monitoring", "error", err)
			monitoringClient = unavailableMonitoringClient{cause: err}
		} else {
			defer mc.Close()
			monitoringClient = mc
		}

		lc, err := tools.NewRealLoggingClient(ctx, cfg.GCPProjectID)
		if err != nil {
			logger.Error("Failed to initialize Cloud Logging", "error", err)
			loggingClient = unavailableLoggingClient{cause: err}
		} else {
			defer lc.Close()
			loggingClient = lc
		}

		digestClient = tools.NewRealDigestClient(fsClient)
	}

	if !cfg.LocalMockMode {
		bc, err := tools.NewRealBillingClient(ctx, cfg.BillingProjectID, cfg.BillingDataset)
		if err != nil {
			logger.Error("Failed to initialize Cloud Billing", "error", err)
			billingClient = unavailableBillingClient{cause: err}
		} else {
			defer bc.Close()
			billingClient = bc
		}
	}

	toolExecutor := tools.NewExecutor(grafanaClient, monitoringClient, loggingClient, digestClient, logger, billingClient)

	// 3. Initialize the ADK model on Vertex AI using ADC.
	llm, isMock := initADKModel(ctx, cfg, logger)
	if isMock && !cfg.LocalMockMode {
		logger.Error("Vertex AI unavailable; refusing to start with simulated responses")
		os.Exit(1)
	}

	// 4. Initialize Orchestrator
	orchestrator := agent.NewOrchestrator(
		llm,
		cfg.GeminiModel,
		toolExecutor,
		memStore,
		cfg.LocalMockMode,
		logger,
	)

	// 5. Initialize HTTP Server
	srv := NewServer(cfg, orchestrator, memStore, logger)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      srv.Routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Channel for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("HTTP server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for termination signal
	sig := <-stop
	logger.Info("Shutdown signal received, shutting down gracefully...", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server forced to shutdown", "error", err)
	}

	logger.Info("Server exited cleanly")
}

func buildGenAIClientConfig(cfg *config.Config) *genai.ClientConfig {
	return &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  cfg.VertexProjectID,
		Location: cfg.GCPLocation,
	}
}

func initADKModel(ctx context.Context, cfg *config.Config, logger *slog.Logger) (model.LLM, bool) {
	if cfg.LocalMockMode {
		return nil, true
	}
	if logger == nil {
		logger = slog.Default()
	}

	client, err := adkgemini.NewModel(ctx, cfg.GeminiModel, buildGenAIClientConfig(cfg))
	if err != nil {
		logger.Error("Failed to initialize GenAI client",
			"backend", "Vertex AI via ADC",
			"error", err,
		)
		return nil, true
	}

	logger.Info("Successfully initialized ADK model",
		"backend", "Vertex AI via ADC",
		"project", cfg.VertexProjectID,
		"location", cfg.GCPLocation,
	)

	return client, false
}

type unavailableBillingClient struct{ cause error }

func (u unavailableBillingClient) QueryCosts(context.Context, tools.BillingQuery) (*tools.BillingResult, error) {
	return nil, fmt.Errorf("Cloud Billing unavailable: %w", u.cause)
}

type unavailableMonitoringClient struct{ cause error }

func (u unavailableMonitoringClient) QueryMetrics(_ context.Context, _ tools.MonitoringQuery) (*tools.MonitoringResult, error) {
	return nil, fmt.Errorf("Cloud Monitoring unavailable: %w", u.cause)
}

type unavailableLoggingClient struct{ cause error }

func (u unavailableLoggingClient) QueryLogs(_ context.Context, _, _, _ string, _ int) ([]tools.LogEntry, error) {
	return nil, fmt.Errorf("Cloud Logging unavailable: %w", u.cause)
}
