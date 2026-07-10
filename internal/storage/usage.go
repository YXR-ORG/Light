package storage

import (
	"fmt"
	"time"
)

// TokenSnapshot 是一次 token 计数快照。
type TokenSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Add 累加另一份快照（流式场景取 max/累加均可，这里累加多轮调用）。
func (t *TokenSnapshot) Add(prompt, completion, total int) {
	if prompt > 0 {
		t.PromptTokens += prompt
	}
	if completion > 0 {
		t.CompletionTokens += completion
	}
	if total > 0 {
		t.TotalTokens += total
	} else if prompt > 0 || completion > 0 {
		t.TotalTokens += prompt + completion
	}
}

func (t TokenSnapshot) Empty() bool {
	return t.PromptTokens == 0 && t.CompletionTokens == 0 && t.TotalTokens == 0
}

// SaveTokenUsage 写入一条用量记录。
func SaveTokenUsage(convID, messageID, provider, model, mode string, snap TokenSnapshot) error {
	if snap.Empty() {
		return nil
	}
	total := snap.TotalTokens
	if total == 0 {
		total = snap.PromptTokens + snap.CompletionTokens
	}
	u := &TokenUsage{
		ID:               NewID(),
		ConversationID:   convID,
		MessageID:        messageID,
		Provider:         provider,
		Model:            model,
		Mode:             mode,
		PromptTokens:     snap.PromptTokens,
		CompletionTokens: snap.CompletionTokens,
		TotalTokens:      total,
		EstimatedCost:    EstimateCostUSD(model, snap.PromptTokens, snap.CompletionTokens),
		CreatedAt:        time.Now(),
	}
	return DB.Create(u).Error
}

// UpdateMessageTokens 给已保存消息补写 token 字段。
func UpdateMessageTokens(messageID string, snap TokenSnapshot) error {
	if messageID == "" || snap.Empty() {
		return nil
	}
	total := snap.TotalTokens
	if total == 0 {
		total = snap.PromptTokens + snap.CompletionTokens
	}
	return DB.Model(&Message{}).Where("id = ?", messageID).Updates(map[string]interface{}{
		"prompt_tokens":     snap.PromptTokens,
		"completion_tokens": snap.CompletionTokens,
		"total_tokens":      total,
	}).Error
}

// UsageSummary 是前端用量统计面板的汇总结构。
type UsageSummary struct {
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	EstimatedCost         float64 `json:"estimated_cost"`
	RequestCount          int64   `json:"request_count"`
	ByDay                 []UsageBucket `json:"by_day"`
	ByProvider            []UsageBucket `json:"by_provider"`
	ByMode                []UsageBucket `json:"by_mode"`
	ByModel               []UsageBucket `json:"by_model"`
}

