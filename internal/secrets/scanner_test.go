package secrets

import (
	"testing"
)

func hasFinding(result *ScanResult, typ string) bool {
	for _, f := range result.Findings {
		if f.Type == typ {
			return true
		}
	}
	return false
}

func TestScan_NoSecrets(t *testing.T) {
	result := Scan("package main; func main() {}")
	if !result.Passed {
		t.Errorf("expected Passed=true, got %d findings", len(result.Findings))
	}
}

func TestScan_AWSAccessKey(t *testing.T) {
	result := Scan("AKIAAAAAAAAAAAAAAAAAAA")
	if !hasFinding(result, "aws-access-key") {
		t.Fatal("expected aws-access-key finding")
	}
}

func TestScan_GitHubPAT(t *testing.T) {
	result := Scan("ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !hasFinding(result, "github-pat") {
		t.Fatal("expected github-pat finding")
	}
}

func TestScan_GitHubOAuth(t *testing.T) {
	result := Scan("gho_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !hasFinding(result, "github-oauth") {
		t.Fatal("expected github-oauth finding")
	}
}

func TestScan_GitHubAppToken(t *testing.T) {
	result := Scan("ghs_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !hasFinding(result, "github-app-token") {
		t.Fatal("expected github-app-token finding")
	}
}

func TestScan_SSHPrivateKey(t *testing.T) {
	result := Scan("-----BEGIN RSA PRIVATE KEY-----")
	if !hasFinding(result, "ssh-private-key") {
		t.Fatal("expected ssh-private-key finding")
	}
}

func TestScan_PGPPrivateKey(t *testing.T) {
	result := Scan("-----BEGIN PGP PRIVATE KEY BLOCK-----")
	if !hasFinding(result, "pgp-private-key") {
		t.Fatal("expected pgp-private-key finding")
	}
}

func TestScan_SlackToken(t *testing.T) {
	result := Scan("xoxb-1abc")
	if !hasFinding(result, "slack-token") {
		t.Fatal("expected slack-token finding")
	}
}

func TestScan_SlackWebhook(t *testing.T) {
	result := Scan("hooks.slack.com/services/T_example_/B_example_/abcdefghijklmnopqrstuvwx")
	if !hasFinding(result, "slack-webhook") {
		t.Fatal("expected slack-webhook finding")
	}
}

func TestScan_GoogleAPIKey(t *testing.T) {
	result := Scan("AIzaSyDTzGHIJKLMNOPQRSTUVWXYZabcdefghij")
	if !hasFinding(result, "google-api-key") {
		t.Fatal("expected google-api-key finding")
	}
}

func TestScan_PasswordInCode(t *testing.T) {
	result := Scan("password = \"s3cret!passw0rd123\"")
	if !hasFinding(result, "password-in-code") {
		t.Fatal("expected password-in-code finding")
	}
}

func TestScan_SecretInCode(t *testing.T) {
	result := Scan("secret = \"my...sult := Scan(content)\"")
	if !hasFinding(result, "secret-in-code") {
		t.Fatal("expected secret-in-code finding")
	}
}

func TestScan_FalsePositiveAvoidance(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"function name", "func getPassword() string { return \"\" }"},
		{"url path", "url := \"https://api.example.com/v1/users\""},
		{"short value", "key = \"abc123\""},
		{"sha hash", "\"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\""},
		{"short hex", "\"deadbeef12345678\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Scan(tt.content)
			if !result.Passed {
				t.Errorf("unexpected findings in %q: %v", tt.content, result.Findings)
			}
		})
	}
}

func TestScan_MultipleFindings(t *testing.T) {
	c := "AKIAAAAAAAAAAAAAAAAAAA\nbar_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"
	result := Scan(c)
	if result.Passed {
		t.Fatal("expected Passed=false for multiple secrets")
	}
	if len(result.Findings) < 1 {
		t.Errorf("expected at least 1 finding, got %d", len(result.Findings))
	}
}

func TestScan_EmptyContent(t *testing.T) {
	result := Scan("")
	if !result.Passed {
		t.Error("expected Passed=true for empty content")
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestDefaultPatterns_NotEmpty(t *testing.T) {
	if len(DefaultPatterns) == 0 {
		t.Fatal("DefaultPatterns should not be empty")
	}
	for _, p := range DefaultPatterns {
		if p.Name == "" {
			t.Errorf("pattern with empty name")
		}
		if p.Regexp == nil {
			t.Errorf("pattern %q has nil regexp", p.Name)
		}
	}
}
