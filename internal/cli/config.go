package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultAPIURL = "http://localhost:8080"
const defaultGitURL = "http://localhost:8081"

type Config struct {
	APIURL   string `yaml:"api_url"`
	GitURL   string `yaml:"git_url"`
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
}

func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func configDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	dir := filepath.Join(base, "govnohub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{APIURL: defaultAPIURL, GitURL: defaultGitURL}
	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return cfg, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.APIURL == "" {
		cfg.APIURL = defaultAPIURL
	}
	if cfg.GitURL == "" {
		cfg.GitURL = defaultGitURL
	}
	return cfg, nil
}

func SaveConfig(cfg *Config, path string) error {
	if path == "" {
		var err error
		path, err = ConfigPath()
		if err != nil {
			return err
		}
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *Config) MergeFlags(apiURL, gitURL, token string) {
	if apiURL != "" {
		c.APIURL = apiURL
	}
	if gitURL != "" {
		c.GitURL = gitURL
	}
	if token != "" {
		c.Token = token
	}
}

func (c *Config) RequireToken() error {
	if c.Token == "" {
		return fmt.Errorf("not authenticated: run `govnohub auth login` or set --token")
	}
	return nil
}

func CloneURL(cfg *Config, owner, repo string) string {
	if cfg.Token != "" {
		return fmt.Sprintf("%s://x-access-token:%s@%s/%s/%s.git",
			scheme(cfg.GitURL), cfg.Token, hostWithoutScheme(cfg.GitURL), owner, repo)
	}
	return fmt.Sprintf("%s/%s/%s.git", stringsTrimSlash(cfg.GitURL), owner, repo)
}

func scheme(raw string) string {
	if len(raw) > 8 && raw[:8] == "https://" {
		return "https"
	}
	return "http"
}

func hostWithoutScheme(raw string) string {
	raw = stringsTrimPrefix(raw, "https://")
	raw = stringsTrimPrefix(raw, "http://")
	return raw
}

func stringsTrimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func stringsTrimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
