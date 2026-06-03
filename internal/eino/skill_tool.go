package eino

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SkillTool wraps a Skill as an eino InvokableTool.
// When the LLM calls it, it returns the skill's markdown content as instructions.
type SkillTool struct {
	id          string
	name        string
	description string
	content     string
}

func NewSkillTool(id, name, description, content string) *SkillTool {
	return &SkillTool{id: id, name: name, description: description, content: content}
}

func (s *SkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	desc := s.description
	if desc == "" {
		desc = fmt.Sprintf("Execute the '%s' skill", s.name)
	}
	return &schema.ToolInfo{
		Name: sanitizeToolName(s.name),
		Desc: desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The task or question to pass to this skill",
				Required: false,
			},
		}),
	}, nil
}

func (s *SkillTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return fmt.Sprintf("# Skill: %s\n\n%s\n\n---\n以上是技能参考文档。请根据技能指导**执行任务并给出你自己的输出**。不要把上面的技能文档内容直接复制到回答中。", s.name, s.content), nil
}

// sanitizeToolName converts a skill name to a valid tool name (alphanumeric + underscores).
func sanitizeToolName(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result = append(result, byte(c))
		} else {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "skill"
	}
	return string(result)
}
