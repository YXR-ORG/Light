package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// FinishGoalTool 提供 finish_goal：goal 模式下 agent 显式声明目标已达成。
// 调用后 react_agent 检测到并结束循环，summary 作为完成总结展示给用户。
type FinishGoalTool struct {
	mu       sync.Mutex
	finished bool
	success  bool
	summary  string
}

// NewFinishGoalTool 创建 finish_goal 工具。
func NewFinishGoalTool() *FinishGoalTool {
	return &FinishGoalTool{success: true}
}

func (t *FinishGoalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "finish_goal",
		Desc: "当且仅当你确认目标已完全达成时调用此工具。调用后任务将立即结束。" +
			"请在 summary 参数中简要总结你做了什么、达成了什么结果。" +
			"如果目标无法达成（遇到了不可解决的障碍），将 success 设为 false 并在 summary 中说明原因。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"summary": {
				Type:     schema.String,
				Desc:     "目标达成总结：你做了什么、结果如何",
				Required: true,
			},
			"success": {
				Type:     schema.Boolean,
				Desc:     "目标是否成功达成（默认 true）",
				Required: false,
			},
		}),
	}, nil
}

func (t *FinishGoalTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Summary string `json:"summary"`
		Success *bool  `json:"success"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("finish_goal: invalid args: %w", err)
	}

	success := true
	if args.Success != nil {
		success = *args.Success
	}

	t.mu.Lock()
	t.finished = true
	t.success = success
	t.summary = args.Summary
	t.mu.Unlock()

	if success {
		return fmt.Sprintf("✅ 目标已达成：%s", args.Summary), nil
	}
	return fmt.Sprintf("⚠️ 目标未达成：%s", args.Summary), nil
}

// IsFinished 返回 finish_goal 是否已被调用。
func (t *FinishGoalTool) IsFinished() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.finished
}

// GetSummary 返回 finish_goal 调用时附带的总结。
func (t *FinishGoalTool) GetSummary() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summary
}

// GetSuccess 返回 finish_goal 调用时是否成功达成。
func (t *FinishGoalTool) GetSuccess() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.success
}

// Reset 清除 finished 状态，允许验收打回后再次调用 finish_goal。
func (t *FinishGoalTool) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = false
	t.success = true
	t.summary = ""
}
