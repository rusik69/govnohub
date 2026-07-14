package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/auth"
)

const sessionCookie = "govnohub_session"

type ctxKey int

const (
	ctxUserKey ctxKey = iota
	ctxCSRFKey
)

type SessionUser struct {
	ID       uuid.UUID
	Username string
	User     *auth.User
}

func userFrom(ctx context.Context) *SessionUser {
	u, _ := ctx.Value(ctxUserKey).(*SessionUser)
	return u
}

func csrfFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxCSRFKey).(string)
	return s
}

func (h *Handler) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

func (h *Handler) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}

func (h *Handler) sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

func (h *Handler) withUser(ctx context.Context, token string) (context.Context, *SessionUser) {
	if token == "" {
		return ctx, nil
	}
	id, username, err := h.deps.Auth.ValidateToken(ctx, token)
	if err != nil {
		return ctx, nil
	}
	u, _ := h.deps.Auth.GetUser(ctx, id)
	su := &SessionUser{ID: id, Username: username, User: u}
	return context.WithValue(ctx, ctxUserKey, su), su
}

func (h *Handler) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := h.withUser(r.Context(), h.sessionToken(r))
		token, _ := h.ensureCSRF(w, r)
		ctx = context.WithValue(ctx, ctxCSRFKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, su := h.withUser(r.Context(), h.sessionToken(r))
		if su == nil {
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
			return
		}
		token, _ := h.ensureCSRF(w, r)
		ctx = context.WithValue(ctx, ctxCSRFKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		su := userFrom(r.Context())
		if su == nil || su.User == nil || su.User.Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func skipWeb(path string) bool {
	return strings.HasPrefix(path, "/api") || path == "/healthz"
}
