//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWebhookCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "webhookuser")
	owner := "webhookuser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "webhooktest", "description": "repo for webhook tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a webhook
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks", token, map[string]any{
		"url":    "https://example.com/webhook",
		"secret": "mysecret123",
		"events": []string{"push", "pull_request"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create webhook status=%d out=%v", resp.StatusCode, out)
	}
	checkWebhookFields(t, out, "https://example.com/webhook", []string{"push", "pull_request"}, true)
	hookID, _ := out["id"].(string)
	if hookID == "" {
		t.Fatal("missing id in webhook response")
	}

	// Create a second webhook with different events
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks", token, map[string]any{
		"url":    "https://hooks.example.com/issues",
		"secret": "anothersecret",
		"events": []string{"issues"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create second webhook status=%d out=%v", resp.StatusCode, out)
	}
	checkWebhookFields(t, out, "https://hooks.example.com/issues", []string{"issues"}, true)

	// Create a webhook with empty secret and single event
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks", token, map[string]any{
		"url":    "https://hooks.example.com/push",
		"secret": "",
		"events": []string{"push"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create webhook with empty secret status=%d out=%v", resp.StatusCode, out)
	}
	if url, _ := out["url"].(string); url != "https://hooks.example.com/push" {
		t.Errorf("url=%q want https://hooks.example.com/push", url)
	}

	// List webhooks
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	respList, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer respList.Body.Close()
	if respList.StatusCode != http.StatusOK {
		t.Fatalf("list webhooks status=%d", respList.StatusCode)
	}
	var hooks []map[string]any
	if err := json.NewDecoder(respList.Body).Decode(&hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 3 {
		t.Fatalf("expected 3 webhooks, got %d", len(hooks))
	}
	urls := make(map[string]bool)
	for _, h := range hooks {
		u, _ := h["url"].(string)
		urls[u] = true
	}
	if !urls["https://example.com/webhook"] {
		t.Error("missing example.com/webhook in list")
	}
	if !urls["https://hooks.example.com/issues"] {
		t.Error("missing hooks.example.com/issues in list")
	}
	if !urls["https://hooks.example.com/push"] {
		t.Error("missing hooks.example.com/push in list")
	}

	// List deliveries for the first webhook (should be empty initially)
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks/"+hookID+"/deliveries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	respDel, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer respDel.Body.Close()
	if respDel.StatusCode != http.StatusOK {
		t.Fatalf("list deliveries status=%d", respDel.StatusCode)
	}
	var deliveries []map[string]any
	if err := json.NewDecoder(respDel.Body).Decode(&deliveries); err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(deliveries))
	}

	// List webhooks for non-existent repo should fail
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/nonexistent/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("list webhooks for nonexistent repo: expected 404, got %d", resp.StatusCode)
	}

	// Create webhook with empty events should succeed
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/webhooktest/webhooks", token, map[string]any{
		"url":    "https://hooks.example.com/empty",
		"secret": "s",
		"events": []string{},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create webhook with empty events status=%d out=%v", resp.StatusCode, out)
	}
}

func TestWebhookAccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	aliceToken := testutil.RegisterAndLogin(t, env.URL, "alice_wh")
	owner := "alice_wh"

	// Create a private repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", aliceToken, map[string]any{
		"name": "privrepo", "description": "private", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to read webhooks on Alice's private repo
	bobToken := testutil.RegisterAndLogin(t, env.URL, "bob_wh")
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/privrepo/webhooks", bobToken, nil)
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Errorf("bob list webhooks on private repo: expected 404/403, got %d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to create webhooks on Alice's private repo
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/privrepo/webhooks", bobToken, map[string]any{
		"url":    "https://evil.com/hook",
		"secret": "steal",
		"events": []string{"push"},
	})
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bob create webhook on private repo: expected 404/403/401, got %d out=%v", resp.StatusCode, out)
	}

	// Alice should be able to create webhooks on her own private repo
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/privrepo/webhooks", aliceToken, map[string]any{
		"url":    "https://hooks.example.com/private",
		"secret": "mysecret",
		"events": []string{"push"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice create webhook on private repo: expected 200, got %d out=%v", resp.StatusCode, out)
	}
}

func checkWebhookFields(t *testing.T, out map[string]any, wantURL string, wantEvents []string, wantActive bool) {
	t.Helper()
	if url, _ := out["url"].(string); url != wantURL {
		t.Errorf("url=%q want %q", url, wantURL)
	}
	if active, _ := out["active"].(bool); active != wantActive {
		t.Errorf("active=%v want %v", active, wantActive)
	}
	if _, ok := out["id"]; !ok {
		t.Error("missing id")
	}
	// Check events (might be []any when decoded from JSON)
	if eventsRaw, ok := out["events"]; ok {
		eventsArr, ok := eventsRaw.([]any)
		if !ok {
			t.Errorf("events is not an array, got %T", eventsRaw)
		}
		if len(eventsArr) != len(wantEvents) {
			t.Errorf("events length=%d want %d", len(eventsArr), len(wantEvents))
		}
	} else {
		t.Error("missing events in webhook response")
	}
}
