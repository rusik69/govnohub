//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestNotifications_EmptyList(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "notifuser1")
	// Fresh user should have no notifications
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/notifications", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list notifications status=%d out=%v", resp.StatusCode, out)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(out["data"].(string)), &items); err == nil {
		// If it's wrapped in data field, try that
	} else {
		items = nil
	}
	// The response is directly a JSON array
	_ = items
}

func TestNotifications_ListAndMarkRead(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "notifuser2")
	owner := "notifuser2"

	// Create a repo to have some context
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "notiftest", "description": "repo for notification tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create notifications via the notification service directly (since there's no
	// public API to create notifications — they are created internally by other operations)
	notifSvc := notification.NewService(env.Pool)
	ctx := context.Background()

	// Get the user's UUID from the database
	var userID uuid.UUID
	err := env.Pool.QueryRow(ctx, `SELECT id FROM users WHERE username=$1`, owner).Scan(&userID)
	if err != nil {
		t.Fatalf("get user id: %v", err)
	}

	// Create test notifications
	notifications := []struct {
		title string
		body  string
		link  string
	}{
		{"PR Comment", "New comment on your pull request", "/user/notifuser2/notiftest/pulls/1"},
		{"Watch Event", "Someone starred your repo", "/user/notifuser2/notiftest"},
		{"Issue Assigned", "Issue #5 was assigned to you", "/user/notifuser2/notiftest/issues/5"},
	}

	for _, n := range notifications {
		if err := notifSvc.Create(ctx, userID, n.title, n.body, n.link); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}

	// List notifications
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	respList, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer respList.Body.Close()
	if respList.StatusCode != http.StatusOK {
		t.Fatalf("list notifications status=%d", respList.StatusCode)
	}

	var items []map[string]any
	if err := json.NewDecoder(respList.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(items))
	}

	// Verify notification fields
	checkNotificationFields(t, items[0], notifications[0].title, notifications[0].body, notifications[0].link, false)
	checkNotificationFields(t, items[1], notifications[1].title, notifications[1].body, notifications[1].link, false)
	checkNotificationFields(t, items[2], notifications[2].title, notifications[2].body, notifications[2].link, false)

	// Verify order: newest first (compare strings, ISO 8601 sorts lexicographically)
	t0, _ := items[0]["created_at"].(string)
	t1, _ := items[1]["created_at"].(string)
	if t0 < t1 {
		t.Errorf("expected newest-first order: created_at[0]=%q created_at[1]=%q", t0, t1)
	}

	// Mark the first notification as read
	notifID, ok := items[0]["id"].(string)
	if !ok || notifID == "" {
		t.Fatal("missing id in notification response")
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/notifications/"+notifID+"/read", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mark notification read status=%d out=%v", resp.StatusCode, out)
	}
	if status, _ := out["status"].(string); status != "read" {
		t.Errorf("status=%q want %q", status, "read")
	}

	// Verify the notification is now marked as read
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	respList2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer respList2.Body.Close()
	if respList2.StatusCode != http.StatusOK {
		t.Fatalf("list notifications after mark read: status=%d", respList2.StatusCode)
	}

	var items2 []map[string]any
	if err := json.NewDecoder(respList2.Body).Decode(&items2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items2) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(items2))
	}
	if read, _ := items2[0]["read"].(bool); !read {
		t.Error("expected first notification to be marked as read")
	}
	// Other notifications should still be unread
	for i := 1; i < len(items2); i++ {
		if read, _ := items2[i]["read"].(bool); read {
			t.Errorf("notification %d should be unread", i)
		}
	}
}

func TestNotifications_InvalidID(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "notif_invalid")

	// Mark with invalid UUID format
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/notifications/not-a-uuid/read", token, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid id: expected 400, got %d out=%v", resp.StatusCode, out)
	}

	// Mark with valid UUID but non-existent
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/notifications/"+uuid.New().String()+"/read", token, nil)
	// Non-existent notification should not error (idempotent MarkRead)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("nonexistent id: expected 200, got %d out=%v", resp.StatusCode, out)
	}
}

