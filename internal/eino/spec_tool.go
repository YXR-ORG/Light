package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SpecTool 提供 submit_spec：工作流模式下 agent 提交需求规格和验收标准。
// 提交后 react_agent 在 finish_goal 时引用这些验收标准发起独立 review 轮次。
type SpecTool struct {
	mu                 sync.Mutex
	requirement        string
	acceptanceCriteria []AcceptItem
}

// NewSpecTool 创建 submit_spec 工具。
func NewSpecTool() *SpecTool {
	return &SpecTool{}
}

func (t *SpecTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "submit_spec",
		Desc: "提交需求规格和验收标准。在工作流模式下，这是你的第一个工具调用。" +
			"需求描述要清晰完整，验收标准是可检查的具体条件（每条应能明确判断是否满足）。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"requirement": {
				Type:     schema.String,
				Desc:     "需求描述：用户想要什么、解决什么问题、预期效果",
				Required: true,
			},
			"acceptance_criteria": {
				Type:     schema.Array,
				Desc:     "验收标准列表：可检查的具体条件，每条应能明确判断是否满足",
				Required: true,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
		}),
	}, nil
}

func (t *SpecTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Requirement         string   `json:"requirement"`
		AcceptanceCriteria  []string `json:"acceptance_criteria"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("submit_spec: invalid args: %w", err)
	}
	if args.Requirement == "" {
		return "", fmt.Errorf("submit_spec: requirement is required")
	}
	if len(args.AcceptanceCriteria) == 0 {
		return "", fmt.Errorf("submit_spec: at least one acceptance criterion is required")
	}

	// 转换为 AcceptItem（requirement 阶段全部 pending）
	items := make([]AcceptItem, len(args.AcceptanceCriteria))
	for i, c := range args.AcceptanceCriteria {
		items[i] = AcceptItem{Content: c, Status: "pending"}
	}

	t.mu.Lock()
	t.requirement = args.Requirement
	t.acceptanceCriteria = items
	t.mu.Unlock()

	humanText := fmt.Sprintf("需求规格已提交：%s（验收标准 %d 条）", truncateRunes(args.Requirement, 80), len(items))

	return EmbedArtifact(humanText, Artifact{
		Type:               "requirement",
		Title:              "需求规格",
		Requirement:        args.Requirement,
		AcceptanceCriteria: items,
	}), nil
}

// GetRequirement 返回已提交的需求描述。
func (t *SpecTool) GetRequirement() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requirement
}

// GetAcceptanceCriteria 返回已提交的验收标准项。
func (t *SpecTool) GetAcceptanceCriteria() []AcceptItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AcceptItem, len(t.acceptanceCriteria))
	copy(out, t.acceptanceCriteria)
	return out
}

// HasSpec 返回是否已提交需求规格。
func (t *SpecTool) HasSpec() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requirement != ""
}
