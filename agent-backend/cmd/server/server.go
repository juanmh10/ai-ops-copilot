package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ai-ops-copilot/agent-backend/internal/agent"
	"ai-ops-copilot/agent-backend/internal/config"
	"ai-ops-copilot/agent-backend/internal/memory"
	"ai-ops-copilot/agent-backend/internal/tools"
)

// Server encapsulates the HTTP handlers and dependencies.
type Server struct {
	cfg    *config.Config
	agent  agent.Agent
	store  memory.SessionStore
	logger *slog.Logger
}

// NewServer creates a new HTTP Server instance.
func NewServer(cfg *config.Config, ag agent.Agent, store memory.SessionStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:    cfg,
		agent:  ag,
		store:  store,
		logger: logger,
	}
}

// Routes configures the HTTP routes and middlewares.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/health", s.handleHealthz)
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{session_id}/messages", s.handleSessionMessages)
	mux.HandleFunc("POST /api/agent/digest", s.handleDigest)

	// Transparently reverse-proxy all Grafana UI and API requests to Grafana on localhost:3000
	if s.cfg.GrafanaURL != "" {
		target, err := url.Parse(s.cfg.GrafanaURL)
		if err == nil {
			proxy := httputil.NewSingleHostReverseProxy(target)
			originalDirector := proxy.Director
			proxy.Director = func(req *http.Request) {
				origHost := req.Host
				originalDirector(req)
				req.Host = origHost
				req.Header.Set("X-Forwarded-Host", origHost)
				if req.Header.Get("X-Forwarded-Proto") == "" {
					req.Header.Set("X-Forwarded-Proto", "https")
				}
			}
			mux.Handle("/", proxy)
		}
	}

	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, configured := range strings.Split(s.cfg.AllowedOrigins, ",") {
				if strings.TrimSpace(configured) == origin {
					allowed = true
				}
			}
			if s.cfg.LocalMockMode && (origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000") {
				allowed = true
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			if !allowed && r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Mock-User")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":            "ok",
		"service":           "ai-ops-copilot-backend",
		"model":             s.cfg.GeminiModel,
		"project_id":        s.cfg.GCPProjectID,
		"vertex_project_id": s.cfg.VertexProjectID,
		"location":          s.cfg.GCPLocation,
		"mock_mode":         s.cfg.LocalMockMode,
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "" && !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		s.writeError(w, http.StatusUnsupportedMediaType, errors.New("Content-Type must be application/json"))
		return
	}

	var req agent.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json request: %w", err))
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		s.writeError(w, http.StatusBadRequest, errors.New("message field is required"))
		return
	}
	userID, err := s.authenticatedUser(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, errors.New("Grafana authentication required"))
		return
	}
	req.UserID = userID
	if req.SessionID == "" {
		session, err := s.createSession(r.Context(), userID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		req.SessionID = session.ID
	} else {
		owned, err := s.ownsSession(r.Context(), userID, req.SessionID)
		if err != nil {
			s.logger.Error("Failed to check session ownership", "error", err)
			s.writeError(w, http.StatusServiceUnavailable, errors.New("Session history is temporarily unavailable"))
			return
		}
		if !owned {
			s.writeError(w, http.StatusNotFound, memory.ErrSessionNotFound)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	if req.Context != nil {
		queryContext := tools.DashboardQueryContext{
			DashboardUID:  req.Context.DashboardUID,
			ActiveFilters: make(map[string]string),
		}
		if req.Context.TimeRange != nil {
			queryContext.TimeFrom, _ = req.Context.TimeRange["from"].(string)
			queryContext.TimeTo, _ = req.Context.TimeRange["to"].(string)
		}
		for key, value := range req.Context.ActiveFilters {
			if text, ok := value.(string); ok {
				queryContext.ActiveFilters[key] = text
			}
		}
		if sessionCookie, err := r.Cookie("grafana_session"); err == nil {
			queryContext.Cookie = "grafana_session=" + sessionCookie.Value
		}
		queryContext.Authorization = r.Header.Get("Authorization")
		ctx = tools.WithDashboardQueryContext(ctx, queryContext)
	}
	resp, err := s.agent.Chat(ctx, req)
	if err != nil {
		s.logger.Error("Error processing chat request", "error", err)
		code := http.StatusBadGateway
		message := "The agent could not complete this request. Please retry."
		if !agent.LikelyEnglish(req.Message) {
			message = "O agente não conseguiu concluir a solicitação. Tente novamente."
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = http.StatusGatewayTimeout
			message = "The telemetry or AI request timed out. Try a narrower time range."
			if !agent.LikelyEnglish(req.Message) {
				message = "A consulta de telemetria ou IA expirou. Tente uma janela de tempo menor."
			}
		} else if strings.Contains(err.Error(), "persist") {
			code = http.StatusServiceUnavailable
			message = "The conversation could not be saved. Please retry."
			if !agent.LikelyEnglish(req.Message) {
				message = "Não foi possível salvar a conversa. Tente novamente."
			}
		}
		s.writeError(w, code, errors.New(message))
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) authenticatedUser(r *http.Request) (string, error) {
	if s.cfg.LocalMockMode {
		if user := strings.TrimSpace(r.Header.Get("X-Mock-User")); user != "" {
			return user, nil
		}
		return "", errors.New("mock user header missing")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.cfg.GrafanaURL, "/")+"/api/user", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Cookie", r.Header.Get("Cookie"))
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("Grafana session is not authenticated")
	}
	var profile struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return "", err
	}
	if profile.Login == "" {
		return "", errors.New("Grafana user login missing")
	}
	return profile.Login, nil
}

func validSessionID(id string) bool {
	if id == "" || len(id) > 80 {
		return false
	}
	for _, char := range id {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func (s *Server) ownsSession(ctx context.Context, userID, sessionID string) (bool, error) {
	if !validSessionID(sessionID) || s.store == nil {
		return false, nil
	}
	session, err := s.store.GetSession(ctx, sessionID)
	if errors.Is(err, memory.ErrSessionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return session.UserID == userID, nil
}

func (s *Server) createSession(ctx context.Context, userID string) (*memory.Session, error) {
	if s.store == nil {
		return nil, errors.New("session store unavailable")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("failed to create session ID: %w", err)
	}
	now := time.Now().UTC()
	session := &memory.Session{ID: hex.EncodeToString(random), UserID: userID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateSession(ctx, *session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	userID, err := s.authenticatedUser(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, errors.New("Grafana authentication required"))
		return
	}
	session, err := s.createSession(r.Context(), userID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	userID, err := s.authenticatedUser(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, errors.New("Grafana authentication required"))
		return
	}
	if s.store == nil {
		s.writeError(w, http.StatusServiceUnavailable, errors.New("session store unavailable"))
		return
	}
	sessions, err := s.store.ListSessions(r.Context(), userID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := s.authenticatedUser(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, errors.New("Grafana authentication required"))
		return
	}
	sessionID := r.PathValue("session_id")
	owned, err := s.ownsSession(r.Context(), userID, sessionID)
	if err != nil {
		s.logger.Error("Failed to check session ownership", "error", err)
		s.writeError(w, http.StatusServiceUnavailable, errors.New("Session history is temporarily unavailable"))
		return
	}
	if !owned {
		s.writeError(w, http.StatusNotFound, memory.ErrSessionNotFound)
		return
	}
	messages, err := s.store.GetHistory(r.Context(), sessionID, 200)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	lookback := 2

	// Check if JSON body is provided (e.g. from Cloud Scheduler)
	if r.Body != nil && r.ContentLength > 0 {
		var payload struct {
			Trigger       string `json:"trigger"`
			LookbackHours int    `json:"lookback_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil && payload.LookbackHours > 0 {
			lookback = payload.LookbackHours
		}
	}

	// Query parameter takes precedence if explicitly supplied
	if q := r.URL.Query().Get("lookback_hours"); q != "" {
		if val, err := strconv.Atoi(q); err == nil && val > 0 {
			lookback = val
		}
	}

	ctx := r.Context()
	digest, err := s.agent.GenerateDigest(ctx, lookback)
	if err != nil {
		s.logger.Error("Error generating telemetry digest", "error", err)
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("digest error: %w", err))
		return
	}

	s.writeJSON(w, http.StatusOK, digest)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}
