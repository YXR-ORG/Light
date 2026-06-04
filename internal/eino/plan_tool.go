package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// PlanTool 提供 update_plan：让 agent 在执行复杂任务前列出计划、执行中更新步骤状态。
// 计划通过产物机制（type:"plan"）推给前端渲染成可视化待办列表。
// 同一 PlanTool 实例共享一个 PlanID，前端按此去重，始终展示最新一次计划。
type PlanTool struct {
	planID string
}

// NewPlanTool 创建一个 update_plan 工具。planID 在每次任务运行时唯一生成。
func NewPlanTool() *PlanTool {
	return &PlanTool{planID: "plan-" + strconv.FormatInt(time.Now().UnixNano(), 36)}
}

func (t *PlanTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "update_plan",
		Desc: "列出或更新当前任务的执行计划（待办清单）。面对多步骤复杂任务时，先调用本工具列出计划；每完成一步，再次调用更新对应步骤的状态。简单任务无需调用。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"steps": {
				Type:     schema.Array,
				Desc:     "计划步骤列表，按执行顺序排列",
				Required: true,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"content": {Type: schema.String, Desc: "步骤描述", Required: true},
						"status": {
							Type:     schema.String,
							Desc:     "步骤状态：pending（待办）| in_progress（进行中）| done（已完成）",
							Required: false,
							Enum:     []string{"pending", "in_progress", "done"},
						},
					},
				},
			},
		}),
	}, nil
}

func (t *PlanTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Steps []PlanStep `json:"steps"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("update_plan: invalid args: %w", err)
	}
	// 规范化 status，默认 pending
	for i := range args.Steps {
		switch args.Steps[i].Status {
		case "pending", "in_progress", "done":
		default:
			args.Steps[i].Status = "pending"
		}
	}

	done := 0
	for _, s := range args.Steps {
		if s.Status == "done" {
			done++
		}
	}
	humanText := fmt.Sprintf("计划已更新：共 %d 步，已完成 %d 步", len(args.Steps), done)

	return EmbedArtifact(humanText, Artifact{
		Type:   "plan",
		Title:  "执行计划",
		PlanID: t.planID,
		Steps:  args.Steps,
	}), nil
}
