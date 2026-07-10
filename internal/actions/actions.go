package actions

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Workflow struct {
	Name  string            `yaml:"name"`
	On    interface{}       `yaml:"on"`
	Env   map[string]string `yaml:"env"`
	Jobs  map[string]Job    `yaml:"jobs"`
}

type Job struct {
	RunsOn string            `yaml:"runs-on"`
	Needs  interface{}       `yaml:"needs"`
	Env    map[string]string `yaml:"env"`
	Steps  []Step            `yaml:"steps"`
}

type Step struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

type TriggerEvent struct {
	Type   string
	Branch string
	SHA    string
	PR     bool
}

func ParseWorkflow(content string) (*Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal([]byte(content), &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func (wf *Workflow) MatchesTrigger(event TriggerEvent) bool {
	switch v := wf.On.(type) {
	case string:
		return v == event.Type
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == event.Type {
				return true
			}
		}
	case map[string]interface{}:
		cfg, ok := v[event.Type]
		if !ok {
			return false
		}
		return matchEventConfig(cfg, event)
	}
	return false
}

func matchEventConfig(cfg interface{}, event TriggerEvent) bool {
	if cfg == nil {
		return true
	}
	m, ok := cfg.(map[string]interface{})
	if !ok {
		return true
	}
	if branches, ok := m["branches"].([]interface{}); ok {
		matched := false
		for _, b := range branches {
			if s, ok := b.(string); ok && (s == event.Branch || s == "*") {
				matched = true
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (wf *Workflow) JobOrder() ([]string, error) {
	visited := map[string]bool{}
	var order []string
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if _, ok := wf.Jobs[id]; !ok {
			return fmt.Errorf("unknown job: %s", id)
		}
		visited[id] = true
		job := wf.Jobs[id]
		for _, dep := range NormalizeNeeds(job.Needs) {
			if err := visit(dep); err != nil {
				return err
			}
		}
		order = append(order, id)
		return nil
	}
	for id := range wf.Jobs {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func NormalizeNeeds(v interface{}) []string {
	switch n := v.(type) {
	case string:
		return []string{n}
	case []interface{}:
		var out []string
		for _, item := range n {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func EvalExpression(expr string, ctx map[string]string) string {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "${{") {
		return expr
	}
	expr = strings.TrimSuffix(strings.TrimPrefix(expr, "${{"), "}}")
	expr = strings.TrimSpace(expr)
	parts := strings.Split(expr, ".")
	if len(parts) >= 2 && parts[0] == "github" {
		key := "GITHUB_" + strings.ToUpper(parts[1])
		if v, ok := ctx[key]; ok {
			return v
		}
	}
	if v, ok := ctx[expr]; ok {
		return v
	}
	return ""
}

func BuildStepScript(step Step, ctx map[string]string) string {
	if step.Run != "" {
		return EvalExpression(step.Run, ctx)
	}
	if step.Uses != "" {
		return BuildUsesScript(step, ctx)
	}
	return "true"
}

func GitHubContext(owner, repo, sha, ref, event, runID, artifactRoot string) map[string]string {
	return map[string]string{
		"GITHUB_REPOSITORY":       owner + "/" + repo,
		"GITHUB_SHA":              sha,
		"GITHUB_REF":              "refs/heads/" + ref,
		"GITHUB_EVENT_NAME":       event,
		"GITHUB_WORKSPACE":        "/github/workspace",
		"GITHUB_RUN_ID":           runID,
		"GOVNOHUB_ARTIFACT_ROOT":  artifactRoot,
	}
}

func WriteJobScript(steps []Step, ctx map[string]string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -e\n")
	for _, step := range steps {
		name := step.Name
		if name == "" {
			name = "step"
		}
		b.WriteString(fmt.Sprintf("echo '::group::%s'\n", name))
		for k, v := range step.Env {
			b.WriteString(fmt.Sprintf("export %s='%s'\n", k, EvalExpression(v, ctx)))
		}
		b.WriteString(BuildStepScript(step, ctx))
		b.WriteString("\necho '::endgroup::'\n")
	}
	return b.String()
}

func SaveLog(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
