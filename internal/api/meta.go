package api

import (
	"net/http"
	"runtime"
	"time"
)

// Version is set at build time via -ldflags.
var Version = "0.1.0-dev"

// BuildTime is the time the binary was built, set via -ldflags.
var BuildTime = ""

type metaResponse struct {
	Version                  string            `json:"version"`
	BuildTime                string            `json:"build_time,omitempty"`
	VerifiablePasswordAuth   bool              `json:"verifiable_password_authentication"`
	GitProtocols             []string          `json:"git_protocols"`
	Capabilities             map[string]bool   `json:"capabilities"`
	AuthMethods              []string          `json:"auth_methods"`
	PublicRegistration       bool              `json:"public_registration"`
	GoVersion                string            `json:"go_version"`
	OSArch                   string            `json:"os_arch"`
	Now                      time.Time         `json:"now"`
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	resp := metaResponse{
		Version:                Version,
		BuildTime:              BuildTime,
		VerifiablePasswordAuth: true,
		GitProtocols:           []string{"http", "ssh"},
		Capabilities: map[string]bool{
			"actions":    true,
			"packages":   true,
			"issues":     true,
			"pull_requests": true,
			"wiki":       true,
			"releases":   true,
			"search":     true,
			"webhooks":   true,
			"ai_review":  true,
			"audit_log":  true,
		},
		AuthMethods:        []string{"token", "jwt"},
		PublicRegistration: s.auth.AllowPublicRegistration(),
		GoVersion:          runtime.Version(),
		OSArch:             runtime.GOOS + "/" + runtime.GOARCH,
		Now:                time.Now(),
	}
	jsonOK(w, resp)
}
