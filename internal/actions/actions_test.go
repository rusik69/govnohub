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

func ghCtx() map[string]string {
	return GitHubContext("o", "r", "sha", "main", "push", "run-1", "/artifacts")
}

func TestBuildUsesScript(t *testing.T) {
	script := BuildUsesScript(Step{Uses: "actions/checkout@v4"}, ghCtx())
	if !contains(script, "Checking out") {
		t.Fatalf("checkout script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/setup-go@v5"}, nil)
	if !contains(script, "Go") {
		t.Fatalf("setup-go script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/setup-node@v4", With: map[string]string{"node-version": "20"}}, ghCtx())
	if !contains(script, "Node.js") {
		t.Fatalf("setup-node script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/upload-artifact@v4", With: map[string]string{"name": "dist", "path": "out/"}}, ghCtx())
	if !contains(script, "Uploading artifact") {
		t.Fatalf("upload-artifact script=%s", script)
	}
	script = BuildUsesScript(Step{Uses: "actions/unknown@v1"}, nil)
	if !contains(script, "warning") {
		t.Fatalf("unknown script=%s", script)
	}
}

func TestWriteJobScript(t *testing.T) {
	script := WriteJobScript([]Step{{Name: "hi", Run: "echo hello"}}, ghCtx())
	if script == "" || !contains(script, "echo hello") {
		t.Fatalf("script=%s", script)
	}
}

// ---- Additional tests for Item 4 ----

// TestMatchesTrigger_Array tests the on: [push, pull_request] trigger format
func TestMatchesTrigger_Array(t *testing.T) {
	content := `
name: CI
on: [push, pull_request]
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

	cases := []struct {
		event TriggerEvent
		want  bool
	}{
		{TriggerEvent{Type: "push"}, true},
		{TriggerEvent{Type: "pull_request"}, true},
		{TriggerEvent{Type: "release"}, false},
		{TriggerEvent{Type: "workflow_dispatch"}, false},
	}
	for _, tc := range cases {
		got := wf.MatchesTrigger(tc.event)
		if got != tc.want {
			t.Errorf("MatchesTrigger(%q) = %v, want %v", tc.event.Type, got, tc.want)
		}
	}
}

// TestMatchesTrigger_MapNoConfig tests map trigger with nil/null config
func TestMatchesTrigger_MapNoConfig(t *testing.T) {
	content := `
name: CI
on:
  push:
jobs:
  build:
    steps: [{run: echo}]
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	if !wf.MatchesTrigger(TriggerEvent{Type: "push", Branch: "any"}) {
		t.Fatal("should match push with null config")
	}
	if wf.MatchesTrigger(TriggerEvent{Type: "pull_request"}) {
		t.Fatal("should not match pull_request")
	}
}

// TestMatchesTrigger_NonMatchingEvent tests that events not in the workflow return false
func TestMatchesTrigger_NonMatchingEvent(t *testing.T) {
	content := `
name: CI
on: push
jobs:
  build:
    steps: [{run: echo}]
`
	wf, _ := ParseWorkflow(content)
	if wf.MatchesTrigger(TriggerEvent{Type: "pull_request"}) {
		t.Fatal("should not match pull_request when on: push")
	}
}

