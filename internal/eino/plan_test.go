package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPlanToolRoundTrip(t *testing.T) {
	pt := NewPlanTool()
	result, err := pt.InvokableRun(context.Background(), `{"steps":[{"content":"第一步","status":"pending"},{"content":"第二步","status":"in_progress"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("raw:", result)

	artifacts := ParseArtifacts(result)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]
	t.Logf("type=%s planID=%s steps=%d", a.Type, a.PlanID, len(a.Steps))
	if a.Type != "plan" {
		t.Errorf("expected type=plan, got %s", a.Type)
	}
	if len(a.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(a.Steps))
	}
	if a.PlanID == "" {
		t.Error("planID should not be empty")
	}

	// 模拟前端 collectArtifacts 的 plan key 逻辑
	planKey := "plan:" + a.PlanID
	t.Logf("plan key: %s", planKey)

	// 验证 JSON 序列化
	b, _ := json.Marshal(a)
	t.Log("json:", string(b))
	_ = fmt.Sprintf("%s", strings.TrimSpace(result))
}