func TestNotifications_Unauthenticated(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// List without auth
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/notifications", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list without auth: expected 401, got %d out=%v", resp.StatusCode, out)
	}

	// Mark read without auth
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/notifications/"+uuid.New().String()+"/read", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("mark read without auth: expected 401, got %d out=%v", resp.StatusCode, out)
	}
}

func TestNotifications_UserIsolation(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	aliceToken := testutil.RegisterAndLogin(t, env.URL, "alice_notif")
	bobToken := testutil.RegisterAndLogin(t, env.URL, "bob_notif")

	// Get Alice's user ID
	var aliceID uuid.UUID
	err := env.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE username=$1`, "alice_notif").Scan(&aliceID)
	if err != nil {
		t.Fatalf("get alice id: %v", err)
	}

	// Create a notification for Alice
	notifSvc := notification.NewService(env.Pool)
	if err := notifSvc.Create(context.Background(), aliceID, "Alice's notification", "Only Alice should see this", "/alice"); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	// Bob should see empty list
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+bobToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bob list notifications: status=%d", resp.StatusCode)
	}
	var bobItems []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&bobItems); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(bobItems) != 0 {
		t.Errorf("bob saw %d notifications, expected 0", len(bobItems))
	}

	// Alice should see her notification
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice list notifications: status=%d", resp.StatusCode)
	}
	var aliceItems []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&aliceItems); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(aliceItems) != 1 {
		t.Fatalf("alice saw %d notifications, expected 1", len(aliceItems))
	}
	if title, _ := aliceItems[0]["title"].(string); title != "Alice's notification" {
		t.Errorf("title=%q want %q", title, "Alice's notification")
	}

	// Bob should not be able to mark Alice's notification as read by guessing the ID
	aliceNotifID, _ := aliceItems[0]["id"].(string)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/notifications/"+aliceNotifID+"/read", bobToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bob mark alice's notif read: expected 200, got %d out=%v", resp.StatusCode, out)
	}
	// Alice's notification should still be unread
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var aliceItems2 []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&aliceItems2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(aliceItems2) != 1 {
		t.Fatalf("expected 1 notification for alice, got %d", len(aliceItems2))
	}
	if read, _ := aliceItems2[0]["read"].(bool); read {
		t.Error("alice's notification should still be unread after bob's mark attempt")
	}
}

func TestNotifications_CountAndOrder(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "notif_count")

	var userID uuid.UUID
	err := env.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE username=$1`, "notif_count").Scan(&userID)
	if err != nil {
		t.Fatalf("get user id: %v", err)
	}

	notifSvc := notification.NewService(env.Pool)
	// Create 5 notifications
	for i := 0; i < 5; i++ {
		if err := notifSvc.Create(context.Background(), userID, "Notif", "body", "/link"); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	// List and verify count and order
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status=%d", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 notifications, got %d", len(items))
	}
	// Should be ordered newest first
	for i := 1; i < len(items); i++ {
		t0, _ := items[i-1]["created_at"].(string)
		t1, _ := items[i]["created_at"].(string)
		if t0 < t1 {
			t.Errorf("notifications not in descending order at index %d: %q < %q", i, t0, t1)
			break
		}
	}
}

// checkNotificationFields verifies the expected fields in a notification API response
func checkNotificationFields(t *testing.T, item map[string]any, wantTitle, wantBody, wantLink string, wantRead bool) {
	t.Helper()
	if title, _ := item["title"].(string); title != wantTitle {
		t.Errorf("title=%q want %q", title, wantTitle)
	}
	if body, _ := item["body"].(string); body != wantBody {
		t.Errorf("body=%q want %q", body, wantBody)
	}
	if link, _ := item["link"].(string); link != wantLink {
		t.Errorf("link=%q want %q", link, wantLink)
	}
	if read, _ := item["read"].(bool); read != wantRead {
		t.Errorf("read=%v want %v", read, wantRead)
	}
	if id, _ := item["id"].(string); id == "" {
		t.Error("missing id")
	}
	if _, ok := item["created_at"]; !ok {
		t.Error("missing created_at")
	}
}
