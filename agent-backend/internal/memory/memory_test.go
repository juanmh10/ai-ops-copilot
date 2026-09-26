package memory

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStore_ChatMessages(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	sessionID := "test-user-session-1"

	// Check empty history
	history, err := store.GetHistory(ctx, sessionID, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(history))
	}

	// Add 3 messages
	msg1 := ChatMessage{
		ID:        "msg-1",
		Role:      "user",
		Content:   "Olá, tudo bem?",
		Timestamp: time.Now().UTC().Add(-2 * time.Minute),
	}
	msg2 := ChatMessage{
		ID:        "msg-2",
		Role:      "model",
		Content:   "Hello! I am AI-Ops Copilot. How can I assist with your infrastructure?",
		Timestamp: time.Now().UTC().Add(-1 * time.Minute),
	}
	msg3 := ChatMessage{
		ID:        "msg-3",
		Role:      "user",
		Content:   "What is the health of enterprise-telemetry-prod?",
		Timestamp: time.Now().UTC(),
	}

	if err := store.SaveMessage(ctx, sessionID, msg1); err != nil {
		t.Fatalf("failed to save msg1: %v", err)
	}
	if err := store.SaveMessage(ctx, sessionID, msg2); err != nil {
		t.Fatalf("failed to save msg2: %v", err)
	}
	if err := store.SaveMessage(ctx, sessionID, msg3); err != nil {
		t.Fatalf("failed to save msg3: %v", err)
	}

	// Retrieve all
	history, err = store.GetHistory(ctx, sessionID, 10)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(history))
	}
	if history[0].ID != "msg-1" || history[2].ID != "msg-3" {
		t.Errorf("history is not in chronological order: %+v", history)
	}

	// Retrieve with limit 2 (should return last 2)
	historyLimited, err := store.GetHistory(ctx, sessionID, 2)
	if err != nil {
		t.Fatalf("failed to get limited history: %v", err)
	}
	if len(historyLimited) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(historyLimited))
	}
	if historyLimited[0].ID != "msg-2" || historyLimited[1].ID != "msg-3" {
		t.Errorf("expected msg-2 and msg-3, got %+v", historyLimited)
	}
}

func TestInMemoryStore_Snapshots(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	defer store.Close()

	// Empty snapshot fallback
	snap, err := store.GetLatestSnapshot(ctx, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil || snap.Summary == "" {
		t.Error("expected valid empty fallback snapshot")
	}

	// Save snapshot
	now := time.Now().UTC()
	err = store.SaveSnapshot(ctx, Snapshot{
		Timestamp:     now,
		LookbackHours: 2,
		Summary:       "All systems operational",
		Projects:      []any{"enterprise-core-prod", "enterprise-telemetry-prod"},
	})
	if err != nil {
		t.Fatalf("failed to save snapshot: %v", err)
	}

	snap, err = store.GetLatestSnapshot(ctx, 2)
	if err != nil {
		t.Fatalf("failed to get snapshot: %v", err)
	}
	if snap.Summary != "All systems operational" {
		t.Errorf("expected 'All systems operational', got %q", snap.Summary)
	}
}
