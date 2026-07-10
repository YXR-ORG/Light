package eino

import (
	"testing"
)

func TestParseCriteriaLines(t *testing.T) {
	raw := " 测试通过 \n\n首页无报错\n  \n文件已写入\n"
	got := parseCriteriaLines(raw)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %#v", len(got), got)
	}
	if got[0] != "测试通过" || got[1] != "首页无报错" || got[2] != "文件已写入" {
		t.Fatalf("unexpected lines: %#v", got)
	}
}

func TestEffectiveMaxTurns(t *testing.T) {
	if effectiveMaxTurns(0) != defaultMaxEvalTurns {
		t.Fatalf("0 should use default %d", defaultMaxEvalTurns)
	}
	if effectiveMaxTurns(-1) != defaultMaxEvalTurns {
		t.Fatalf("negative should use default")
	}
	if effectiveMaxTurns(8) != 8 {
		t.Fatalf("want 8")
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"overall":"pass"}`, `{"overall":"pass"}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"前置文字 {\"items\":[],\"overall\":\"fail\"} 后置", `{"items":[],"overall":"fail"}`},
		{"no json here", ""},
		{`{"nested":{"x":1},"y":"a}b"}`, `{"nested":{"x":1},"y":"a}b"}`},
	}
	for _, c := range cases {
		got := extractJSONObject(c.in)
		if got != c.want {
			t.Errorf("extractJSONObject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizePassFail(t *testing.T) {
	if normalizePassFail("PASS") != "pass" {
		t.Fatal("PASS")
	}
	if normalizePassFail("failed") != "fail" {
		t.Fatal("failed")
	}
	if normalizePassFail("maybe") != "pending" {
		t.Fatal("maybe")
	}
}

func TestParseEvalJSON(t *testing.T) {
	criteria := []string{"测试全部通过", "无控制台报错"}
	raw := `{"items":[{"criterion":"测试全部通过","status":"pass","reason":"go test ok"},{"criterion":"无控制台报错","status":"fail","reason":"有 error"}],"overall":"fail","issues":["控制台有 error"]}`
	r := parseEvalJSON(raw, criteria)
	if r.passed {
		t.Fatal("should not pass")
	}
	if len(r.issues) == 0 {
		t.Fatal("want issues")
	}
	if r.artifact.Type != "review" {
		t.Fatalf("artifact type: %s", r.artifact.Type)
	}
	if len(r.artifact.AcceptanceCriteria) != 2 {
		t.Fatalf("want 2 items, got %d", len(r.artifact.AcceptanceCriteria))
	}
	if r.artifact.AcceptanceCriteria[0].Status != "pass" {
		t.Fatalf("item0 status: %s", r.artifact.AcceptanceCriteria[0].Status)
	}
	if r.artifact.AcceptanceCriteria[1].Status != "fail" {
		t.Fatalf("item1 status: %s", r.artifact.AcceptanceCriteria[1].Status)
	}
}

func TestParseEvalJSONAllPass(t *testing.T) {
	criteria := []string{"A", "B"}
	raw := `{"items":[{"criterion":"A","status":"pass","reason":"ok"},{"criterion":"B","status":"pass","reason":"ok"}],"overall":"pass","issues":[]}`
	r := parseEvalJSON(raw, criteria)
	if !r.passed {
		t.Fatal("should pass")
	}
}

func TestIsReadOnlyBashCmd(t *testing.T) {
	if !isReadOnlyBashCmd("ls -la") {
		t.Fatal("ls should pass")
	}
	if !isReadOnlyBashCmd("git diff") {
		t.Fatal("git diff should pass")
	}
	if !isReadOnlyBashCmd("go test ./...") {
		t.Fatal("go test should pass")
	}
	if isReadOnlyBashCmd("rm -rf /tmp/x") {
		t.Fatal("rm should fail")
	}
	if isReadOnlyBashCmd("echo hi > file.txt") {
		t.Fatal("redirect write should fail")
	}
	if isReadOnlyBashCmd("npm install lodash") {
		t.Fatal("npm install should fail")
	}
}

func TestExtractWriteFilePath(t *testing.T) {
	p := extractWriteFilePath(`{"path":"src/main.go","content":"package main"}`)
	if p != "src/main.go" {
		t.Fatalf("got %q", p)
	}
	if extractWriteFilePath(`not json`) != "" {
		t.Fatal("invalid json should return empty")
	}
}

func TestFinishGoalToolReset(t *testing.T) {
	tTool := NewFinishGoalTool()
	_, err := tTool.InvokableRun(nil, `{"summary":"done","success":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tTool.IsFinished() {
		t.Fatal("should be finished")
	}
	tTool.Reset()
	if tTool.IsFinished() {
		t.Fatal("should be reset")
	}
	if tTool.GetSummary() != "" {
		t.Fatal("summary should clear")
	}
}
