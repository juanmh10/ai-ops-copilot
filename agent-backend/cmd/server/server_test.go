package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-ops-copilot/agent-backend/internal/agent"
	"ai-ops-copilot/agent-backend/internal/config"
	"ai-ops-copilot/agent-backend/internal/memory"
	"ai-ops-copilot/agent-backend/internal/tools"
)

type mockAgent struct {
	chatFunc   func(ctx context.Context, req agent.ChatRequest) (*agent.ChatResponse, error)
	digestFunc func(ctx context.Context, lookbackHours int) (*tools.DigestSnapshot, error)
}

type failingSessionStore struct{ memory.SessionStore }

func (f failingSessionStore) GetSession(context.Context, string) (*memory.Session, error) {
	return nil, errors.New("firestore unavailable")
}

func (m *mockAgent) Chat(ctx context.Context, req agent.ChatRequest) (*agent.ChatResponse, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, req)
	}
	return &agent.ChatResponse{
		Response:  "Resposta mock do agente",
		SessionID: req.SessionID,
	}, nil
}

func (m *mockAgent) GenerateDigest(ctx context.Context, lookbackHours int) (*tools.DigestSnapshot, error) {
	if m.digestFunc != nil {
		return m.digestFunc(ctx, lookbackHours)
	}
	return &tools.DigestSnapshot{
		Timestamp:     "2026-09-19T20:00:00Z",
		LookbackHours: lookbackHours,
		Summary:       "Todos os serviços saudáveis",
	}, nil
}

func TestServer_Healthz(t *testing.T) {
	cfg := &config.Config{
		GeminiModel:     "gemini-3.8-flash",
		GCPProjectID:    "enterprise-core-prod",
		VertexProjectID: "enterprise-ai-gateway",
		GCPLocation:     "us-central1",
		LocalMockMode:   true,
	}
	srv := NewServer(cfg, &mockAgent{}, memory.NewInMemoryStore(), nil)

	for _, path := range []string{"/healthz", "/api/healthz", "/api/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		srv.Routes().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("path %s: expected status 200, got %d", path, w.Code)
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("path %s: failed to decode response: %v", path, err)
		}

		if resp["status"] != "ok" {
			t.Errorf("path %s: expected status 'ok', got %v", path, resp["status"])
		}
	}
}

