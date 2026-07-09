package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("HTTP_ADDR")
	cfg := Load()
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("addr=%s", cfg.HTTPAddr)
	}
	if cfg.RunnerNS != "govnohub-runners" {
		t.Fatalf("runner ns=%s", cfg.RunnerNS)
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")
	if got := GetEnvInt("TEST_INT", 0); got != 42 {
		t.Fatalf("got=%d", got)
	}
}