// UsageBucket 是某个维度的汇总桶。
type UsageBucket struct {
	Key                   string  `json:"key"`
	PromptTokens          int64   `json:"prompt_tokens"`
	CompletionTokens      int64   `json:"completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	EstimatedCost         float64 `json:"estimated_cost"`
	RequestCount          int64   `json:"request_count"`
}

// GetUsageSummary 按时间范围汇总 token 用量。rangeKey: today|week|month|all
func GetUsageSummary(rangeKey string) (*UsageSummary, error) {
	since, err := usageSince(rangeKey)
	if err != nil {
		return nil, err
	}

	q := DB.Model(&TokenUsage{})
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}

	var total struct {
		Prompt     int64
		Completion int64
		Total      int64
		Cost       float64
		Count      int64
	}
	if err := q.Select(`
		COALESCE(SUM(prompt_tokens),0) as prompt,
		COALESCE(SUM(completion_tokens),0) as completion,
		COALESCE(SUM(total_tokens),0) as total,
		COALESCE(SUM(estimated_cost),0) as cost,
		COUNT(*) as count
	`).Scan(&total).Error; err != nil {
		return nil, err
	}

	summary := &UsageSummary{
		TotalPromptTokens:     total.Prompt,
		TotalCompletionTokens: total.Completion,
		TotalTokens:           total.Total,
		EstimatedCost:         total.Cost,
		RequestCount:          total.Count,
	}

	// by day
	dayQ := DB.Model(&TokenUsage{})
	if !since.IsZero() {
		dayQ = dayQ.Where("created_at >= ?", since)
	}
	var days []struct {
		Day        string
		Prompt     int64
		Completion int64
		Total      int64
		Cost       float64
		Count      int64
	}
	if err := dayQ.Select(`
		strftime('%Y-%m-%d', created_at) as day,
		COALESCE(SUM(prompt_tokens),0) as prompt,
		COALESCE(SUM(completion_tokens),0) as completion,
		COALESCE(SUM(total_tokens),0) as total,
		COALESCE(SUM(estimated_cost),0) as cost,
		COUNT(*) as count
	`).Group("day").Order("day ASC").Scan(&days).Error; err != nil {
		return nil, err
	}
	for _, d := range days {
		summary.ByDay = append(summary.ByDay, UsageBucket{
			Key: d.Day, PromptTokens: d.Prompt, CompletionTokens: d.Completion,
			TotalTokens: d.Total, EstimatedCost: d.Cost, RequestCount: d.Count,
		})
	}

	// by provider / mode / model
	for _, dim := range []struct {
		col  string
		dest *[]UsageBucket
	}{
		{"provider", &summary.ByProvider},
		{"mode", &summary.ByMode},
		{"model", &summary.ByModel},
	} {
		buckets, err := groupUsage(since, dim.col)
		if err != nil {
			return nil, err
		}
		*dim.dest = buckets
	}

	return summary, nil
}

func groupUsage(since time.Time, col string) ([]UsageBucket, error) {
	// col 白名单，防止注入
	switch col {
	case "provider", "mode", "model":
	default:
		return nil, fmt.Errorf("invalid group column: %s", col)
	}
	q := DB.Model(&TokenUsage{})
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}
	var rows []struct {
		Key        string
		Prompt     int64
		Completion int64
		Total      int64
		Cost       float64
		Count      int64
	}
	sql := fmt.Sprintf(`
		CASE WHEN %s = '' OR %s IS NULL THEN 'unknown' ELSE %s END as key,
		COALESCE(SUM(prompt_tokens),0) as prompt,
		COALESCE(SUM(completion_tokens),0) as completion,
		COALESCE(SUM(total_tokens),0) as total,
		COALESCE(SUM(estimated_cost),0) as cost,
		COUNT(*) as count
	`, col, col, col)
	if err := q.Select(sql).Group("key").Order("total DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]UsageBucket, 0, len(rows))
	for _, r := range rows {
		out = append(out, UsageBucket{
			Key: r.Key, PromptTokens: r.Prompt, CompletionTokens: r.Completion,
			TotalTokens: r.Total, EstimatedCost: r.Cost, RequestCount: r.Count,
		})
	}
	return out, nil
}

// GetConversationTokenTotal 返回会话累计 total_tokens。
func GetConversationTokenTotal(convID string) (int64, float64, error) {
	var row struct {
		Total int64
		Cost  float64
	}
	err := DB.Model(&TokenUsage{}).
		Where("conversation_id = ?", convID).
		Select("COALESCE(SUM(total_tokens),0) as total, COALESCE(SUM(estimated_cost),0) as cost").
		Scan(&row).Error
	return row.Total, row.Cost, err
}

func usageSince(rangeKey string) (time.Time, error) {
	now := time.Now()
	switch rangeKey {
	case "", "all":
		return time.Time{}, nil
	case "today":
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), nil
	case "week":
		return now.AddDate(0, 0, -7), nil
	case "month":
		return now.AddDate(0, -1, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unknown range: %s", rangeKey)
	}
}
