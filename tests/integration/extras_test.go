//go:build integration

package integration

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestReleaseAssetAndWebhookDeliveries(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "assetuser")
	owner := "assetuser"
	repo := "assetrepo"
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": repo, "description": "test", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/"+repo+"/releases", token, map[string]any{
		"tag_name": "v1.0.0", "name": "First", "body": "notes",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create release status=%d", resp.StatusCode)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "hello.txt")
	part.Write([]byte("hello asset"))
	w.Close()
	resp, _ = testutil.DoBody(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/"+repo+"/releases/v1.0.0/assets", token, w.FormDataContentType(), &body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/"+repo+"/releases/v1.0.0/assets/hello.txt", nil)
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
		t.Fatalf("asset body=%q", b)
	}

	resp, hook := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/"+repo+"/webhooks", token, map[string]any{
		"url": "http://127.0.0.1:1/hook", "secret": "s", "events": []string{"push"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status=%d", resp.StatusCode)
	}
	hookID, _ := hook["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/"+repo+"/webhooks/"+hookID+"/deliveries", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deliveries status=%d", resp.StatusCode)
	}
}
