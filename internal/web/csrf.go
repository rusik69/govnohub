package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

const csrfCookie = "govnohub_csrf"

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// csrfKey derives an HMAC key for CSRF token signing, bound to the current
// session. When a user is logged in, the key is derived from their session
// token, so CSRF tokens are invalidated on session change (login/logout).
// For anonymous users, a fixed key is used.
func (h *Handler) csrfKey(r *http.Request) []byte {
	sessionToken := h.sessionToken(r)
	if sessionToken != "" {
		mac := hmac.New(sha256.New, []byte("govnohub-csrf-v2"))
		mac.Write([]byte(sessionToken))
		return mac.Sum(nil)
	}
	return []byte("govnohub-csrf-v2")
}

func (h *Handler) signCSRF(token string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// ensureCSRF returns the current CSRF token, generating one if needed.
// On first call per session, it creates a new token and sets it as a cookie.
// Subsequent calls reuse the existing token if it is still valid.
func (h *Handler) ensureCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	c, err := r.Cookie(csrfCookie)
	if err == nil && c.Value != "" {
		// Validate token format and signature before reusing
		parts := strings.SplitN(c.Value, ".", 2)
		if len(parts) == 2 && len(parts[0]) == 32 {
			key := h.csrfKey(r)
			expectedSig := h.signCSRF(parts[0], key)
			if hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
				return c.Value, nil
			}
		}
	}
	// No valid cookie — create a new token
	return h.newCSRFToken(w, r)
}

// rotateCSRF generates a fresh CSRF token, replacing the existing one
// in the cookie. The old token is invalidated immediately.
func (h *Handler) rotateCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	return h.newCSRFToken(w, r)
}

// newCSRFToken creates a fresh CSRF token, sets it as a cookie, and returns it.
func (h *Handler) newCSRFToken(w http.ResponseWriter, r *http.Request) (string, error) {
	raw, err := randomHex(16)
	if err != nil {
		return "", err
	}
	key := h.csrfKey(r)
	token := raw + "." + h.signCSRF(raw, key)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	return token, nil
}

func (h *Handler) validateCSRF(r *http.Request) bool {
	expected := csrfFrom(r.Context())
	if expected == "" {
		return false
	}
	got := r.FormValue("csrf_token")
	if got == "" {
		got = r.Header.Get("X-CSRF-Token")
	}
	return got != "" && hmac.Equal([]byte(got), []byte(expected))
}

func parseFormCSRF(r *http.Request) error {
	if r.Method == http.MethodPost && strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		return r.ParseMultipartForm(32 << 20)
	}
	return r.ParseForm()
}
