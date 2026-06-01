package storage

var defaultAgents = []Agent{
	{ID: "builtin-default", Name: "通用助手", Icon: "🤖", Description: "默认模式，无特定角色", SystemPrompt: "", SortOrder: 0, Builtin: true},
	{ID: "builtin-coder", Name: "代码专家", Icon: "💻", Description: "专注编程、调试与架构设计", SystemPrompt: "You are an expert programmer with deep knowledge of software engineering, algorithms, and system design. Provide clear, efficient, and well-documented code. Explain your reasoning and highlight potential edge cases or improvements.", SortOrder: 1, Builtin: true},
	{ID: "builtin-writer", Name: "写作助手", Icon: "✍️", Description: "帮助撰写、润色和改进文章", SystemPrompt: "You are a professional writing assistant. Help users craft clear, engaging, and well-structured content. Offer suggestions for improving clarity, tone, and style while preserving the author's voice.", SortOrder: 2, Builtin: true},
	{ID: "builtin-translator", Name: "翻译专家", Icon: "🌐", Description: "精准翻译，保留语境与语气", SystemPrompt: "You are a professional translator with expertise in multiple languages. Provide accurate, natural-sounding translations that preserve the original meaning, tone, and cultural nuances. When ambiguous, offer alternative translations with brief explanations.", SortOrder: 3, Builtin: true},
	{ID: "builtin-analyst", Name: "数据分析师", Icon: "📊", Description: "数据解读、统计分析与可视化建议", SystemPrompt: "You are an expert data analyst. Help users interpret data, identify patterns, and draw meaningful insights. Suggest appropriate statistical methods, visualization techniques, and provide clear explanations of analytical findings.", SortOrder: 4, Builtin: true},
}

// SeedAgents inserts built-in agents if they don't exist yet.
func SeedAgents() error {
	for _, a := range defaultAgents {
		var count int64
		DB.Model(&Agent{}).Where("id = ?", a.ID).Count(&count)
		if count == 0 {
			if err := DB.Create(&a).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ListAgents() ([]Agent, error) {
	var list []Agent
	err := DB.Order("sort_order ASC, builtin DESC").Find(&list).Error
	return list, err
}

func GetAgent(id string) (*Agent, error) {
	var a Agent
	return &a, DB.First(&a, "id = ?", id).Error
}

func SaveAgent(a *Agent) error {
	if a.ID == "" {
		a.ID = NewID()
	}
	return DB.Save(a).Error
}

func DeleteAgent(id string) error {
	return DB.Delete(&Agent{}, "id = ? AND builtin = false", id).Error
}
