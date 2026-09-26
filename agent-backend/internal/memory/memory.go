package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrSessionNotFound = errors.New("session not found")

// Session is the owner-scoped metadata for a persisted conversation.
type Session struct {
	ID        string    `json:"id" firestore:"id"`
	UserID    string    `json:"user_id" firestore:"user_id"`
	Title     string    `json:"title" firestore:"title"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt time.Time `json:"updated_at" firestore:"updated_at"`
}

// ChatMessage represents a single message turn in the conversation.
type ChatMessage struct {
	ID        string         `json:"id" firestore:"id"`
	Role      string         `json:"role" firestore:"role"` // "user" or "model"
	Content   string         `json:"content" firestore:"content"`
	Timestamp time.Time      `json:"timestamp" firestore:"timestamp"`
	Context   map[string]any `json:"context,omitempty" firestore:"context,omitempty"`
}

// Snapshot represents an aggregated infrastructure status snapshot.
type Snapshot struct {
	Timestamp     time.Time      `json:"timestamp" firestore:"timestamp"`
	LookbackHours int            `json:"lookback_hours" firestore:"lookback_hours"`
	Summary       string         `json:"summary" firestore:"summary"`
	Projects      []any          `json:"projects" firestore:"projects"`
	Metadata      map[string]any `json:"metadata,omitempty" firestore:"metadata,omitempty"`
}

// SessionStore defines the contract for persisting messages and snapshots.
type SessionStore interface {
	CreateSession(ctx context.Context, session Session) error
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	ListSessions(ctx context.Context, userID string) ([]Session, error)
	SaveMessage(ctx context.Context, sessionID string, msg ChatMessage) error
	GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	SaveSnapshot(ctx context.Context, snapshot Snapshot) error
	GetLatestSnapshot(ctx context.Context, lookbackHours int) (*Snapshot, error)
	Close() error
}

// InMemoryStore provides an in-memory, thread-safe implementation of SessionStore.
type InMemoryStore struct {
	mu        sync.RWMutex
	sessions  map[string][]ChatMessage
	metadata  map[string]Session
	snapshots []Snapshot
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions:  make(map[string][]ChatMessage),
		metadata:  make(map[string]Session),
		snapshots: make([]Snapshot, 0),
	}
}

func (m *InMemoryStore) CreateSession(ctx context.Context, session Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.metadata[session.ID]; exists {
		return fmt.Errorf("session already exists")
	}
	m.metadata[session.ID] = session
	return nil
}

func (m *InMemoryStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.metadata[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return &session, nil
}

func (m *InMemoryStore) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Session, 0)
	for _, session := range m.metadata {
		if session.UserID == userID {
			result = append(result, session)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (m *InMemoryStore) SaveMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	m.sessions[sessionID] = append(m.sessions[sessionID], msg)
	if session, ok := m.metadata[sessionID]; ok {
		session.UpdatedAt = msg.Timestamp
		if session.Title == "" && msg.Role == "user" {
			session.Title = truncateTitle(msg.Content)
		}
		m.metadata[sessionID] = session
	}
	return nil
}

func (m *InMemoryStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs, ok := m.sessions[sessionID]
	if !ok || len(msgs) == 0 {
		return []ChatMessage{}, nil
	}

	if limit <= 0 || limit >= len(msgs) {
		copied := make([]ChatMessage, len(msgs))
		copy(copied, msgs)
		return copied, nil
	}

	// Return last 'limit' messages
	start := len(msgs) - limit
	copied := make([]ChatMessage, limit)
	copy(copied, msgs[start:])
	return copied, nil
}

func (m *InMemoryStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now().UTC()
	}
	m.snapshots = append(m.snapshots, snapshot)
	return nil
}

func (m *InMemoryStore) GetLatestSnapshot(ctx context.Context, lookbackHours int) (*Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.snapshots) == 0 {
		return &Snapshot{
			Timestamp:     time.Now().UTC(),
			LookbackHours: lookbackHours,
			Summary:       "No snapshots recorded in in-memory storage.",
			Projects:      []any{},
		}, nil
	}

	latest := m.snapshots[len(m.snapshots)-1]
	return &latest, nil
}

func (m *InMemoryStore) Close() error {
	return nil
}

// FirestoreStore provides Firestore Native persistence.
type FirestoreStore struct {
	client *firestore.Client
}

// NewFirestoreStore creates a SessionStore backed by Cloud Firestore.
func NewFirestoreStore(ctx context.Context, projectID string) (*FirestoreStore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firestore client: %w", err)
	}
	return &FirestoreStore{client: client}, nil
}

// Client returns the underlying firestore.Client.
func (f *FirestoreStore) Client() *firestore.Client {
	return f.client
}

func (f *FirestoreStore) CreateSession(ctx context.Context, session Session) error {
	_, err := f.client.Collection("agent_sessions").Doc(session.ID).Create(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (f *FirestoreStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	doc, err := f.client.Collection("agent_sessions").Doc(sessionID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}
	var session Session
	if err := doc.DataTo(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session: %w", err)
	}
	return &session, nil
}

func (f *FirestoreStore) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	iter := f.client.Collection("agent_sessions").Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()
	result := make([]Session, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list sessions: %w", err)
		}
		var session Session
		if err := doc.DataTo(&session); err != nil {
			return nil, fmt.Errorf("failed to decode session: %w", err)
		}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func truncateTitle(content string) string {
	runes := []rune(content)
	if len(runes) > 64 {
		return string(runes[:64]) + "…"
	}
	return content
}

func (f *FirestoreStore) SaveMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}

	parent := f.client.Collection("agent_sessions").Doc(sessionID)
	child := parent.Collection("messages").NewDoc()
	batch := f.client.Batch()
	updates := []firestore.Update{{Path: "updated_at", Value: msg.Timestamp}}
	if msg.Role == "user" {
		session, err := f.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if session.Title == "" {
			updates = append(updates, firestore.Update{Path: "title", Value: truncateTitle(msg.Content)})
		}
	}
	batch.Update(parent, updates)
	batch.Create(child, msg)
	_, err := batch.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to save chat message in firestore: %w", err)
	}
	return nil
}

func (f *FirestoreStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 20
	}

	iter := f.client.Collection("agent_sessions").
		Doc(sessionID).
		Collection("messages").
		OrderBy("timestamp", firestore.Desc).
		Limit(limit).
		Documents(ctx)

	var msgs []ChatMessage
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to query chat history: %w", err)
		}

		var m ChatMessage
		if err := doc.DataTo(&m); err != nil {
			return nil, fmt.Errorf("failed to decode message: %w", err)
		}
		msgs = append(msgs, m)
	}

	// Reverse back to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (f *FirestoreStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now().UTC()
	}

	docName := snapshot.Timestamp.Format(time.RFC3339)
	_, err := f.client.Collection("infra_snapshots").Doc(docName).Set(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("failed to save snapshot in firestore: %w", err)
	}
	return nil
}

func (f *FirestoreStore) GetLatestSnapshot(ctx context.Context, lookbackHours int) (*Snapshot, error) {
	iter := f.client.Collection("infra_snapshots").
		OrderBy("timestamp", firestore.Desc).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return &Snapshot{
			Timestamp:     time.Now().UTC(),
			LookbackHours: lookbackHours,
			Summary:       "Nenhum snapshot encontrado no Firestore.",
			Projects:      []any{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve latest snapshot: %w", err)
	}

	var snap Snapshot
	if err := doc.DataTo(&snap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snap, nil
}

func (f *FirestoreStore) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
