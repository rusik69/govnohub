//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestReleaseAssetUploadDownload(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "rel" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/releases", token, map[string]any{
		"tag_name": "v1.0.0", "name": "First", "body": "notes",
	})
	requireStatus(t, resp, http.StatusOK, "create release")

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "hello.txt")
	part.Write([]byte("hello asset"))
	w.Close()
	resp, _ = testutil.DoBody(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/releases/v1.0.0/assets", token, w.FormDataContentType(), &body)
	requireStatus(t, resp, http.StatusOK, "upload asset")
	resp.Body.Close()

	resp, assets := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/releases/v1.0.0/assets", token, nil)
	requireStatus(t, resp, http.StatusOK, "list assets")
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}

	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/releases/v1.0.0/assets/hello.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	dl, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	requireStatus(t, dl, http.StatusOK, "download asset")
	b, _ := io.ReadAll(dl.Body)
	if string(b) != "hello asset" {
		t.Fatalf("asset body=%q", b)
	}
}

func TestPackagePublishDownload(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pkg" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoBody(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/packages?name=demo&version=1.0.0&type=generic", token, "application/octet-stream", strings.NewReader("package-data"))
	requireStatus(t, resp, http.StatusOK, "publish package")

	resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
	requireStatus(t, resp, http.StatusOK, "list packages")
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}

	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/demo/1.0.0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	dl, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	requireStatus(t, dl, http.StatusOK, "download package")
	b, _ := io.ReadAll(dl.Body)
	if string(b) != "package-data" {
		t.Fatalf("package body=%q", b)
	}
}
