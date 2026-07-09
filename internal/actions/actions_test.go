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
