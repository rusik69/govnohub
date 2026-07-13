//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestReleaseCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "releaseuser")
	owner := "releaseuser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "testrepo", "description": "repo for release tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a release
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"tag_name":    "v1.0.0",
		"name":        "First Release",
		"body":        "Initial release notes",
		"draft":       false,
		"prerelease":  false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create release status=%d out=%v", resp.StatusCode, out)
	}
	checkReleaseFields(t, out, "v1.0.0", "First Release", "Initial release notes", false, false)
	releaseID, _ := out["id"].(string)
	if releaseID == "" {
		t.Fatal("missing id in release response")
	}

	// Create a draft release
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"tag_name":   "v2.0.0-draft",
		"name":       "Draft Release",
		"body":       "",
		"draft":      true,
		"prerelease": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create draft release status=%d out=%v", resp.StatusCode, out)
	}
	checkReleaseFields(t, out, "v2.0.0-draft", "Draft Release", "", true, false)

	// Create a prerelease
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"tag_name":   "v3.0.0-rc1",
		"name":       "Release Candidate",
		"body":       "Testing release",
		"draft":      false,
		"prerelease": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create prerelease status=%d out=%v", resp.StatusCode, out)
	}
	checkReleaseFields(t, out, "v3.0.0-rc1", "Release Candidate", "Testing release", false, true)

	// Create a release with empty name and body
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"tag_name":   "v4.0.0",
		"name":       "",
		"body":       "",
		"draft":      false,
		"prerelease": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create release with empty fields status=%d out=%v", resp.StatusCode, out)
	}
	if name, _ := out["name"].(string); name != "" {
		t.Errorf("name=%q want empty", name)
	}
	if body, _ := out["body"].(string); body != "" {
		t.Errorf("body=%q want empty", body)
	}

	// Duplicate tag name should fail
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"tag_name": "v1.0.0",
		"name":     "Duplicate",
	})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("duplicate tag: expected 500, got %d out=%v", resp.StatusCode, out)
	}

	// Missing tag_name should fail
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", token, map[string]any{
		"name": "No Tag",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing tag: expected 400, got %d out=%v", resp.StatusCode, out)
	}

	// List releases
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list releases status=%d", resp.StatusCode)
	}
	var releases []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		t.Fatal(err)
	}
	if len(releases) != 4 {
		t.Fatalf("expected 4 releases, got %d", len(releases))
	}
	// Verify newest first order
	if len(releases) >= 2 {
		created0, _ := releases[0]["created_at"].(string)
		created1, _ := releases[1]["created_at"].(string)
		if created0 < created1 {
			t.Error("expected releases ordered by created_at DESC (newest first)")
		}
	}

	// List releases for different repo should be empty
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/other/releases", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("list releases for nonexistent repo: expected 404, got %d", resp.StatusCode)
	}

	// Get release by tag via assets endpoint to verify release lookup
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases/v1.0.0/assets", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("get release by tag (via assets): expected 200, got %d", resp.StatusCode)
	}

	// Try getting assets for nonexistent tag
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/releases/nonexistent/assets", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("nonexistent tag assets: expected 404, got %d out=%v", resp.StatusCode, out)
	}
}

func TestReleaseAssetLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "assetuser2")
	owner := "assetuser2"

	// Create repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "assetrepo", "description": "test", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a release
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases", token, map[string]any{
		"tag_name": "v1.0.0", "name": "Asset Test", "body": "test",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create release status=%d out=%v", resp.StatusCode, out)
	}

	// Upload an asset (multipart form)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "hello.txt")
	part.Write([]byte("hello asset"))
	w.Close()

	resp, out = testutil.DoBody(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets", token, w.FormDataContentType(), &body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload asset status=%d out=%v", resp.StatusCode, out)
	}
	if n, _ := out["name"].(string); n != "hello.txt" {
		t.Errorf("asset name=%q want hello.txt", n)
	}
	if sz, _ := out["size_bytes"].(float64); sz != 11 {
		t.Errorf("asset size=%v want 11", sz)
	}
	if _, ok := out["id"]; !ok {
		t.Error("missing asset id")
	}
	if _, ok := out["release_id"]; !ok {
		t.Error("missing asset release_id")
	}
	resp.Body.Close()

	// Upload a second asset with different content type
	body.Reset()
	w = multipart.NewWriter(&body)
	part, _ = w.CreateFormFile("file", "data.json")
	part.Write([]byte(`{"key":"value"}`))
	w.Close()

	resp, out = testutil.DoBody(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets", token, w.FormDataContentType(), &body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload second asset status=%d out=%v", resp.StatusCode, out)
	}
	if n, _ := out["name"].(string); n != "data.json" {
		t.Errorf("asset name=%q want data.json", n)
	}
	resp.Body.Close()

	// Upload asset without file should fail
	resp, out = testutil.DoBody(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets", token, "application/json", bytes.NewReader([]byte(`{}`)))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("upload without file: expected 400, got %d", resp.StatusCode)
	}

	// Upload asset to nonexistent release tag should fail
	body.Reset()
	w = multipart.NewWriter(&body)
	part, _ = w.CreateFormFile("file", "orphan.txt")
	part.Write([]byte("data"))
	w.Close()
	resp, out = testutil.DoBody(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/nonexistent/assets", token, w.FormDataContentType(), &body)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("upload to nonexistent tag: expected 404, got %d", resp.StatusCode)
	}

	// List assets
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list assets status=%d", resp.StatusCode)
	}
	var assets []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&assets); err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	names := make(map[string]bool)
	for _, a := range assets {
		n, _ := a["name"].(string)
		names[n] = true
	}
	if !names["hello.txt"] {
		t.Error("missing hello.txt in assets list")
	}
	if !names["data.json"] {
		t.Error("missing data.json in assets list")
	}

	// Download asset by name
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets/hello.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	dl, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("download status=%d", dl.StatusCode)
	}
	b, _ := io.ReadAll(dl.Body)
	if string(b) != "hello asset" {
		t.Fatalf("asset body=%q want %q", string(b), "hello asset")
	}
	if dl.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type=%q", dl.Header.Get("Content-Type"))
	}

	// Download nonexistent asset should fail
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/assetrepo/releases/v1.0.0/assets/nonexistent.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	dl, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dl.Body.Close()
	if dl.StatusCode != http.StatusNotFound {
		t.Errorf("download nonexistent asset: expected 404, got %d", dl.StatusCode)
	}
}

func TestReleaseAccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	aliceToken := testutil.RegisterAndLogin(t, env.URL, "alice_rel")
	owner := "alice_rel"

	// Create a private repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", aliceToken, map[string]any{
		"name": "privrepo", "description": "private", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a release in private repo
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/privrepo/releases", aliceToken, map[string]any{
		"tag_name": "v1.0.0", "name": "Private Release", "body": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create release in private repo: %d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to access the private repo releases
	bobToken := testutil.RegisterAndLogin(t, env.URL, "bob_rel")
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/privrepo/releases", bobToken, nil)
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Errorf("bob access to private repo releases: expected 404/403, got %d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to create a release in Alice's repo (needs write access)
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/privrepo/releases", bobToken, map[string]any{
		"tag_name": "v99.0.0", "name": "Bob's Release",
	})
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bob create release in private repo: expected 404/403/401, got %d out=%v", resp.StatusCode, out)
	}
}

func checkReleaseFields(t *testing.T, out map[string]any, wantTag, wantName, wantBody string, wantDraft, wantPrerelease bool) {
	t.Helper()
	if tag, _ := out["tag_name"].(string); tag != wantTag {
		t.Errorf("tag_name=%q want %q", tag, wantTag)
	}
	if name, _ := out["name"].(string); name != wantName {
		t.Errorf("name=%q want %q", name, wantName)
	}
	if body, _ := out["body"].(string); body != wantBody {
		t.Errorf("body=%q want %q", body, wantBody)
	}
	if draft, _ := out["draft"].(bool); draft != wantDraft {
		t.Errorf("draft=%v want %v", draft, wantDraft)
	}
	if prerelease, _ := out["prerelease"].(bool); prerelease != wantPrerelease {
		t.Errorf("prerelease=%v want %v", prerelease, wantPrerelease)
	}
	if _, ok := out["id"]; !ok {
		t.Error("missing id")
	}
	if _, ok := out["created_at"]; !ok {
		t.Error("missing created_at")
	}
}
