package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"

	"github.com/rusik69/govnohub/internal/actions"
)

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, path, active FROM workflows WHERE repo_id=$1`, repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var wfs []map[string]interface{}
	for rows.Next() {
		var id uuid.UUID
		var name, path string
		var active bool
		rows.Scan(&id, &name, &path, &active)
		wfs = append(wfs, map[string]interface{}{"id": id, "name": name, "path": path, "active": active})
	}
	jsonOK(w, wfs)
}

func (s *Server) handleUpsertWorkflow(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if _, err := actions.ParseWorkflow(req.Content); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	var id uuid.UUID
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO workflows (repo_id, name, path, content)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (repo_id, path) DO UPDATE SET name=EXCLUDED.name, content=EXCLUDED.content
		RETURNING id`, repository.ID, req.Name, req.Path, req.Content).Scan(&id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{"id": id})
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, run_number, event, head_sha, head_branch, status, conclusion, created_at
		FROM workflow_runs WHERE repo_id=$1 ORDER BY run_number DESC LIMIT 50`, repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var runs []map[string]interface{}
	for rows.Next() {
		var id uuid.UUID
		var runNumber int
		var event, sha, branch, status string
		var conclusion *string
		var createdAt interface{}
		rows.Scan(&id, &runNumber, &event, &sha, &branch, &status, &conclusion, &createdAt)
		runs = append(runs, map[string]interface{}{
			"id": id, "run_number": runNumber, "event": event, "head_sha": sha,
			"head_branch": branch, "status": status, "conclusion": conclusion, "created_at": createdAt,
		})
	}
	jsonOK(w, runs)
}

func (s *Server) handleTriggerRun(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		WorkflowID uuid.UUID `json:"workflow_id"`
		Event      string    `json:"event"`
		Branch     string    `json:"branch"`
		SHA        string    `json:"sha"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Branch == "" {
		req.Branch = repository.DefaultBranch
	}
	if req.SHA == "" {
		req.SHA, _ = s.git.UpdateHead(repository.OwnerName, repository.Name, req.Branch)
	}
	var runNumber int
	s.pool.QueryRow(r.Context(), `SELECT COALESCE(MAX(run_number),0)+1 FROM workflow_runs WHERE repo_id=$1`, repository.ID).Scan(&runNumber)
	var runID uuid.UUID
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO workflow_runs (repo_id, workflow_id, run_number, event, head_sha, head_branch, status)
		VALUES ($1,$2,$3,$4,$5,$6,'queued') RETURNING id`,
		repository.ID, req.WorkflowID, runNumber, req.Event, req.SHA, req.Branch).Scan(&runID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{"id": runID, "run_number": runNumber, "status": "queued"})
}

func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT job_id, name, status, conclusion, log_path FROM workflow_jobs WHERE run_id=$1`, runID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type jobLog struct {
		JobID  string `json:"job_id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Log    string `json:"log"`
	}
	var logs []jobLog
	for rows.Next() {
		var jl jobLog
		var logPath *string
		rows.Scan(&jl.JobID, &jl.Name, &jl.Status, new(*string), &logPath)
		if logPath != nil {
			b, _ := os.ReadFile(*logPath)
			jl.Log = string(b)
		}
		logs = append(logs, jl)
	}
	jsonOK(w, logs)
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	releases, err := s.releases.List(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, releases)
}

func (s *Server) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		TagName, Name, Body string
		Draft, Prerelease   bool
	}
	json.NewDecoder(r.Body).Decode(&req)
	rel, err := s.releases.Create(r.Context(), repository.ID, userIDFrom(r.Context()), req.TagName, req.Name, req.Body, req.Draft, req.Prerelease)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, rel)
}

func (s *Server) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	tag := chi.URLParam(r, "tag")
	releases, _ := s.releases.List(r.Context(), repository.ID)
	var releaseID uuid.UUID
	for _, rel := range releases {
		if rel.TagName == tag {
			releaseID = rel.ID
			break
		}
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()
	asset, err := s.releases.UploadAsset(r.Context(), releaseID, header.Filename, header.Header.Get("Content-Type"), file, header.Size)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, asset)
}

func (s *Server) handleListPackages(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	pkgs, err := s.packages.List(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, pkgs)
}

func (s *Server) handlePublishPackage(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	version := r.URL.Query().Get("version")
	pkgType := r.URL.Query().Get("type")
	if pkgType == "" {
		pkgType = "container"
	}
	p, err := s.packages.Publish(r.Context(), repository.ID, name, pkgType, version, r.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, p)
}

func (s *Server) handleDownloadPackage(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	rc, err := s.packages.Open(r.Context(), repository.ID, chi.URLParam(r, "name"), chi.URLParam(r, "version"))
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, rc)
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, url, events, active FROM webhooks WHERE repo_id=$1`, repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var hooks []map[string]interface{}
	for rows.Next() {
		var id uuid.UUID
		var url string
		var events []string
		var active bool
		rows.Scan(&id, &url, &events, &active)
		hooks = append(hooks, map[string]interface{}{"id": id, "url": url, "events": events, "active": active})
	}
	jsonOK(w, hooks)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	h, err := s.webhooks.Create(r.Context(), repository.ID, req.URL, req.Secret, req.Events)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, h)
}

func (s *Server) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct{ Name, Base string }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Base == "" {
		req.Base = repository.DefaultBranch
	}
	if err := s.git.CreateBranch(repository.OwnerName, repository.Name, req.Name, req.Base); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sha, _ := s.git.UpdateHead(repository.OwnerName, repository.Name, req.Name)
	s.repos.UpdateBranchHead(r.Context(), repository.ID, req.Name, sha)
	jsonOK(w, map[string]string{"branch": req.Name, "sha": sha})
}

func (s *Server) handleProtectBranch(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		Branch         string   `json:"branch"`
		RequiredChecks []string `json:"required_checks"`
		RequireReviews int      `json:"require_reviews"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	_, err := s.pool.Exec(r.Context(), `
		INSERT INTO protected_branches (repo_id, branch_name, required_checks, require_reviews)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (repo_id, branch_name) DO UPDATE SET required_checks=EXCLUDED.required_checks, require_reviews=EXCLUDED.require_reviews`,
		repository.ID, req.Branch, req.RequiredChecks, req.RequireReviews)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "protected"})
}
