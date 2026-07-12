package web

import (
	"fmt"
	"os"
	"strings"
)

func gitHTTPBase() string {
	if v := os.Getenv("GIT_HTTP_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://git.govnohub.local"
}

func gitSSHHost() string {
	if v := os.Getenv("GIT_SSH_HOST"); v != "" {
		return v
	}
	return "git@git.govnohub.local:2222"
}

func cloneHTTPSURL(owner, repo string) string {
	return fmt.Sprintf("%s/%s/%s.git", gitHTTPBase(), owner, repo)
}

func cloneSSHURL(owner, repo string) string {
	return fmt.Sprintf("%s:%s/%s.git", gitSSHHost(), owner, repo)
}
