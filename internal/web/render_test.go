package web

import (
	"net/http"
	"testing"
)

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello\n")) {
		t.Fatal("text should not be binary")
	}
	if !isBinaryContent([]byte{0x00, 0x01}) {
		t.Fatal("nul byte should be binary")
	}
}

func TestRedirectReferer(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	if got := redirectReferer(r, "/fallback"); got != "/fallback" {
		t.Fatalf("got %q", got)
	}
	r.Header.Set("Referer", "/alice/app")
	if got := redirectReferer(r, "/fallback"); got != "/alice/app" {
		t.Fatalf("got %q", got)
	}
}
