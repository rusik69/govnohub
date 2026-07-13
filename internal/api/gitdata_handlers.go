package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	gitstore "github.com/rusik69/govnohub/internal/git"
)

// handleCreateGitBlob creates a blob object via the Git Data API.
// POST /api/v1/repos/{owner}/{repo}/git/blobs
func (s *Server) handleCreateGitBlob(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"` // "utf-8" or "base64"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		jsonError(w, http.StatusBadRequest, "content is required")
		return
	}
	content := []byte(req.Content)
	if req.Encoding == "base64" {
		decoded, err := decodeBase64(req.Content)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid base64 content")
			return
		}
		content = decoded
	}
	sha, err := s.git.CreateBlob(repository.OwnerName, repository.Name, content)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"sha": sha})
}

// handleCreateGitTree creates a tree object via the Git Data API.
// POST /api/v1/repos/{owner}/{repo}/git/trees
func (s *Server) handleCreateGitTree(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Tree []gitstore.TreeEntryInput `json:"tree"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Tree) == 0 {
		jsonError(w, http.StatusBadRequest, "tree entries are required")
		return
	}
	sha, err := s.git.CreateTree(repository.OwnerName, repository.Name, req.Tree)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"sha": sha})
}

// handleCreateGitCommit creates a commit object via the Git Data API.
// POST /api/v1/repos/{owner}/{repo}/git/commits
func (s *Server) handleCreateGitCommit(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req gitstore.CreateCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Message == "" {
		jsonError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.Tree == "" {
		jsonError(w, http.StatusBadRequest, "tree is required")
		return
	}
	sha, err := s.git.CreateCommit(repository.OwnerName, repository.Name, req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"sha": sha})
}

// handleCreateGitRef creates or updates a git reference via the Git Data API.
// POST /api/v1/repos/{owner}/{repo}/git/refs
func (s *Server) handleCreateGitRef(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req gitstore.CreateRefParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" {
		jsonError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if req.SHA == "" {
		jsonError(w, http.StatusBadRequest, "sha is required")
		return
	}
	if err := s.git.CreateRef(repository.OwnerName, repository.Name, req); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"ref": req.Ref, "sha": req.SHA})
}

// decodeBase64 decodes a base64-encoded string.
func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
