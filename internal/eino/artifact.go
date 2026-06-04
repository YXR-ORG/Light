package eino

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
)

// Artifact 是工具产出的、需要在前端 UI 展示的结构化产物。
//
// 设计理念（通用产物机制）：
// 工具在返回给 LLM 的文本里夹带一段对人类不可见、对前端可解析的标记。
// LLM 读到的是人类可读文本（标记是 HTML 注释，模型通常忽略），
// 前端解析标记后自动在“产物区”渲染对应卡片。
// 新工具想展示产物，只需调用 EmbedArtifact，无需改 agent 或前端任何代码。
type Artifact struct {
	Type    string `json:"type"`               // 产物类型：file | image | url | plan | ...（可扩展）
	Action  string `json:"action,omitempty"`   // 动作：write | read（file 专用）
	Title   string `json:"title,omitempty"`    // 展示标题（如文件名）
	Path    string `json:"path,omitempty"`     // 相对路径（file）
	AbsPath string `json:"abs_path,omitempty"` // 绝对路径（file，用于打开/定位）
	URL     string `json:"url,omitempty"`      // 链接（url/image）
	Bytes   int    `json:"bytes,omitempty"`    // 字节大小（file）
	Mime    string `json:"mime,omitempty"`     // MIME 类型（可选）

	// plan 专用：同一 plan 多次更新共享 PlanID，前端按此去重保留最新
	PlanID string     `json:"plan_id,omitempty"`
	Steps  []PlanStep `json:"steps,omitempty"`
}

// PlanStep 是 plan 产物中的一个步骤。
type PlanStep struct {
	Content string `json:"content"`          // 步骤描述
	Status  string `json:"status,omitempty"` // pending | in_progress | done
}

// artifactMarkerRe 匹配产物标记：<!--ARTIFACT:base64-->
// meta 用 base64 编码，彻底避免产物内容含 "-->" 导致解析错位。
var artifactMarkerRe = regexp.MustCompile(`<!--ARTIFACT:([A-Za-z0-9+/=]*)-->`)

// EmbedArtifact 把一个产物以标记形式追加到工具返回文本末尾。
// humanText 是给 LLM/人类看的可读文本，a 是给前端的结构化产物。
//
// 用法（任意工具）：
//
//	return EmbedArtifact("文件已写入: report.md（1234 字节）", Artifact{
//	    Type: "file", Action: "write", Title: "report.md",
//	    Path: "report.md", AbsPath: abs, Bytes: 1234,
//	}), nil
func EmbedArtifact(humanText string, a Artifact) string {
	meta, err := json.Marshal(a)
	if err != nil {
		return humanText
	}
	enc := base64.StdEncoding.EncodeToString(meta)
	return fmt.Sprintf("%s\n<!--ARTIFACT:%s-->", humanText, enc)
}

// EmbedArtifacts 追加多个产物。
func EmbedArtifacts(humanText string, arts ...Artifact) string {
	out := humanText
	for _, a := range arts {
		out = EmbedArtifact(out, a)
	}
	return out
}

// StripArtifacts 移除文本中的所有产物标记（用于纯文本展示/存储）。
func StripArtifacts(s string) string {
	return artifactMarkerRe.ReplaceAllString(s, "")
}

// ParseArtifacts 从文本中提取所有产物（后端侧，便于测试/复用）。
func ParseArtifacts(s string) []Artifact {
	matches := artifactMarkerRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	arts := make([]Artifact, 0, len(matches))
	for _, m := range matches {
		raw, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			continue
		}
		var a Artifact
		if err := json.Unmarshal(raw, &a); err != nil {
			continue
		}
		if a.Type == "" {
			continue
		}
		arts = append(arts, a)
	}
	return arts
}

// CompletePlanArtifact 返回一个把所有未完成步骤标记为 done 的 plan 产物。
// 用于 agent 已输出最终答案但模型没有再调用 update_plan 的收尾场景。
func CompletePlanArtifact(a Artifact) (Artifact, bool) {
	if a.Type != "plan" || len(a.Steps) == 0 {
		return Artifact{}, false
	}
	out := a
	out.Steps = append([]PlanStep(nil), a.Steps...)
	for i := range out.Steps {
		out.Steps[i].Status = "done"
	}
	return out, true
}
