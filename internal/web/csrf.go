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

func (h *Handler) csrfKey() []byte {
	return []byte("govnohub-csrf-v1")
}

func (h *Handler) signCSRF(token string) string {
	mac := hmac.New(sha256.New, h.csrfKey())
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) ensureCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	c, err := r.Cookie(csrfCookie)
	if err == nil && c.Value != "" {
		return c.Value, nil
	}
	raw, err := randomHex(16)
	if err != nil {
		return "", err
	}
	token := raw + "." + h.signCSRF(raw)
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
