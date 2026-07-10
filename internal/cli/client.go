package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    stringsTrimSlash(baseURL),
		token:      token,
		httpClient: http.DefaultClient,
	}
}

func (c *Client) Login(username, password string) (string, map[string]any, error) {
	var out map[string]any
	err := c.post("/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	}, &out)
	if err != nil {
		return "", nil, err
	}
	token, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	return token, user, nil
}

func (c *Client) CreatePAT(name string, scopes []string) (string, error) {
	var out map[string]string
	err := c.post("/api/v1/user/tokens", c.token, map[string]any{
		"name": name, "scopes": scopes,
	}, &out)
	if err != nil {
		return "", err
	}
	return out["token"], nil
}

func (c *Client) ListRepos() ([]map[string]any, error) {
	var out []map[string]any
	err := c.get("/api/v1/user/repos", c.token, &out)
	return out, err
}

func (c *Client) CreateRepo(name, description string, private bool) (map[string]any, error) {
	user, err := c.CurrentUser()
	if err != nil {
		return nil, err
	}
	username, _ := user["username"].(string)
	var out map[string]any
	err = c.post(fmt.Sprintf("/api/v1/users/%s/repos", url.PathEscape(username)), c.token, map[string]any{
		"name": name, "description": description, "private": private,
	}, &out)
	return out, err
}

func (c *Client) CurrentUser() (map[string]any, error) {
	var out map[string]any
	err := c.get("/api/v1/user", c.token, &out)
	return out, err
}

func (c *Client) ListIssues(owner, repo string) ([]map[string]any, error) {
	var out []map[string]any
	err := c.get(fmt.Sprintf("/api/v1/repos/%s/%s/issues", owner, repo), c.token, &out)
	return out, err
}

func (c *Client) CreateIssue(owner, repo, title, body string) (map[string]any, error) {
	var out map[string]any
	err := c.post(fmt.Sprintf("/api/v1/repos/%s/%s/issues", owner, repo), c.token, map[string]string{
		"title": title, "body": body,
	}, &out)
	return out, err
}

func (c *Client) ListPRs(owner, repo string) ([]map[string]any, error) {
	var out []map[string]any
	err := c.get(fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), c.token, &out)
	return out, err
}

func (c *Client) CreatePR(owner, repo, title, body, head, base string) (map[string]any, error) {
	var out map[string]any
	err := c.post(fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), c.token, map[string]string{
		"title": title, "body": body, "head": head, "base": base,
	}, &out)
	return out, err
}

func (c *Client) MergePR(owner, repo string, number int, squash bool) (map[string]any, error) {
	var out map[string]any
	err := c.post(fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", owner, repo, number), c.token, map[string]any{
		"squash": squash,
	}, &out)
	return out, err
}

func (c *Client) CreateRelease(owner, repo, tag, name, body string) (map[string]any, error) {
	var out map[string]any
	err := c.post(fmt.Sprintf("/api/v1/repos/%s/%s/releases", owner, repo), c.token, map[string]any{
		"tag_name": tag, "name": name, "body": body,
	}, &out)
	return out, err
}

func (c *Client) UploadReleaseAsset(owner, repo, tag, filePath string) (map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	w.Close()
	req, err := http.NewRequest(http.MethodPost,
		c.baseURL+fmt.Sprintf("/api/v1/repos/%s/%s/releases/%s/assets", owner, repo, url.PathEscape(tag)),
		&buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s: %s", resp.Status, string(data))
	}
	var out map[string]any
	return out, json.Unmarshal(data, &out)
}

func (c *Client) ListRuns(owner, repo string) ([]map[string]any, error) {
	var out []map[string]any
	err := c.get(fmt.Sprintf("/api/v1/repos/%s/%s/actions/runs", owner, repo), c.token, &out)
	return out, err
}

func (c *Client) TriggerRun(owner, repo, workflowID, event, branch string) (map[string]any, error) {
	var out map[string]any
	err := c.post(fmt.Sprintf("/api/v1/repos/%s/%s/actions/runs", owner, repo), c.token, map[string]any{
		"workflow_id": workflowID, "event": event, "branch": branch,
	}, &out)
	return out, err
}

func (c *Client) Search(query string) ([]map[string]any, error) {
	var out []map[string]any
	err := c.get("/api/v1/search?q="+url.QueryEscape(query), c.token, &out)
	return out, err
}

func (c *Client) get(path, token string, out any) error {
	return c.do(http.MethodGet, path, token, nil, out)
}

func (c *Client) post(path, token string, body any, out any) error {
	return c.do(http.MethodPost, path, token, body, out)
}

func (c *Client) do(method, path, token string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var errBody map[string]string
		_ = json.Unmarshal(data, &errBody)
		msg := errBody["error"]
		if msg == "" {
			msg = string(data)
		}
		return fmt.Errorf("%s: %s", resp.Status, msg)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func ParseOwnerRepo(spec string) (owner, repo string, err error) {
	spec = strings.TrimSuffix(spec, ".git")
	parts := strings.Split(spec, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/repo, got %q", spec)
	}
	return parts[0], parts[1], nil
}

func GitClone(cloneURL, dir string) error {
	args := []string{"clone", cloneURL}
	if dir != "" {
		args = append(args, dir)
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
