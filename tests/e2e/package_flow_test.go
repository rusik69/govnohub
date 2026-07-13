//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestPackageFlow_PublishListDownload(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pfl" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "pkg-repo")
	repo := owner + "/pkg-repo"

	// ---- Publish a package ----
	t.Run("publish package", func(t *testing.T) {
		resp, out := testutil.DoBody(t, http.MethodPost,
			base+"/api/v1/repos/"+repo+"/packages?name=mylib&version=1.0.0&type=generic",
			token, "application/octet-stream", strings.NewReader("artifact-data"))
		requireStatus(t, resp, http.StatusOK, "publish package")
		if out["name"] != "mylib" {
			t.Fatalf("expected name 'mylib', got %q", out["name"])
		}
		if out["version"] != "1.0.0" {
			t.Fatalf("expected version '1.0.0', got %q", out["version"])
		}
		if out["package_type"] != "generic" {
			t.Fatalf("expected package_type 'generic', got %q", out["package_type"])
		}
		if id, ok := out["id"].(string); !ok || id == "" {
			t.Fatal("missing package id")
		}
	})

	// ---- List packages ----
	t.Run("list packages", func(t *testing.T) {
		resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
		requireStatus(t, resp, http.StatusOK, "list packages")
		if len(pkgs) != 1 {
			t.Fatalf("expected 1 package, got %d", len(pkgs))
		}
		if pkgs[0]["name"] != "mylib" {
			t.Fatalf("expected name 'mylib', got %q", pkgs[0]["name"])
		}
		if pkgs[0]["version"] != "1.0.0" {
			t.Fatalf("expected version '1.0.0', got %q", pkgs[0]["version"])
		}
	})

	// ---- Download package ----
	t.Run("download package", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/mylib/1.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download package")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "artifact-data" {
			t.Fatalf("expected body 'artifact-data', got %q", body)
		}
	})
}

func TestPackageFlow_MultipleVersions(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pmv" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "mv-repo")
	repo := owner + "/mv-repo"

	// Publish v1.0.0
	resp, _ := testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=mylib&version=1.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("v1-content"))
	requireStatus(t, resp, http.StatusOK, "publish v1.0.0")

	// Publish v2.0.0
	resp, _ = testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=mylib&version=2.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("v2-content"))
	requireStatus(t, resp, http.StatusOK, "publish v2.0.0")

	// List should show both
	t.Run("list shows both versions", func(t *testing.T) {
		resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
		requireStatus(t, resp, http.StatusOK, "list packages")
		if len(pkgs) != 2 {
			t.Fatalf("expected 2 packages, got %d", len(pkgs))
		}
	})

	// Download v1.0.0
	t.Run("download v1.0.0", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/mylib/1.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download v1.0.0")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "v1-content" {
			t.Fatalf("expected 'v1-content', got %q", body)
		}
	})

	// Download v2.0.0
	t.Run("download v2.0.0", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/mylib/2.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download v2.0.0")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "v2-content" {
			t.Fatalf("expected 'v2-content', got %q", body)
		}
	})
}

func TestPackageFlow_UpsertSameVersion(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pus" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "up-repo")
	repo := owner + "/up-repo"

	// Publish first version
	resp, out := testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=mylib&version=1.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("original"))
	requireStatus(t, resp, http.StatusOK, "first publish")
	firstID := out["id"].(string)

	// Re-publish same name+version
	resp, out = testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=mylib&version=1.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("updated"))
	requireStatus(t, resp, http.StatusOK, "re-publish")

	// Should return same ID (upsert)
	if out["id"].(string) != firstID {
		t.Fatalf("expected same package ID on upsert, got %s != %s", out["id"], firstID)
	}

	// Download should return updated content
	t.Run("download returns updated content", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/mylib/1.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download after upsert")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "updated" {
			t.Fatalf("expected 'updated', got %q", body)
		}
	})
}

func TestPackageFlow_NotFound(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pnf" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "nf-repo")
	repo := owner + "/nf-repo"

	t.Run("download nonexistent package", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/nope/0.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for nonexistent package, got %d", resp.StatusCode)
		}
	})
}

func TestPackageFlow_MultiplePackages(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pmp" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "mp-repo")
	repo := owner + "/mp-repo"

	// Publish two different packages
	resp, _ := testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=lib-a&version=1.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("lib-a-data"))
	requireStatus(t, resp, http.StatusOK, "publish lib-a")

	resp, _ = testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=lib-b&version=2.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("lib-b-data"))
	requireStatus(t, resp, http.StatusOK, "publish lib-b")

	// List should show both
	t.Run("list shows both packages", func(t *testing.T) {
		resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
		requireStatus(t, resp, http.StatusOK, "list packages")
		if len(pkgs) != 2 {
			t.Fatalf("expected 2 packages, got %d", len(pkgs))
		}
	})

	// Download each
	t.Run("download lib-a", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/lib-a/1.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download lib-a")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "lib-a-data" {
			t.Fatalf("expected 'lib-a-data', got %q", body)
		}
	})

	t.Run("download lib-b", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/packages/lib-b/2.0.0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		requireStatus(t, resp, http.StatusOK, "download lib-b")
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "lib-b-data" {
			t.Fatalf("expected 'lib-b-data', got %q", body)
		}
	})
}

func TestPackageFlow_DifferentTypes(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pdt" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "dt-repo")
	repo := owner + "/dt-repo"

	// Publish a generic package
	resp, _ := testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=tool&version=1.0.0&type=generic",
		token, "application/octet-stream", strings.NewReader("generic-data"))
	requireStatus(t, resp, http.StatusOK, "publish generic")

	// Publish a container type package (default type)
	resp, out := testutil.DoBody(t, http.MethodPost,
		base+"/api/v1/repos/"+repo+"/packages?name=my-container&version=latest",
		token, "application/octet-stream", strings.NewReader("container-data"))
	requireStatus(t, resp, http.StatusOK, "publish container (default)")
	if out["package_type"] != "container" {
		t.Fatalf("expected package_type 'container' (default), got %q", out["package_type"])
	}

	// List packages
	resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
	requireStatus(t, resp, http.StatusOK, "list packages")
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
}

func TestPackageFlow_EmptyList(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pel" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "el-repo")
	repo := owner + "/el-repo"

	t.Run("list packages on repo with none", func(t *testing.T) {
		resp, pkgs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/packages", token, nil)
		requireStatus(t, resp, http.StatusOK, "list empty packages")
		if len(pkgs) != 0 {
			t.Fatalf("expected 0 packages on empty repo, got %d", len(pkgs))
		}
	})
}
