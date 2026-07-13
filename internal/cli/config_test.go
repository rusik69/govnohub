package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &Config{
		APIURL:   "http://api.test",
		GitURL:   "http://git.test",
		Token:    "ghp_test",
		Username: "alice",
	}
	if err := SaveConfig(cfg, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != cfg.Token || loaded.Username != cfg.Username {
		t.Fatalf("got %+v", loaded)
	}
}

func TestParseOwnerRepo(t *testing.T) {
	owner, repo, err := ParseOwnerRepo("alice/demo.git")
	if err != nil || owner != "alice" || repo != "demo" {
		t.Fatalf("got %s/%s err=%v", owner, repo, err)
	}
	if _, _, err := ParseOwnerRepo("bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"jwt123","user":{"username":"bob"}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	token, user, err := client.Login("bob", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if token != "jwt123" {
		t.Fatalf("token=%s", token)
	}
	if user["username"] != "bob" {
		t.Fatalf("user=%v", user)
	}
}

func TestCloneURL(t *testing.T) {
	cfg := &Config{GitURL: "https://git.example.com", Token: "ghp_abc"}
	url := CloneURL(cfg, "o", "r")
	if url != "https://x-access-token:ghp_abc@git.example.com/o/r.git" {
		t.Fatalf("got %s", url)
	}
}

func TestRequireToken(t *testing.T) {
	cfg := &Config{}
	err := cfg.RequireToken()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not authenticated: run `govnohub auth login` or set --token" {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
	cfg.Token = "x"
	if err := cfg.RequireToken(); err != nil {
		t.Fatal(err)
	}
}

func TestMergeFlags_Precedence(t *testing.T) {
	t.Run("non-empty flags override config", func(t *testing.T) {
		cfg := &Config{APIURL: "http://default", GitURL: "http://default.git", Token: "old-token"}
		cfg.MergeFlags("http://override", "http://override.git", "new-token")
		if cfg.APIURL != "http://override" {
			t.Fatalf("APIURL = %q, want %q", cfg.APIURL, "http://override")
		}
		if cfg.GitURL != "http://override.git" {
			t.Fatalf("GitURL = %q, want %q", cfg.GitURL, "http://override.git")
		}
		if cfg.Token != "new-token" {
			t.Fatalf("Token = %q, want %q", cfg.Token, "new-token")
		}
	})

	t.Run("empty flags do not override config", func(t *testing.T) {
		cfg := &Config{APIURL: "http://persist", GitURL: "http://persist.git", Token: "persist-token"}
		cfg.MergeFlags("", "", "")
		if cfg.APIURL != "http://persist" {
			t.Fatalf("APIURL = %q, want %q", cfg.APIURL, "http://persist")
		}
		if cfg.GitURL != "http://persist.git" {
			t.Fatalf("GitURL = %q, want %q", cfg.GitURL, "http://persist.git")
		}
		if cfg.Token != "persist-token" {
			t.Fatalf("Token = %q, want %q", cfg.Token, "persist-token")
		}
	})

	t.Run("partial override leaves other fields intact", func(t *testing.T) {
		cfg := &Config{APIURL: "http://api", GitURL: "http://git", Token: "tok"}
		cfg.MergeFlags("http://new-api", "", "")
		if cfg.APIURL != "http://new-api" {
			t.Fatalf("APIURL = %q, want %q", cfg.APIURL, "http://new-api")
		}
		if cfg.GitURL != "http://git" {
			t.Fatalf("GitURL = %q, want %q", cfg.GitURL, "http://git")
		}
		if cfg.Token != "tok" {
			t.Fatalf("Token = %q, want %q", cfg.Token, "tok")
		}
	})

	t.Run("defaults in config survive empty flags", func(t *testing.T) {
		cfg := &Config{APIURL: defaultAPIURL, GitURL: defaultGitURL}
		cfg.MergeFlags("", "", "")
		if cfg.APIURL != defaultAPIURL {
			t.Fatalf("APIURL = %q, want %q", cfg.APIURL, defaultAPIURL)
		}
		if cfg.GitURL != defaultGitURL {
			t.Fatalf("GitURL = %q, want %q", cfg.GitURL, defaultGitURL)
		}
	})
}

func TestConfigPathCreatesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func TestAppRunHelp(t *testing.T) {
	app := &App{}
	if code := app.Run([]string{"help"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}
