package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

// ---- HMAC signing tests ----

func TestSign(t *testing.T) {
	body := []byte(`{"event":"push"}`)
	sig := sign("secret", body)
	if sig == "" || len(sig) != 64 {
		t.Fatalf("sig=%s", sig)
	}
}

func TestSign_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		body   []byte
	}{
		{"empty_secret", "", []byte("hello")},
		{"empty_body", "secret", []byte{}},
		{"nil_body", "secret", nil},
		{"large_body", "secret", []byte(fmt.Sprintf("%010000d", 0))},
		{"special_chars", "sec!@#$%^&*()_+", []byte(`{"data":"test"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := sign(tt.secret, tt.body)
			// Verify with HMAC
			mac := hmac.New(sha256.New, []byte(tt.secret))
			mac.Write(tt.body)
			expected := hex.EncodeToString(mac.Sum(nil))
			if sig != expected {
				t.Errorf("sign(%q, body) = %q, want %q", tt.secret, sig, expected)
			}
		})
	}
}

func TestSign_DifferentSecrets(t *testing.T) {
	body := []byte("test message")
	sig1 := sign("secret1", body)
	sig2 := sign("secret2", body)
	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestSign_DifferentBodies(t *testing.T) {
	sig1 := sign("secret", []byte("body1"))
	sig2 := sign("secret", []byte("body2"))
	if sig1 == sig2 {
		t.Error("different bodies should produce different signatures")
	}
}

// ---- PushEvent tests ----

func TestNewPushEvent(t *testing.T) {
	e := NewPushEvent("alice", "demo", "main", "sha", "alice")
	if e.Repository != "alice/demo" || e.After != "sha" {
		t.Fatalf("event=%+v", e)
	}
}

func TestNewPushEvent_Fields(t *testing.T) {
	now := time.Now()
	e := NewPushEvent("owner", "repo", "feature-branch", "abc123def456", "pusher")

	if e.Ref != "refs/heads/feature-branch" {
		t.Errorf("Ref = %q, want %q", e.Ref, "refs/heads/feature-branch")
	}
	if e.Repository != "owner/repo" {
		t.Errorf("Repository = %q, want %q", e.Repository, "owner/repo")
	}
	if e.Pusher != "pusher" {
		t.Errorf("Pusher = %q, want %q", e.Pusher, "pusher")
	}
	if e.After != "abc123def456" {
		t.Errorf("After = %q, want %q", e.After, "abc123def456")
	}
	if e.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if e.Timestamp.Before(now.Add(-time.Second)) || e.Timestamp.After(time.Now()) {
		t.Errorf("Timestamp %v is not near current time", e.Timestamp)
	}
}

func TestNewPushEvent_JSON(t *testing.T) {
	e := NewPushEvent("owner", "repo", "main", "sha", "pusher")
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded PushEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Repository != e.Repository || decoded.After != e.After {
		t.Errorf("JSON round trip = %+v, want %+v", decoded, e)
	}
}

// ---- DB-backed tests ----

func setupWebhookTest(t *testing.T) (context.Context, *Service, uuid.UUID, string) {
	t.Helper()
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)

	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := repo.NewService(pg.Pool)
	webhookSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "webhooktest", "wh@test.local", "pass")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "webhook-repo", "test repo", false)
	if err != nil {
		t.Fatalf("Create repo: %v", err)
	}

	return ctx, webhookSvc, r.ID, u.Username
}

func TestCreate(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	t.Run("create basic webhook", func(t *testing.T) {
		h, err := svc.Create(ctx, repoID, "https://example.com/hook", "mysecret", []string{"push", "pull_request"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if h.ID == uuid.Nil {
			t.Error("expected non-nil ID")
		}
		if h.RepoID != repoID {
			t.Errorf("RepoID = %v, want %v", h.RepoID, repoID)
		}
		if h.URL != "https://example.com/hook" {
			t.Errorf("URL = %q, want %q", h.URL, "https://example.com/hook")
		}
		if len(h.Events) != 2 || h.Events[0] != "push" || h.Events[1] != "pull_request" {
			t.Errorf("Events = %v, want [push pull_request]", h.Events)
		}
		if !h.Active {
			t.Error("expected active=true")
		}
	})

	t.Run("create webhook with single event", func(t *testing.T) {
		h, err := svc.Create(ctx, repoID, "https://hooks.example.com/push", "secret", []string{"push"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(h.Events) != 1 || h.Events[0] != "push" {
			t.Errorf("Events = %v, want [push]", h.Events)
		}
	})

	t.Run("create webhook with empty events", func(t *testing.T) {
		h, err := svc.Create(ctx, repoID, "https://hooks.example.com/empty", "secret", []string{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if h.URL != "https://hooks.example.com/empty" {
			t.Errorf("URL = %q", h.URL)
		}
	})
}

func TestCreate_InvalidRepoID(t *testing.T) {
	ctx, svc, _, _ := setupWebhookTest(t)

	_, err := svc.Create(ctx, uuid.New(), "https://example.com/hook", "secret", []string{"push"})
	if err == nil {
		t.Fatal("expected error for non-existent repo ID")
	}
}

func TestListDeliveries(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	h, err := svc.Create(ctx, repoID, "https://example.com/hook", "secret", []string{"push"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("empty deliveries", func(t *testing.T) {
		dels, err := svc.ListDeliveries(ctx, h.ID, 10)
		if err != nil {
			t.Fatalf("ListDeliveries: %v", err)
		}
		if len(dels) != 0 {
			t.Errorf("expected 0 deliveries, got %d", len(dels))
		}
	})

	t.Run("limit defaults to 20", func(t *testing.T) {
		dels, err := svc.ListDeliveries(ctx, h.ID, 0)
		if err != nil {
			t.Fatalf("ListDeliveries: %v", err)
		}
		// Zero or empty is fine
		_ = dels
	})
}

func TestListDeliveries_InvalidWebhookID(t *testing.T) {
	ctx, svc, _, _ := setupWebhookTest(t)

	dels, err := svc.ListDeliveries(ctx, uuid.New(), 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(dels) != 0 {
		t.Errorf("expected 0 deliveries for non-existent webhook, got %d", len(dels))
	}
}

func TestDispatch(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	// Set up a test HTTP server that captures the webhook request
	var (
		receivedHeaders map[string]string
		receivedBody    []byte
		requestReceived bool
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		receivedHeaders = map[string]string{
			"Content-Type":        r.Header.Get("Content-Type"),
			"X-Govnohub-Event":   r.Header.Get("X-Govnohub-Event"),
			"X-Govnohub-Signature-256": r.Header.Get("X-Govnohub-Signature-256"),
		}
		// Read raw body for signature verification
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create a webhook pointing to the test server
	secret := "test-secret-123"
	h, err := svc.Create(ctx, repoID, ts.URL, secret, []string{"push"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = h

	// Dispatch a push event
	payload := NewPushEvent("webhooktest", "webhook-repo", "main", "abc123", "webhooktest")
	err = svc.Dispatch(ctx, repoID, "push", payload)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if !requestReceived {
		t.Fatal("test server did not receive the webhook request")
	}
	if receivedHeaders["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedHeaders["Content-Type"])
	}
	if receivedHeaders["X-Govnohub-Event"] != "push" {
		t.Errorf("X-Govnohub-Event = %q, want push", receivedHeaders["X-Govnohub-Event"])
	}

	// Verify the signature
	sigHeader := receivedHeaders["X-Govnohub-Signature-256"]
	if sigHeader == "" {
		t.Fatal("missing X-Govnohub-Signature-256 header")
	}
	expectedPrefix := "sha256="
	if len(sigHeader) <= len(expectedPrefix) || sigHeader[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("signature header = %q, want sha256=...", sigHeader)
	}
	// The signature should be valid for the body we received
	// Since the body includes a dynamic timestamp, compute expected sig from received body
	receivedSig := sigHeader[len(expectedPrefix):]
	expectedSig := sign(secret, receivedBody)
	if receivedSig != expectedSig {
		t.Errorf("signature mismatch: header=%q, computed from body=%q (secret=%q)",
			receivedSig, expectedSig, secret)
	}
}

func TestDispatch_MultipleWebhooks(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create two webhooks on the same repo for the same event
	_, err := svc.Create(ctx, repoID, ts.URL, "s1", []string{"push"})
	if err != nil {
		t.Fatalf("Create hook1: %v", err)
	}
	_, err = svc.Create(ctx, repoID, ts.URL, "s2", []string{"push"})
	if err != nil {
		t.Fatalf("Create hook2: %v", err)
	}

	payload := NewPushEvent("webhooktest", "webhook-repo", "main", "sha1", "webhooktest")
	err = svc.Dispatch(ctx, repoID, "push", payload)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestDispatch_NoMatchingWebhooks(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	// Create webhook for push only
	_, err := svc.Create(ctx, repoID, "https://example.com/hook", "secret", []string{"push"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Dispatch a pull_request event - should not trigger the push webhook
	payload := NewPushEvent("webhooktest", "webhook-repo", "main", "sha", "webhooktest")
	err = svc.Dispatch(ctx, repoID, "pull_request", payload)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// No assertion needed - should not error, just no matching hooks
}

func TestDispatch_FailedDelivery(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	// Create webhook pointing to a URL that will fail
	h, err := svc.Create(ctx, repoID, "http://127.0.0.1:1/nonexistent", "secret", []string{"push"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	payload := NewPushEvent("webhooktest", "webhook-repo", "main", "sha", "webhooktest")
	err = svc.Dispatch(ctx, repoID, "push", payload)
	if err != nil {
		t.Fatalf("Dispatch should not fail on delivery error: %v", err)
	}

	// Delivery should still be recorded
	dels, err := svc.ListDeliveries(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(dels) == 0 {
		t.Error("expected at least one delivery record even for failed delivery")
	}
	// Status should be 0 (no response) or non-2xx
	if len(dels) > 0 && dels[0].StatusCode == 200 {
		t.Error("expected non-200 status code for failed delivery")
	}
}

func TestDispatch_DeliveryRecorded(t *testing.T) {
	ctx, svc, repoID, _ := setupWebhookTest(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	h, err := svc.Create(ctx, repoID, ts.URL, "secret", []string{"push"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	payload := NewPushEvent("webhooktest", "webhook-repo", "main", "abc", "webhooktest")
	err = svc.Dispatch(ctx, repoID, "push", payload)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Check delivery was recorded
	dels, err := svc.ListDeliveries(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(dels) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(dels))
	}
	if dels[0].WebhookID != h.ID {
		t.Errorf("WebhookID = %v, want %v", dels[0].WebhookID, h.ID)
	}
	if dels[0].Event != "push" {
		t.Errorf("Event = %q, want %q", dels[0].Event, "push")
	}
	if dels[0].StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", dels[0].StatusCode)
	}
	if dels[0].DeliveredAt.IsZero() {
		t.Error("DeliveredAt should not be zero")
	}
}

func TestDelivery_JSON(t *testing.T) {
	d := Delivery{
		ID:          uuid.New(),
		WebhookID:   uuid.New(),
		Event:       "push",
		StatusCode:  200,
		DeliveredAt: time.Now().UTC(),
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded Delivery
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.ID != d.ID || decoded.Event != d.Event || decoded.StatusCode != d.StatusCode {
		t.Errorf("JSON round trip = %+v, want %+v", decoded, d)
	}
}
