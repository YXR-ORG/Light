package storage

import "strings"

// 内置模型单价（美元 / 百万 token）。找不到时费用记 0。
// key 用小写模型名子串匹配。
var defaultPricing = []struct {
	match              string
	promptPerMillion   float64
	completionPerMillion float64
}{
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-4o", 2.50, 10.00},
	{"gpt-4.1-mini", 0.40, 1.60},
	{"gpt-4.1", 2.00, 8.00},
	{"gpt-4", 30.00, 60.00},
	{"o3-mini", 1.10, 4.40},
	{"o1-mini", 1.10, 4.40},
	{"o1", 15.00, 60.00},
	{"claude-3-5-haiku", 0.80, 4.00},
	{"claude-3-haiku", 0.25, 1.25},
	{"claude-3-5-sonnet", 3.00, 15.00},
	{"claude-sonnet-4", 3.00, 15.00},
	{"claude-3-opus", 15.00, 75.00},
	{"claude-opus-4", 15.00, 75.00},
	{"deepseek-chat", 0.27, 1.10},
	{"deepseek-reasoner", 0.55, 2.19},
	{"deepseek-v3", 0.27, 1.10},
	{"deepseek-v4-flash", 0.14, 0.28},
	{"deepseek-v4", 0.55, 2.19},
	{"deepseek", 0.27, 1.10},
	{"qwen-turbo", 0.05, 0.20},
	{"qwen-plus", 0.40, 1.20},
	{"qwen-max", 1.60, 6.40},
	{"gemini-1.5-flash", 0.075, 0.30},
	{"gemini-2.0-flash", 0.10, 0.40},
	{"gemini-1.5-pro", 1.25, 5.00},
	{"gemini-2.5-pro", 1.25, 10.00},
}

// EstimateCostUSD 按模型名估算费用（美元）。未知模型返回 0。
func EstimateCostUSD(model string, promptTokens, completionTokens int) float64 {
	if promptTokens <= 0 && completionTokens <= 0 {
		return 0
	}
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, p := range defaultPricing {
		if strings.Contains(lower, p.match) {
			return float64(promptTokens)/1_000_000*p.promptPerMillion +
				float64(completionTokens)/1_000_000*p.completionPerMillion
		}
	}
	return 0
}
