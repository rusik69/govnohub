package actions

import (
	"testing"
)

func TestParseWorkflow(t *testing.T) {
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: linux
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	if wf.Name != "CI" {
		t.Fatalf("name=%s", wf.Name)
	}
	if !wf.MatchesTrigger(TriggerEvent{Type: "push", Branch: "main"}) {
		t.Fatal("should match push")
	}
	order, err := wf.JobOrder()
	if err != nil || len(order) != 1 || order[0] != "build" {
		t.Fatalf("order=%v err=%v", order, err)
	}
}

func TestEvalExpression(t *testing.T) {
	ctx := map[string]string{"GITHUB_SHA": "abc123"}
	if got := EvalExpression("${{ github.sha }}", ctx); got != "abc123" {
		t.Fatalf("got %s", got)
	}
}

func TestJobOrderWithNeeds(t *testing.T) {
	content := `
name: Pipeline
on: [push]
jobs:
  build:
    steps: [{run: echo build}]
  test:
    needs: build
    steps: [{run: echo test}]
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	order, err := wf.JobOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "build" || order[1] != "test" {
		t.Fatalf("order=%v", order)
	}
}

func TestBranchFilter(t *testing.T) {
	content := `
name: CI
on:
  push:
    branches: [main]
jobs:
  x:
    steps: [{run: echo}]
`
	wf, _ := ParseWorkflow(content)
	if !wf.MatchesTrigger(TriggerEvent{Type: "push", Branch: "main"}) {
		t.Fatal("should match main")
	}
	if wf.MatchesTrigger(TriggerEvent{Type: "push", Branch: "dev"}) {
		t.Fatal("should not match dev")
	}
}

func TestBuildUsesScript(t *testing.T) {
	script := BuildUsesScript(Step{Uses: "actions/checkout@v4"}, GitHubContext("o", "r", "sha", "main", "push"))
	if !contains(script, "Checking out") {
		t.Fatalf("checkout script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/setup-go@v5"}, nil)
	if !contains(script, "Go") {
		t.Fatalf("setup-go script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/unknown@v1"}, nil)
	if !contains(script, "warning") {
		t.Fatalf("unknown script=%s", script)
	}
}

func TestWriteJobScript(t *testing.T) {
	script := WriteJobScript([]Step{{Name: "hi", Run: "echo hello"}}, GitHubContext("o", "r", "sha", "main", "push"))
	if script == "" || !contains(script, "echo hello") {
		t.Fatalf("script=%s", script)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
