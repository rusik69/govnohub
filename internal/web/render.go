package web

import (
	"bytes"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/repo"
)

// relativeTime returns a human-readable "time ago" string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "1m ago"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 2*time.Hour:
		return "1h ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	case d < 48*time.Hour:
		return "yesterday"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	default:
		return t.Format("Jan 2")
	}
}

// timeAttr returns an RFC3339 string for use in the datetime attribute of <time>.
func timeAttr(t time.Time) string {
	return t.Format(time.RFC3339)
}

// timeTooltip returns a full date/time string for use in the title attribute of <time>.
func timeTooltip(t time.Time) string {
	return t.Format("Jan 2, 2006 3:04 PM")
}

func (h *Handler) layout(r *http.Request, title string) LayoutData {
	su := userFrom(r.Context())
	unread := 0
	if su != nil {
		unread, _ = h.deps.Notify.UnreadCount(r.Context(), su.ID)
	}
	flash := r.URL.Query().Get("ok")
	flashErr := false
	if flash == "" {
		flash = r.URL.Query().Get("err")
		flashErr = flash != ""
	}
	return LayoutData{
		Title:        title,
		User:         su,
		CSRF:         csrfFrom(r.Context()),
		UnreadNotifs: unread,
		Flash:        flash,
		FlashErr:     flashErr,
	}
}

func (h *Handler) repoPageCtx(r *http.Request, repository *repo.Repository, tab string) (RepoHeaderData, RepoNavData) {
	issues, _ := h.deps.Issues.List(r.Context(), repository.ID)
	prs, _ := h.deps.Pulls.List(r.Context(), repository.ID)
	header := RepoHeaderData{
		Repository: repository,
		CSRF:       csrfFrom(r.Context()),
		CloneHTTPS: cloneHTTPSURL(repository.OwnerName, repository.Name),
		CloneSSH:   cloneSSHURL(repository.OwnerName, repository.Name),
	}
	if su := userFrom(r.Context()); su != nil {
		header.Starred, _ = h.deps.Repos.IsStarred(r.Context(), repository.ID, su.ID)
		header.Watched, _ = h.deps.Repos.IsWatched(r.Context(), repository.ID, su.ID)
	}
	return header, RepoNavData{
		Owner:      repository.OwnerName,
		Repo:       repository.Name,
		Tab:        tab,
		FullName:   repository.FullName,
		IssueCount: len(issues),
		PullCount:  len(prs),
	}
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) repoBranches(r *http.Request, repository *repo.Repository) []RepoBranchOption {
	branches, err := h.deps.Repos.ListBranches(r.Context(), repository.ID)
	if err != nil || len(branches) == 0 {
		return []RepoBranchOption{{Name: repository.DefaultBranch}}
	}
	out := make([]RepoBranchOption, len(branches))
	for i, b := range branches {
		out[i] = RepoBranchOption{Name: b.Name}
	}
	return out
}

func isBinaryContent(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func redirectReferer(r *http.Request, fallback string) string {
	if ref := r.Header.Get("Referer"); ref != "" {
		return ref
	}
	return fallback
}

func redirectWithFlash(w http.ResponseWriter, r *http.Request, path, msg string, isErr bool) {
	u, err := url.Parse(path)
	if err != nil {
		http.Redirect(w, r, path, http.StatusSeeOther)
		return
	}
	q := u.Query()
	if isErr {
		q.Set("err", msg)
		q.Del("ok")
	} else {
		q.Set("ok", msg)
		q.Del("err")
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (h *Handler) getRepo(w http.ResponseWriter, r *http.Request, perm string) (*repo.Repository, bool) {
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "repo")
	repository, err := h.deps.Repos.GetByFullName(r.Context(), owner, name)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	su := userFrom(r.Context())
	if su == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, false
	}
	ok, _ := h.deps.Repos.CanAccess(r.Context(), repository.ID, su.ID, perm)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return repository, true
}

func parseNum(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, key))
	if err != nil || n <= 0 {
		http.Error(w, "invalid number", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

func parsePageLimit(r *http.Request, defaultPage, defaultLimit int) (page, limit int) {
	page = defaultPage
	limit = defaultLimit
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 1 {
			page = n
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}
	return
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, key))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if err := parseFormCSRF(r); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return false
	}
	if !h.validateCSRF(r) {
		http.Error(w, "invalid csrf", http.StatusForbidden)
		return false
	}
	return true
}

func renderToast(w http.ResponseWriter, r *http.Request, msg, variant string) {
	render(w, r, Toast(msg, variant))
}
