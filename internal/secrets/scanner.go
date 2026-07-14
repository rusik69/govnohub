// Package secrets provides secret scanning for git push operations.
// It detects common secret patterns (API keys, tokens, passwords, etc.)
// in text content and is used by the git-server pre-receive hook to
// prevent committing secrets to repositories.
package secrets

import (
	"regexp"
)

// Pattern defines a secret detection pattern.
type Pattern struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Regexp      *regexp.Regexp `json:"-"`
}

// SecretFinding represents a detected secret in scanned content.
type SecretFinding struct {
	Type     string `json:"type"`
	Line     int    `json:"line"`
	Match    string `json:"match,omitempty"`
	Severity string `json:"severity"`
}

// ScanResult contains all findings from a scan.
type ScanResult struct {
	Findings []SecretFinding `json:"findings"`
	Passed   bool            `json:"passed"`
}

// DefaultPatterns is the default set of secret detection patterns.
var DefaultPatterns = []Pattern{
	{
		Name:        "aws-access-key",
		Description: "AWS Access Key ID (AKIA prefix)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:        "aws-secret-key",
		Description: "AWS Secret Access Key near 'aws' context",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`(?i)aws.{0,60}(key|secret|token).{0,30}=.{0,10}['\"][0-9a-zA-Z\/+=]{40}['\"]`),
	},
	{
		Name:        "github-pat",
		Description: "GitHub Personal Access Token (ghp_)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`ghp_[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "github-oauth",
		Description: "GitHub OAuth Access Token (gho_)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`gho_[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "github-app-token",
		Description: "GitHub App Installation Token (ghs_)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`ghs_[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "github-refresh-token",
		Description: "GitHub App Refresh Token (ghr_)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`ghr_[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "github-fine-grained-pat",
		Description: "GitHub Fine-Grained PAT (github_pat_)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`github_pat_[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "ssh-private-key",
		Description: "SSH Private Key header",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`-----BEGIN (RSA|DSA|EC|OPENSSH) PRIVATE KEY-----`),
	},
	{
		Name:        "pgp-private-key",
		Description: "PGP Private Key Block header",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`-----BEGIN PGP PRIVATE KEY BLOCK-----`),
	},
	{
		Name:        "slack-token",
		Description: "Slack API Token (xox*)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`xox[baprs]-[0-9a-zA-Z]{4,}\b`),
	},
	{
		Name:        "slack-webhook",
		Description: "Slack Webhook URL",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`(?:https://)?hooks\.slack\.com/services/T[a-zA-Z0-9_]{8,}/B[a-zA-Z0-9_]{8,}/[a-zA-Z0-9_]{24,}`),
	},
	{
		Name:        "google-api-key",
		Description: "Google API Key (AIza prefix)",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`AIza[0-9A-Za-z_-]{4,}\b`),
	},
	{
		Name:        "heroku-api-key",
		Description: "Heroku API Key",
		Severity:    "high",
		Regexp:      regexp.MustCompile(`[hH][eE][rR][oO][kK][uU].{0,30}[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}`),
	},
	{
		Name:        "password-in-code",
		Description: "Password string literal assigned in code",
		Severity:    "medium",
		Regexp:      regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*['\"][^'\"]{8,}['\"]`),
	},
	{
		Name:        "secret-in-code",
		Description: "Generic secret or token assigned in code",
		Severity:    "medium",
		Regexp:      regexp.MustCompile(`(?i)(secret|token)\s*[:=]\s*['\"][^'\"]{8,}['\"]`),
	},
}

// Scan scans the given text content for secret patterns.
func Scan(content string) *ScanResult {
	res := &ScanResult{
		Findings: []SecretFinding{},
		Passed:   true,
	}

	lines := splitLines(content)
	seen := make(map[string]bool)

	for _, p := range DefaultPatterns {
		matches := p.Regexp.FindAllStringIndex(content, -1)
		for _, m := range matches {
			lineNum := lineFromPos(lines, m[0])
			matchText := content[m[0]:m[1]]
			key := p.Name + ":" + itoa(lineNum)
			if seen[key] {
				continue
			}
			seen[key] = true
			res.Findings = append(res.Findings, SecretFinding{
				Type:     p.Name,
				Line:     lineNum,
				Match:    maskSecret(matchText),
				Severity: p.Severity,
			})
		}
	}

	if len(res.Findings) > 0 {
		res.Passed = false
	}
	return res
}

// maskSecret masks the middle portion of a secret for safe display.
func maskSecret(s string) string {
	if len(s) <= 8 {
		return s[:min(4, len(s))] + "..."
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// lineFromPos returns the 1-based line number for a byte position.
func lineFromPos(lines []string, pos int) int {
	offset := 0
	for i, line := range lines {
		if pos >= offset && pos < offset+len(line) {
			return i + 1
		}
		offset += len(line) + 1
	}
	return len(lines)
}

// itoa is a simple int to string converter.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