func TestServer_CORS(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: "http://localhost:3000"}
	srv := NewServer(cfg, &mockAgent{}, memory.NewInMemoryStore(), nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 No Content for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("expected CORS allow configured origin, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestServer_Chat(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: "*", LocalMockMode: true}
	srv := NewServer(cfg, &mockAgent{}, memory.NewInMemoryStore(), nil)

	// 1. Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBufferString("not a json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}

	// 2. Missing message
	emptyReq, _ := json.Marshal(agent.ChatRequest{Message: "  "})
	req = httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBuffer(emptyReq))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty message, got %d", w.Code)
	}

	// 3. Valid chat request
	validReq, _ := json.Marshal(agent.ChatRequest{
		UserID:    "ops-lead@example.com",
		SessionID: "",
		Message:   "How is the infrastructure?",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBuffer(validReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mock-User", "test-user")
	w = httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid chat, got %d", w.Code)
	}

	var chatResp agent.ChatResponse
	if err := json.NewDecoder(w.Body).Decode(&chatResp); err != nil {
		t.Fatalf("failed to decode chat response: %v", err)
	}
	if chatResp.Response != "Resposta mock do agente" {
		t.Errorf("expected mock response, got %s", chatResp.Response)
	}
}

func TestServer_Digest(t *testing.T) {
	cfg := &config.Config{}
	srv := NewServer(cfg, &mockAgent{}, memory.NewInMemoryStore(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/digest?lookback_hours=3", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for digest, got %d", w.Code)
	}

	var snap tools.DigestSnapshot
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatalf("failed to decode digest snapshot: %v", err)
	}
	if snap.LookbackHours != 3 {
		t.Errorf("expected lookback_hours 3, got %d", snap.LookbackHours)
	}

	// Test 2: JSON body payload from Cloud Scheduler
	bodyPayload := []byte(`{"trigger":"cloud-scheduler","lookback_hours":4}`)
	req = httptest.NewRequest(http.MethodPost, "/api/agent/digest", bytes.NewBuffer(bodyPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for digest with JSON body, got %d", w.Code)
	}

	var snapFromJSON tools.DigestSnapshot
	if err := json.NewDecoder(w.Body).Decode(&snapFromJSON); err != nil {
		t.Fatalf("failed to decode digest snapshot: %v", err)
	}
	if snapFromJSON.LookbackHours != 4 {
		t.Errorf("expected lookback_hours 4 from JSON body, got %d", snapFromJSON.LookbackHours)
	}
}

func TestSessionEndpointsEnforceGrafanaOwner(t *testing.T) {
	store := memory.NewInMemoryStore()
	srv := NewServer(&config.Config{LocalMockMode: true}, &mockAgent{}, store, nil)
	handler := srv.Routes()

	create := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	create.Header.Set("X-Mock-User", "operator-a")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", w.Code, w.Body.String())
	}
	var session memory.Session
	if err := json.NewDecoder(w.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.UserID != "operator-a" || session.ID == "" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if err := store.SaveMessage(context.Background(), session.ID, memory.ChatMessage{ID: "msg-1", Role: "user", Content: "private question", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		user string
		want int
	}{
		{"operator-a", http.StatusOK},
		{"operator-b", http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/messages", nil)
		request.Header.Set("X-Mock-User", tc.user)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != tc.want {
			t.Fatalf("user %s: got %d, want %d", tc.user, w.Code, tc.want)
		}
	}

	forged, _ := json.Marshal(agent.ChatRequest{UserID: "operator-a", SessionID: session.ID, Message: "read prior context"})
	request := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(forged))
	request.Header.Set("X-Mock-User", "operator-b")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusNotFound {
		t.Fatalf("forged user_id accessed another session: %d", w.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	request.Header.Set("X-Mock-User", "operator-b")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	var sessions []memory.Session
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil || len(sessions) != 0 {
		t.Fatalf("other user's list leaked sessions: %+v, %v", sessions, err)
	}
}

func TestProductionSessionUsesGrafanaCookie(t *testing.T) {
	grafana := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user" || r.Header.Get("Cookie") != "grafana_session=valid" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"operator-a"}`))
	}))
	defer grafana.Close()

	srv := NewServer(&config.Config{GrafanaURL: grafana.URL}, &mockAgent{}, memory.NewInMemoryStore(), nil)
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	request.Header.Set("Cookie", "grafana_session=valid")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, request)
	if w.Code != http.StatusCreated {
		t.Fatalf("authenticated session creation returned %d", w.Code)
	}
	var session memory.Session
	if err := json.NewDecoder(w.Body).Decode(&session); err != nil || session.UserID != "operator-a" {
		t.Fatalf("session owner was not Grafana login: %+v, %v", session, err)
	}
}

func TestChatTimeoutExplainsRetry(t *testing.T) {
	agentStub := &mockAgent{chatFunc: func(ctx context.Context, req agent.ChatRequest) (*agent.ChatResponse, error) {
		return nil, context.DeadlineExceeded
	}}
	srv := NewServer(&config.Config{LocalMockMode: true}, agentStub, memory.NewInMemoryStore(), nil)
	request := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBufferString(`{"message":"tokens","locale":"pt"}`))
	request.Header.Set("X-Mock-User", "operator-a")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, request)
	if w.Code != http.StatusGatewayTimeout || !bytes.Contains(w.Body.Bytes(), []byte("narrower time range")) {
		t.Fatalf("expected orienting timeout response, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionHistoryStoreFailureIsUnavailable(t *testing.T) {
	store := failingSessionStore{memory.NewInMemoryStore()}
	srv := NewServer(&config.Config{LocalMockMode: true}, &mockAgent{}, store, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/sessions/abc/messages", nil)
	request.Header.Set("X-Mock-User", "operator-a")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, request)
	if w.Code != http.StatusServiceUnavailable || !bytes.Contains(w.Body.Bytes(), []byte("temporarily unavailable")) {
		t.Fatalf("expected unavailable history response, got %d: %s", w.Code, w.Body.String())
	}
}