// TestJobOrder_DAG tests complex dependency graphs
func TestJobOrder_DAG(t *testing.T) {
	content := `
name: DAG Pipeline
on: push
jobs:
  build:
    steps: [{run: echo build}]
  lint:
    steps: [{run: echo lint}]
  test:
    needs: [build, lint]
    steps: [{run: echo test}]
  deploy:
    needs: test
    steps: [{run: echo deploy}]
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	order, err := wf.JobOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 jobs, got %v", order)
	}
	// build and lint must come before test; test must come before deploy
	buildIdx := indexOfStr(order, "build")
	lintIdx := indexOfStr(order, "lint")
	testIdx := indexOfStr(order, "test")
	deployIdx := indexOfStr(order, "deploy")

	if buildIdx < 0 || lintIdx < 0 || testIdx < 0 || deployIdx < 0 {
		t.Fatal("missing job in order")
	}
	if buildIdx > testIdx || lintIdx > testIdx {
		t.Fatal("build/lint must come before test")
	}
	if testIdx > deployIdx {
		t.Fatal("test must come before deploy")
	}
}

// TestJobOrder_UnknownDep tests error handling for missing dependencies
func TestJobOrder_UnknownDep(t *testing.T) {
	content := `
name: Broken
on: push
jobs:
  test:
    needs: missing_job
    steps: [{run: echo}]
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	_, err = wf.JobOrder()
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

// TestNormalizeNeeds tests all variants of the needs field
func TestNormalizeNeeds(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  []string
	}{
		{"string", "build", []string{"build"}},
		{"array", []interface{}{"build", "lint"}, []string{"build", "lint"}},
		{"nil", nil, nil},
		{"empty_array", []interface{}{}, []string{}},
		{"mixed_array", []interface{}{"build", 42, "lint"}, []string{"build", "lint"}},
		{"single_element_array", []interface{}{"build"}, []string{"build"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeNeeds(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("NormalizeNeeds(%v) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("NormalizeNeeds(%v) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}

// TestEvalExpression_EdgeCases tests expression evaluation edge cases
func TestEvalExpression_EdgeCases(t *testing.T) {
	ctx := map[string]string{
		"GITHUB_SHA":      "abc123",
		"GITHUB_REF_NAME": "main",
		"MY_VAR":          "hello",
	}

	tests := []struct {
		name string
		expr string
		ctx  map[string]string
		want string
	}{
		{"github_ref_name", "${{ github.ref_name }}", ctx, "main"},
		{"non_expression", "echo hello", ctx, "echo hello"},
		{"empty_expression", "${{  }}", ctx, ""},
		{"missing_key", "${{ github.nonexistent }}", ctx, ""},
		{"custom_context", "${{ MY_VAR }}", ctx, "hello"},
		{"nested_with_dot", "${{ github.sha }}", ctx, "abc123"},
		{"empty_string", "", ctx, ""},
		{"only_braces", "${}", ctx, "${}"},
		{"malformed_prefix", "${{github.sha}}", ctx, "abc123"},
		{"nil_context", "${{ github.sha }}", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalExpression(tt.expr, tt.ctx)
			if got != tt.want {
				t.Errorf("EvalExpression(%q, ctx) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

// TestBuildStepScript tests the step script builder
func TestBuildStepScript(t *testing.T) {
	ctx := map[string]string{"GITHUB_SHA": "abc123"}

	tests := []struct {
		name string
		step Step
		want string // substring to check
	}{
		{"run_command", Step{Run: "echo hi"}, "echo hi"},
		{"uses_fallback", Step{Uses: "actions/checkout@v4"}, "Checking out"},
		{"empty_step", Step{}, "true"},
		{"expression_in_run", Step{Run: "${{ github.sha }}"}, "abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildStepScript(tt.step, ctx)
			if !contains(got, tt.want) {
				t.Errorf("BuildStepScript(%+v) = %q, want containing %q", tt.step, got, tt.want)
			}
		})
	}
}

// TestWriteJobScript_EdgeCases tests the full script writer
func TestWriteJobScript_EdgeCases(t *testing.T) {
	t.Run("empty_steps", func(t *testing.T) {
		script := WriteJobScript([]Step{}, ghCtx())
		if !contains(script, "#!/bin/sh") {
			t.Fatal("script should have shebang")
		}
	})

	t.Run("steps_with_env", func(t *testing.T) {
		steps := []Step{
			{
				Name: "build",
				Run:  "make build",
				Env:  map[string]string{"GOOS": "linux", "GOARCH": "amd64"},
			},
		}
		script := WriteJobScript(steps, ghCtx())
		if !contains(script, "export GOOS") {
			t.Fatalf("missing env var in script: %s", script)
		}
		if !contains(script, "make build") {
			t.Fatalf("missing command in script: %s", script)
		}
		if !contains(script, "::group::build") {
			t.Fatalf("missing group markers in script: %s", script)
		}
	})

	t.Run("multi_step", func(t *testing.T) {
		steps := []Step{
			{Run: "echo step1"},
			{Run: "echo step2"},
			{Run: "echo step3"},
		}
		script := WriteJobScript(steps, nil)
		if !contains(script, "echo step1") || !contains(script, "echo step3") {
			t.Fatalf("missing steps in script: %s", script)
		}
	})

	t.Run("step_with_empty_name", func(t *testing.T) {
		steps := []Step{
			{Run: "echo test"},
		}
		script := WriteJobScript(steps, nil)
		if !contains(script, "::group::step") {
			t.Fatalf("missing default step name in script: %s", script)
		}
	})
}

// ---- Tests for Item 16: matrix strategy, needs with arrays, if: conditional skip ----

// TestParseWorkflow_MatrixStrategy tests parsing workflow with strategy/matrix
func TestParseWorkflow_MatrixStrategy(t *testing.T) {
	content := `
name: Matrix CI
on: push
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        node: [18, 20]
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo "Node ${{ matrix.node }} on ${{ matrix.os }}"
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["test"]
	if !ok {
		t.Fatal("missing test job")
	}
	if job.Strategy == nil {
		t.Fatal("strategy is nil")
	}
	osVars, ok := job.Strategy.Matrix["os"]
	if !ok {
		t.Fatal("missing os in matrix")
	}
	if len(osVars) != 2 || osVars[0] != "ubuntu-latest" || osVars[1] != "macos-latest" {
		t.Fatalf("unexpected os vars: %v", osVars)
	}
	nodeVars, ok := job.Strategy.Matrix["node"]
	if !ok {
		t.Fatal("missing node in matrix")
	}
	if len(nodeVars) != 2 || nodeVars[0] != "18" || nodeVars[1] != "20" {
		t.Fatalf("unexpected node vars: %v", nodeVars)
	}
}

// TestParseWorkflow_MatrixStrategy_NoMatrix tests job without strategy
func TestParseWorkflow_MatrixStrategy_NoMatrix(t *testing.T) {
	content := `
name: Simple
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	if wf.Jobs["build"].Strategy != nil {
		t.Fatal("expected nil strategy for simple job")
	}
}

// TestParseWorkflow_MatrixStrategy_EmptyMatrix tests strategy with empty matrix
func TestParseWorkflow_MatrixStrategy_EmptyMatrix(t *testing.T) {
	content := `
name: Empty Matrix
on: push
jobs:
  build:
    strategy:
      matrix: {}
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	if wf.Jobs["build"].Strategy == nil {
		t.Fatal("strategy should not be nil")
	}
	if len(wf.Jobs["build"].Strategy.Matrix) != 0 {
		t.Fatalf("expected empty matrix, got %v", wf.Jobs["build"].Strategy.Matrix)
	}
}

// TestJobOrder_NeedsArray tests needs with array syntax [build, lint]
func TestJobOrder_NeedsArray(t *testing.T) {
	content := `
name: Pipeline
on: [push]
jobs:
  build:
    steps: [{run: echo build}]
  lint:
    steps: [{run: echo lint}]
  test:
    needs: [build, lint]
    steps: [{run: echo test}]
  deploy:
    needs: test
    steps: [{run: echo deploy}]
`
	wf, err := ParseWorkflow(content)
	if err != nil {
		t.Fatal(err)
	}
	order, err := wf.JobOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 jobs, got %v", order)
	}
	buildIdx := indexOfStr(order, "build")
	lintIdx := indexOfStr(order, "lint")
	testIdx := indexOfStr(order, "test")
	deployIdx := indexOfStr(order, "deploy")
	if buildIdx > testIdx || lintIdx > testIdx {
		t.Fatal("build/lint must come before test")
	}
	if testIdx > deployIdx {
		t.Fatal("test must come before deploy")
	}
}

// TestEvalIf tests the EvalIf function
func TestEvalIf(t *testing.T) {
	ctx := map[string]string{"GITHUB_SHA": "abc123"}

	tests := []struct {
		name string
		cond string
		ctx  map[string]string
		want bool
	}{
		{"empty", "", nil, true},
		{"true_literal", "true", nil, true},
		{"false_literal", "false", nil, false},
		{"always", "always()", nil, true},
		{"success", "success()", nil, true},
		{"failure", "failure()", nil, false},
		{"expression_with_value", "${{ github.sha }}", ctx, true},
		{"expression_without_value", "${{ github.nonexistent }}", ctx, false},
		{"expression_false", "${{ false }}", ctx, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalIf(tt.cond, tt.ctx)
			if got != tt.want {
				t.Errorf("EvalIf(%q, ctx) = %v, want %v", tt.cond, got, tt.want)
			}
		})
	}
}

// TestBuildStepScript_IfCondition tests BuildStepScript with if: conditions
func TestBuildStepScript_IfCondition(t *testing.T) {
	ctx := map[string]string{"GITHUB_SHA": "abc123"}

	t.Run("no_if_runs_normally", func(t *testing.T) {
		step := Step{Name: "build", Run: "echo hi"}
		script := BuildStepScript(step, ctx)
		if !contains(script, "echo hi") {
			t.Fatalf("expected 'echo hi' in script, got: %s", script)
		}
	})

	t.Run("if_true_runs", func(t *testing.T) {
		step := Step{Name: "build", Run: "echo hi", If: "true"}
		script := BuildStepScript(step, ctx)
		if !contains(script, "echo hi") {
			t.Fatalf("expected 'echo hi' in script, got: %s", script)
		}
	})

	t.Run("if_false_skips", func(t *testing.T) {
		step := Step{Name: "conditional", Run: "echo hi", If: "false"}
		script := BuildStepScript(step, ctx)
		if contains(script, "echo hi") {
			t.Fatalf("should not contain 'echo hi' when if: false, got: %s", script)
		}
		if !contains(script, "Skipping") {
			t.Fatalf("expected skip warning in script, got: %s", script)
		}
	})

	t.Run("if_expression_true", func(t *testing.T) {
		step := Step{Name: "build", Run: "echo hi", If: "${{ github.sha }}"}
		script := BuildStepScript(step, ctx)
		if !contains(script, "echo hi") {
			t.Fatalf("expected 'echo hi' in script, got: %s", script)
		}
	})

	t.Run("if_expression_false", func(t *testing.T) {
		step := Step{Name: "conditional", Run: "echo hi", If: "${{ github.nonexistent }}"}
		script := BuildStepScript(step, ctx)
		if contains(script, "echo hi") {
			t.Fatalf("should not contain 'echo hi' for false expression, got: %s", script)
		}
		if !contains(script, "Skipping") {
			t.Fatalf("expected skip warning, got: %s", script)
		}
	})

	t.Run("if_always_runs", func(t *testing.T) {
		step := Step{Name: "cleanup", Run: "echo done", If: "always()"}
		script := BuildStepScript(step, ctx)
		if !contains(script, "echo done") {
			t.Fatalf("expected 'echo done' in script, got: %s", script)
		}
	})
}

// TestWriteJobScript_IfCondition tests WriteJobScript with conditional steps
func TestWriteJobScript_IfCondition(t *testing.T) {
	ctx := map[string]string{"GITHUB_SHA": "abc123"}

	t.Run("mixed_if_conditions", func(t *testing.T) {
		steps := []Step{
			{Name: "always_run", Run: "echo step1"},
			{Name: "never_run", Run: "echo step2", If: "false"},
			{Name: "conditional_run", Run: "echo step3", If: "${{ github.sha }}"},
		}
		script := WriteJobScript(steps, ctx)
		if !contains(script, "echo step1") {
			t.Fatal("should contain step1")
		}
		if contains(script, "echo step2") {
			t.Fatal("should NOT contain step2 (if: false)")
		}
		if !contains(script, "echo step3") {
			t.Fatal("should contain step3 (if: expression == true)")
		}
		if !contains(script, "Skipping") {
			t.Fatal("should contain skip warning")
		}
	})
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

// indexOfStr returns the index of target in a string slice, or -1
func indexOfStr(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}
