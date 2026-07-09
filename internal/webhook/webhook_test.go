package webhook

import (
	"testing"
)

func TestSign(t *testing.T) {
	body := []byte(`{"event":"push"}`)
	sig := sign("secret", body)
	if sig == "" || len(sig) != 64 {
		t.Fatalf("sig=%s", sig)
	}
}

func TestNewPushEvent(t *testing.T) {
	e := NewPushEvent("alice", "demo", "main", "sha", "alice")
	if e.Repository != "alice/demo" || e.After != "sha" {
		t.Fatalf("event=%+v", e)
	}
}
