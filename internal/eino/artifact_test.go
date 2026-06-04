package eino

import "testing"

func TestArtifactRoundTrip(t *testing.T) {
	a := Artifact{
		Type: "file", Action: "write", Title: "r.md",
		Path: "r.md", AbsPath: "/abs/r.md", Bytes: 42,
	}
	text := EmbedArtifact("文件已写入: r.md（42 字节）", a)

	got := ParseArtifacts(text)
	if len(got) != 1 {
		t.Fatalf("want 1 artifact, got %d", len(got))
	}
	if got[0].AbsPath != "/abs/r.md" || got[0].Action != "write" || got[0].Bytes != 42 {
		t.Fatalf("artifact mismatch: %+v", got[0])
	}

	// Strip 后应不含标记
	if s := StripArtifacts(text); s == text || ParseArtifacts(s) != nil {
		t.Fatalf("strip failed: %q", s)
	}
}

func TestArtifactContentWithArrowSeq(t *testing.T) {
	// 产物内容/上下文里含 "-->"（如读取 HTML），base64 编码后不应破坏解析
	body := "<html><!-- comment --> <div>--></div>"
	a := Artifact{Type: "file", Action: "read", Title: "x.html", Path: "x.html", AbsPath: "/abs/x.html", Bytes: 10}
	text := EmbedArtifact(body, a)

	got := ParseArtifacts(text)
	if len(got) != 1 {
		t.Fatalf("want 1 artifact despite '-->' in body, got %d", len(got))
	}
	if got[0].AbsPath != "/abs/x.html" {
		t.Fatalf("abs_path mismatch: %+v", got[0])
	}
}

func TestParseMultipleArtifacts(t *testing.T) {
	text := EmbedArtifacts("done",
		Artifact{Type: "file", Action: "read", AbsPath: "/a"},
		Artifact{Type: "file", Action: "write", AbsPath: "/b"},
	)
	got := ParseArtifacts(text)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
}

func TestCompletePlanMarksRemainingStepsDone(t *testing.T) {
	plan := Artifact{
		Type:   "plan",
		Title:  "执行计划",
		PlanID: "plan-1",
		Steps: []PlanStep{
			{Content: "收集资料", Status: "done"},
			{Content: "整理输出总结", Status: "in_progress"},
			{Content: "复查", Status: "pending"},
		},
	}

	completed, ok := CompletePlanArtifact(plan)
	if !ok {
		t.Fatal("expected plan artifact to be completable")
	}
	for i, step := range completed.Steps {
		if step.Status != "done" {
			t.Fatalf("step %d status = %q, want done", i, step.Status)
		}
	}
}

func TestCompletePlanRejectsNonPlanArtifacts(t *testing.T) {
	_, ok := CompletePlanArtifact(Artifact{Type: "file", Title: "x.md"})
	if ok {
		t.Fatal("file artifact should not be completable as plan")
	}
}
